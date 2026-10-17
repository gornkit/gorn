package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/gornkit/gorn/internal/cache"
	"github.com/gornkit/gorn/internal/progress"
)

type CacheOpts struct {
	Operation string
	JSON      bool
	KeyPrefix string
	All       bool
	Yes       bool
	DryRun    bool
	MaxAge    *time.Duration
	MaxBytes  *int64
	Verbose   bool
	Quiet     bool
	Stdout    io.Writer
	Stderr    io.Writer
}

func Cache(o CacheOpts) error { return CacheContext(context.Background(), o) }
func CacheContext(ctx context.Context, o CacheOpts) (resultErr error) {
	if ctx == nil {
		return fmt.Errorf("nil cache context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	options := cache.MaintenanceOptions{Operation: o.Operation, KeyPrefix: o.KeyPrefix, All: o.All, Yes: o.Yes, DryRun: o.DryRun, MaxAge: o.MaxAge, MaxBytes: o.MaxBytes}
	switch o.Operation {
	case "clean", "prune":
		if err := options.Validate(); err != nil {
			return &StatusError{Code: 2, Err: err}
		}
	case "path", "list":
	default:
		return &StatusError{Code: 2, Err: fmt.Errorf("unknown cache operation")}
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	ctx, report := progress.New(ctx, o.Stderr, o.Quiet)
	defer func() {
		if err := report.Finish(); err != nil {
			resultErr = &StatusError{Code: 1, Err: fmt.Errorf("write output: %w", errors.Join(err, resultErr))}
		}
	}()
	out := report.Synchronize(o.Stdout)
	if err := progress.Phase(ctx, "resolving cache root"); err != nil {
		return err
	}
	root, err := cache.Resolve()
	if err != nil {
		return err
	}
	switch o.Operation {
	case "path":
		if o.JSON {
			return writeCacheJSON(out, struct {
				Schema int    `json:"schema"`
				Root   string `json:"root"`
			}{1, root.Path()})
		}
		return writeCacheBytes(out, []byte(root.Path()+"\n"))
	case "list":
		if err := progress.Phase(ctx, "inspecting cache"); err != nil {
			return err
		}
		inventory, err := root.Inspect(ctx)
		if err != nil {
			return err
		}
		if o.JSON {
			return writeCacheJSON(out, inventory)
		}
		return writeCacheBytes(out, inventoryText(inventory))
	default:
		result, err := root.Maintain(ctx, options)
		var writeErr error
		if o.JSON {
			writeErr = writeCacheJSON(out, result)
		} else {
			writeErr = writeCacheBytes(out, maintenanceText(result))
		}
		return errors.Join(err, writeErr)
	}
}

func writeCacheJSON(out io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeCacheBytes(out, append(data, '\n'))
}

func writeCacheBytes(out io.Writer, data []byte) error {
	n, err := out.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return fmt.Errorf("write cache output: %w", err)
	}
	return nil
}

func sizeText(size cache.Size) string {
	if !size.Complete {
		return fmt.Sprintf("%d (incomplete)", size.Bytes)
	}
	return fmt.Sprint(size.Bytes)
}

func inventoryText(inventory cache.Inventory) []byte {
	var buffer bytes.Buffer
	table := tabwriter.NewWriter(&buffer, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "KIND\tSTATE\tBYTES\tBUILT\tGO / TARGET\tKEY\tSOURCE / PATH")
	for _, item := range inventory.Items {
		source := item.Source
		if source == "" {
			source = item.Path
		}
		provenance := ""
		if item.Profile != nil {
			provenance = fmt.Sprintf("%s %s/%s", item.Profile.GoVersion, item.Profile.GOOS, item.Profile.GOARCH)
		}
		_, _ = fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%q\t%s\t%q\n", item.Kind, item.State, sizeText(cache.Size{Bytes: item.SizeBytes, Complete: item.SizeComplete}), item.BuiltAt, provenance, item.Key, source)
		if item.Reason != "" {
			_, _ = fmt.Fprintf(table, "\t%s\n", item.Reason)
		}
	}
	_ = table.Flush()
	_, _ = fmt.Fprintf(&buffer, "Managed bytes: %s; quarantine: %s; stages: %s; unmanaged: %s; metadata: %s\n", sizeText(inventory.Totals.Managed), sizeText(inventory.Totals.Quarantine), sizeText(inventory.Totals.Stages), sizeText(inventory.Totals.Unmanaged), sizeText(inventory.Totals.Metadata))
	return buffer.Bytes()
}

func maintenanceText(report cache.MaintenanceReport) []byte {
	var buffer bytes.Buffer
	if report.DryRun {
		_, _ = fmt.Fprintln(&buffer, "Preview only; activity is not reserved.")
	}
	for _, action := range report.Actions {
		_, _ = fmt.Fprintf(&buffer, "%s %s %s (%d bytes)\n", action.State, action.Kind, action.Key, action.Bytes)
		if action.Error != "" {
			_, _ = fmt.Fprintln(&buffer, action.Error)
		}
	}
	if len(report.Actions) == 0 {
		_, _ = fmt.Fprintln(&buffer, "Nothing selected.")
	}
	_, _ = fmt.Fprintf(&buffer, "Managed bytes: %s\n", sizeText(report.After.Managed))
	if report.Predicted != nil {
		_, _ = fmt.Fprintf(&buffer, "Predicted managed bytes: %s\n", sizeText(report.Predicted.Managed))
	}
	if report.UnmetBudget {
		_, _ = fmt.Fprintln(&buffer, "Requested managed-byte budget remains unmet.")
	}
	return buffer.Bytes()
}
