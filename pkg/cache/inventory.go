package cache

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/gornkit/gorn/pkg/buildprofile"
)

// Item describes observed data; its fields confer no deletion authority.
type Item struct {
	Kind         string               `json:"kind"`
	Path         string               `json:"path"`
	Key          string               `json:"key,omitempty"`
	Source       string               `json:"source,omitempty"`
	State        string               `json:"state"`
	Reason       string               `json:"reason,omitempty"`
	Created      string               `json:"created,omitempty"`
	BuiltAt      string               `json:"built_at,omitempty"`
	SizeBytes    int64                `json:"size_bytes"`
	SizeComplete bool                 `json:"size_complete"`
	Profile      *buildprofile.Record `json:"profile,omitempty"`
}

type (
	Size struct {
		Bytes    int64 `json:"bytes"`
		Complete bool  `json:"complete"`
	}
	Totals struct {
		Managed    Size `json:"managed"`
		Quarantine Size `json:"quarantine"`
		Stages     Size `json:"stages"`
		Unmanaged  Size `json:"unmanaged"`
		Metadata   Size `json:"metadata"`
	}
)

type Inventory struct {
	Schema int    `json:"schema"`
	Root   string `json:"root"`
	Items  []Item `json:"items"`
	Totals Totals `json:"totals"`
}

type candidate struct {
	item     Item
	owner    ownership
	identity os.FileInfo
}

// Inspect performs a Go-independent, read-only snapshot without following links.
// Provenance consistency is recorded identity, not installed-toolchain validation.
func (r Root) Inspect(ctx context.Context) (Inventory, error) {
	inventory, _, err := r.inspect(ctx)
	return inventory, err
}

func (r Root) inspect(ctx context.Context) (inventory Inventory, candidates []candidate, resultErr error) {
	inventory = Inventory{Schema: 1, Root: r.Path(), Items: []Item{}, Totals: Totals{Managed: Size{Complete: true}, Quarantine: Size{Complete: true}, Stages: Size{Complete: true}, Unmanaged: Size{Complete: true}, Metadata: Size{Complete: true}}}
	unknownCategories := map[string]bool{}
	defer func() {
		finishInventory(&inventory, resultErr == nil)
		if unknownCategories["entry"] {
			inventory.Totals.Managed.Complete = false
		}
		if unknownCategories["stage"] {
			inventory.Totals.Stages.Complete = false
		}
		if unknownCategories["quarantine"] {
			inventory.Totals.Quarantine.Complete = false
		}
	}()
	if ctx == nil {
		return inventory, nil, fmt.Errorf("nil inventory context")
	}
	if err := ctx.Err(); err != nil {
		return inventory, nil, err
	}
	add := func(path, kind string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, owner, uncertain := r.inspectItem(ctx, path, kind)
		unknownCategories[kind] = unknownCategories[kind] || uncertain
		if err := ctx.Err(); err != nil {
			return err
		}
		if item.Kind == "entry" || item.Kind == "stage" || item.Kind == "quarantine" {
			identity, err := captureIdentity(path)
			if err != nil {
				item.SizeComplete = false
				item.Reason = "directory identity unavailable; preserved"
			}
			candidates = append(candidates, candidate{item: item, owner: owner, identity: identity})
		}
		inventory.Items = append(inventory.Items, item)
		return nil
	}
	scan := func(path, kind string) error {
		if err := r.checkDir(path); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			unknownCategories[kind] = uncertainOwnership(err)
			return add(path, "unmanaged")
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			unknownCategories[kind] = true
			return add(path, "unmanaged")
		}
		for _, entry := range entries {
			if err := add(filepath.Join(path, entry.Name()), kind); err != nil {
				return err
			}
		}
		return nil
	}
	roots, err := os.ReadDir(r.Path())
	if os.IsNotExist(err) {
		return inventory, nil, nil
	}
	if err != nil {
		return inventory, nil, err
	}
	for _, entry := range roots {
		path := filepath.Join(r.Path(), entry.Name())
		switch entry.Name() {
		case "apps":
			if err := r.checkDir(path); err != nil {
				unknownCategories["entry"] = uncertainOwnership(err)
				if err := add(path, "unmanaged"); err != nil {
					return inventory, nil, err
				}
				continue
			}
			layouts, err := os.ReadDir(path)
			if err != nil {
				unknownCategories["entry"] = true
				if err := add(path, "unmanaged"); err != nil {
					return inventory, nil, err
				}
				continue
			}
			for _, layout := range layouts {
				location := filepath.Join(path, layout.Name())
				if layout.Name() == strconv.Itoa(manifestSchema) {
					if err := scan(location, "entry"); err != nil {
						return inventory, nil, err
					}
				} else if err := add(location, "unmanaged"); err != nil {
					return inventory, nil, err
				}
			}
		case "tmp":
			if err := scan(path, "stage"); err != nil {
				return inventory, nil, err
			}
		case "quarantine":
			if err := scan(path, "quarantine"); err != nil {
				return inventory, nil, err
			}
		case "locks":
			if err := add(path, "metadata"); err != nil {
				return inventory, nil, err
			}
		default:
			if err := add(path, "unmanaged"); err != nil {
				return inventory, nil, err
			}
		}
	}
	return inventory, candidates, ctx.Err()
}

