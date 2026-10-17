package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/gornkit/gorn/internal/progress"
)

var ErrMaintenanceRequest = errors.New("invalid cache maintenance request")

type MaintenanceOptions struct {
	Operation string
	KeyPrefix string
	All       bool
	Yes       bool
	DryRun    bool
	MaxAge    *time.Duration
	MaxBytes  *int64
}
type Action struct {
	Kind  string `json:"kind"`
	Key   string `json:"key"`
	Path  string `json:"path"`
	State string `json:"state"`
	Bytes int64  `json:"bytes"`
	Error string `json:"error,omitempty"`
}
type MaintenanceReport struct {
	Schema      int      `json:"schema"`
	Root        string   `json:"root"`
	Operation   string   `json:"operation"`
	DryRun      bool     `json:"dry_run"`
	Actions     []Action `json:"actions"`
	Before      Totals   `json:"before"`
	After       Totals   `json:"after"`
	Predicted   *Totals  `json:"predicted,omitempty"`
	UnmetBudget bool     `json:"unmet_budget"`
	Error       string   `json:"error,omitempty"`
}

// Validate checks explicit selection, bounds and noninteractive deletion intent.
func (o MaintenanceOptions) Validate() error { return validateMaintenance(o) }

func validateMaintenance(o MaintenanceOptions) error {
	invalid := func(message string) error { return fmt.Errorf("%w: %s", ErrMaintenanceRequest, message) }
	if !o.Yes && !o.DryRun {
		return invalid("use --yes to delete, or --dry-run to preview")
	}
	switch o.Operation {
	case "clean":
		if (o.KeyPrefix == "") == !o.All {
			return invalid("select one key prefix or --all")
		}
		if o.MaxAge != nil || o.MaxBytes != nil {
			return invalid("clean does not accept prune bounds")
		}
		if o.KeyPrefix != "" {
			if len(o.KeyPrefix) > 64 {
				return invalid("key prefix must contain 1–64 lowercase hexadecimal characters")
			}
			for _, char := range o.KeyPrefix {
				if !strings.ContainsRune("0123456789abcdef", char) {
					return invalid("key prefix must contain 1–64 lowercase hexadecimal characters")
				}
			}
		}
	case "prune":
		if o.All || o.KeyPrefix != "" {
			return invalid("prune selects by age/bytes, not key or --all")
		}
		if o.MaxAge == nil && o.MaxBytes == nil {
			return invalid("prune requires --max-age and/or --max-bytes")
		}
		if o.MaxAge != nil && *o.MaxAge < 0 {
			return invalid("--max-age must be nonnegative")
		}
		if o.MaxBytes != nil && *o.MaxBytes < 0 {
			return invalid("--max-bytes must be nonnegative")
		}
	default:
		return invalid("unknown operation")
	}
	return nil
}

// Maintain selects once, skips active/changed publications, and reports a final
// snapshot. It never accepts public inventory records as deletion authority.
func (r Root) Maintain(ctx context.Context, o MaintenanceOptions) (report MaintenanceReport, resultErr error) {
	report = MaintenanceReport{Schema: 1, Root: r.Path(), Operation: o.Operation, DryRun: o.DryRun, Actions: []Action{}}
	defer func() {
		if resultErr != nil {
			report.Error = sanitizeDiagnostic(resultErr.Error())
		}
	}()
	if ctx == nil {
		return report, fmt.Errorf("nil maintenance context")
	}
	if err := validateMaintenance(o); err != nil {
		return report, err
	}
	if o.MaxBytes != nil {
		report.UnmetBudget = true
	}
	now := time.Now()
	if err := progress.Phase(ctx, "inspecting cache"); err != nil {
		return report, err
	}
	before, candidates, err := r.inspect(ctx)
	report.Before = before.Totals
	if o.DryRun || err != nil {
		report.After = before.Totals
	}
	if err != nil {
		return report, err
	}
	if o.Operation == "clean" && !o.All {
		selected := []candidate{}
		for _, candidate := range candidates {
			if candidate.item.Kind == "entry" && strings.HasPrefix(candidate.owner.Key, o.KeyPrefix) {
				selected = append(selected, candidate)
			}
		}
		if len(selected) != 1 {
			return report, fmt.Errorf("key prefix %q matches %d owned current entries; select one unambiguous key", o.KeyPrefix, len(selected))
		}
		candidates = selected
	}
	slices.SortFunc(candidates, func(a, b candidate) int {
		if o.Operation == "prune" {
			first, _ := time.Parse(time.RFC3339Nano, a.owner.BuiltAt)
			second, _ := time.Parse(time.RFC3339Nano, b.owner.BuiltAt)
			if first.Before(second) {
				return -1
			}
			if first.After(second) {
				return 1
			}
		}
		return strings.Compare(a.item.Path, b.item.Path)
	})
	predicted := before.Totals
	estimated := before.Totals.Managed.Bytes
	sizeKnown := before.Totals.Managed.Complete
	attempted := map[string]bool{}
	if o.Operation == "prune" && o.MaxBytes != nil && !sizeKnown {
		resultErr = fmt.Errorf("cannot select size eviction: managed byte total is incomplete")
	}
	apply := func(candidate candidate, reason string) {
		attempted[candidate.item.Path] = true
		action := Action{Kind: candidate.item.Kind, Key: candidate.owner.Key, Path: candidate.item.Path, Bytes: candidate.item.SizeBytes, State: "selected"}
		if o.DryRun {
			switch candidate.item.Kind {
			case "entry":
				subtractSize(&predicted.Managed, candidate.item.SizeBytes)
			case "stage":
				subtractSize(&predicted.Stages, candidate.item.SizeBytes)
			case "quarantine":
				subtractSize(&predicted.Quarantine, candidate.item.SizeBytes)
			}
		} else {
			var err error
			action, err = r.removeCandidate(ctx, candidate, reason == "size")
			resultErr = errors.Join(resultErr, err)
			if action.State == "skipped-size-unknown" {
				sizeKnown = false
			}
		}
		report.Actions = append(report.Actions, action)
		if candidate.item.Kind == "entry" && (action.State == "selected" || action.State == "removed") {
			estimated = max(0, estimated-candidate.item.SizeBytes)
		}
	}
	label := "cleaning cache"
	if o.Operation == "prune" {
		label = "pruning cache"
	}
	if err := progress.Phase(ctx, label); err != nil {
		return report, errors.Join(resultErr, err)
	}
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		if o.Operation == "clean" {
			apply(candidate, "clean")
			continue
		}
		if candidate.item.Kind != "entry" || o.MaxAge == nil {
			continue
		}
		built, _ := time.Parse(time.RFC3339Nano, candidate.owner.BuiltAt)
		if built.Before(now.Add(-*o.MaxAge)) {
			apply(candidate, "age")
		}
	}
	if o.Operation == "prune" && o.MaxBytes != nil && sizeKnown {
		for _, candidate := range candidates {
			if ctx.Err() != nil || !sizeKnown || estimated <= *o.MaxBytes {
				break
			}
			if candidate.item.Kind != "entry" || attempted[candidate.item.Path] {
				continue
			}
			apply(candidate, "size")
		}
	}
	if o.DryRun {
		report.Predicted = &predicted
		if o.MaxBytes != nil {
			report.UnmetBudget = !predicted.Managed.Complete || predicted.Managed.Bytes > *o.MaxBytes
		}
		return report, errors.Join(resultErr, ctx.Err())
	}
	// A canceled/failed scan is explicitly incomplete. Never turn it into a
	// successful zero-byte result or repeatedly evict against concurrent writers.
	after, scanErr := r.Inspect(ctx)
	report.After = after.Totals
	resultErr = errors.Join(resultErr, scanErr)
	if o.MaxBytes != nil {
		report.UnmetBudget = !after.Totals.Managed.Complete || after.Totals.Managed.Bytes > *o.MaxBytes
		if report.UnmetBudget {
			resultErr = errors.Join(resultErr, fmt.Errorf("managed cache byte budget remains unmet; active, changed, failed or newly built entries may remain"))
		}
	}
	return report, errors.Join(resultErr, ctx.Err())
}

func subtractSize(size *Size, bytes int64) {
	if bytes > size.Bytes {
		size.Bytes = 0
		size.Complete = false
	} else {
		size.Bytes -= bytes
	}
}

func (r Root) removeCandidate(ctx context.Context, c candidate, sizeDriven bool) (action Action, resultErr error) {
	action = Action{Kind: c.item.Kind, Key: c.owner.Key, Path: c.item.Path, Bytes: c.item.SizeBytes, State: "failed"}
	defer func() {
		if resultErr != nil {
			action.Error = sanitizeDiagnostic(resultErr.Error())
		}
	}()
	var first, second *fileLock
	defer func() { resultErr = errors.Join(resultErr, second.close(), first.close()) }()
	var err error
	if c.item.Kind == "stage" {
		first, err = r.lock(ctx, "stages", c.owner.StageID, true, false)
	} else {
		first, err = r.lock(ctx, "entries", c.owner.Key+".mutation", true, false)
		if err == nil {
			second, err = r.lock(ctx, "entries", c.owner.Key+".activity", true, false)
		}
	}
	if errors.Is(err, errLockBusy) {
		action.State = "skipped-active"
		return action, nil
	}
	if err != nil {
		return action, err
	}
	if err := r.checkDir(c.item.Path); err != nil {
		if os.IsNotExist(err) {
			action.State = "skipped-missing"
			return action, nil
		}
		action.State = "skipped-changed"
		return action, nil
	}
	actual, err := readOwner(c.item.Path)
	if err != nil || actual != c.owner {
		action.State = "skipped-changed"
		return action, nil
	}
	identity, err := captureIdentity(c.item.Path)
	if err != nil {
		return action, err
	}
	if c.identity == nil || !os.SameFile(c.identity, identity) {
		action.State = "skipped-changed"
		return action, nil
	}
	size := measuredSize(ctx, c.item.Path)
	if err := ctx.Err(); err != nil {
		return action, err
	}
	if sizeDriven && !size.Complete {
		action.State = "skipped-size-unknown"
		return action, fmt.Errorf("cannot size-prune entry with incomplete byte count: %s", c.owner.Key)
	}
	action.Bytes = size.Bytes
	if err := removeOwnedDir(ctx, c.item.Path, c.owner, identity); err != nil {
		return action, err
	}
	action.State = "removed"
	return action, nil
}