func finishInventory(inventory *Inventory, complete bool) {
	inventory.Totals = Totals{Managed: Size{Complete: complete}, Quarantine: Size{Complete: complete}, Stages: Size{Complete: complete}, Unmanaged: Size{Complete: complete}, Metadata: Size{Complete: complete}}
	slices.SortFunc(inventory.Items, func(a, b Item) int { return strings.Compare(a.Path, b.Path) })
	for _, item := range inventory.Items {
		var total *Size
		switch item.Kind {
		case "entry":
			total = &inventory.Totals.Managed
		case "stage":
			total = &inventory.Totals.Stages
		case "quarantine":
			total = &inventory.Totals.Quarantine
		case "metadata":
			total = &inventory.Totals.Metadata
		default:
			total = &inventory.Totals.Unmanaged
		}
		addSize(total, item.SizeBytes, item.SizeComplete)
	}
}

func addSize(total *Size, bytes int64, complete bool) {
	total.Complete = total.Complete && complete
	if bytes < 0 || bytes > math.MaxInt64-total.Bytes {
		total.Bytes = math.MaxInt64
		total.Complete = false
		return
	}
	total.Bytes += bytes
}

func measuredSize(ctx context.Context, path string) Size {
	result := Size{Complete: true}
	_ = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			result.Complete = false
			return err
		}
		if walkErr != nil {
			result.Complete = false
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			result.Complete = false
			return nil
		}
		if info.Mode().IsRegular() {
			addSize(&result, info.Size(), true)
		}
		return nil
	})
	return result
}

// A filesystem failure can conceal valid ownership. Malformed, missing or
// nonregular records establish unmanaged data instead of unknown membership.
func uncertainOwnership(err error) bool {
	var pathErr *os.PathError
	return !os.IsNotExist(err) && errors.As(err, &pathErr)
}

func (r Root) inspectItem(ctx context.Context, path, kind string) (Item, ownership, bool) {
	size := measuredSize(ctx, path)
	item := Item{Kind: kind, Path: path, State: kind, SizeBytes: size.Bytes, SizeComplete: size.Complete}
	unknown := func(reason string, uncertain bool) (Item, ownership, bool) {
		item.Kind = "unmanaged"
		item.State = "unmanaged"
		item.Reason = reason
		return item, ownership{}, uncertain
	}
	if kind == "unmanaged" {
		return unknown("unrecognized data or unsupported layout; preserved", false)
	}
	if err := r.checkDir(path); err != nil {
		return unknown("not a plain cache directory; preserved", uncertainOwnership(err))
	}
	if kind == "metadata" {
		return item, ownership{}, false
	}
	owner, err := readOwner(path)
	if err != nil {
		return unknown("missing, invalid or unreadable independent ownership; preserved", uncertainOwnership(err))
	}
	base := filepath.Base(path)
	switch kind {
	case "entry":
		if !validKey(base) || owner.Key != base || owner.Kind != "entry" {
			return unknown("ownership does not match entry location; preserved", false)
		}
	case "stage":
		if !validStage(base) || owner.StageID != base {
			return unknown("ownership does not match stage location; preserved", false)
		}
	case "quarantine":
		if owner.Kind != "entry" || !strings.HasPrefix(base, owner.Key+"-") || len(base) <= len(owner.Key)+1 {
			return unknown("ownership does not match quarantine location; preserved", false)
		}
	}
	item.Key, item.Source, item.Created, item.BuiltAt = owner.Key, sanitizeDiagnostic(owner.Source), owner.Created, owner.BuiltAt
	if kind != "entry" {
		return item, owner, false
	}
	item.State = "invalid"
	if !regular(filepath.Join(path, manifestName)) {
		item.Reason = "manifest missing or nonregular"
		return item, owner, false
	}
	manifest, err := LoadManifest(path)
	if err != nil {
		item.Reason = "manifest unreadable or invalid JSON"
		return item, owner, false
	}
	if manifest.Schema != manifestSchema || manifest.Key != owner.Key || manifest.Profile.Schema != 1 || manifest.Profile.Identity == "" || manifest.Profile.Fingerprint() != manifest.Profile.Identity || manifest.GoVersion != manifest.Profile.GoVersion || manifest.GOOS != manifest.Profile.GOOS || manifest.GOARCH != manifest.Profile.GOARCH {
		item.Reason = "manifest key, profile or schema mismatch"
		return item, owner, false
	}
	profile := safeProfile(manifest.Profile)
	item.Profile = &profile
	if manifest.GOOS != runtime.GOOS || manifest.GOARCH != runtime.GOARCH {
		item.State = "foreign"
		item.Reason = "recorded target differs from this host"
		return item, owner, false
	}
	if r.checkDir(filepath.Join(path, "bin")) != nil || !regular(filepath.Join(path, "bin", binName)) {
		item.Reason = "binary missing or nonregular"
		return item, owner, false
	}
	sum, err := fileSHA256Context(ctx, filepath.Join(path, "bin", binName))
	if err != nil || sum != manifest.BinSHA256 {
		item.Reason = "binary unreadable or checksum mismatch"
		return item, owner, false
	}
	item.State = "valid"
	return item, owner, false
}

func safeProfile(record buildprofile.Record) buildprofile.Record {
	// Record consists of scalar fields; sanitize every string so future string
	// metadata receives the same transport-credential redaction automatically.
	value := reflect.ValueOf(&record).Elem()
	for i := range value.NumField() {
		if value.Field(i).Kind() == reflect.String {
			value.Field(i).SetString(sanitizeDiagnostic(value.Field(i).String()))
		}
	}
	return record
}
