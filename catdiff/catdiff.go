// Package catdiff compares two OSCAL catalogs control by control. It is used
// to check resolved profiles against reference resolutions (such as NIST's
// published resolved baselines) and to show what changes between baselines.
package catdiff

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/SlateGitOrg/oscalkit/oscal"
)

// Report is the difference between catalog A and catalog B.
type Report struct {
	ControlsA int `json:"controls-a"`
	ControlsB int `json:"controls-b"`
	// OnlyInA / OnlyInB list control ids present in just one catalog.
	OnlyInA []string `json:"only-in-a,omitempty"`
	OnlyInB []string `json:"only-in-b,omitempty"`
	// OrderDiffers is set when the shared controls appear in a different
	// document order.
	OrderDiffers bool `json:"order-differs,omitempty"`
	// Moved lists shared controls whose parent control or group differs.
	Moved []Move `json:"moved,omitempty"`
	// Changed lists shared controls whose own content (title, params,
	// props, links, parts; child controls excluded) differs after
	// normalization.
	Changed []string `json:"changed,omitempty"`
	// GroupsOnlyInA / GroupsOnlyInB list group ids present in one catalog.
	GroupsOnlyInA []string `json:"groups-only-in-a,omitempty"`
	GroupsOnlyInB []string `json:"groups-only-in-b,omitempty"`
	// ChangedGroups lists shared groups whose own content differs.
	ChangedGroups []string `json:"changed-groups,omitempty"`
	// ResourcesOnlyInA / ResourcesOnlyInB list back-matter resource uuids.
	ResourcesOnlyInA []string `json:"resources-only-in-a,omitempty"`
	ResourcesOnlyInB []string `json:"resources-only-in-b,omitempty"`
}

// Move records a control placed differently in the two catalogs.
type Move struct {
	ID string `json:"id"`
	A  string `json:"a"`
	B  string `json:"b"`
}

// Options tunes the comparison.
type Options struct {
	// ExactWhitespace disables whitespace normalization of string values.
	// By default runs of whitespace collapse to one space and strings are
	// trimmed, because XML-based resolvers routinely reflow prose.
	ExactWhitespace bool
}

// Identical reports whether the catalogs have the same controls, order,
// placement, content, groups and resources.
func (r *Report) Identical() bool {
	return len(r.OnlyInA) == 0 && len(r.OnlyInB) == 0 && !r.OrderDiffers &&
		len(r.Moved) == 0 && len(r.Changed) == 0 &&
		len(r.GroupsOnlyInA) == 0 && len(r.GroupsOnlyInB) == 0 && len(r.ChangedGroups) == 0 &&
		len(r.ResourcesOnlyInA) == 0 && len(r.ResourcesOnlyInB) == 0
}

// SameControlSet reports whether both catalogs contain exactly the same
// control ids.
func (r *Report) SameControlSet() bool {
	return len(r.OnlyInA) == 0 && len(r.OnlyInB) == 0
}

type entry struct {
	ctl       *oscal.Control
	placement string
}

func index(c *oscal.Catalog) ([]string, map[string]entry) {
	var order []string
	idx := map[string]entry{}
	c.WalkControls(func(ref oscal.ControlRef) bool {
		id := ref.Control.ID
		if _, dup := idx[id]; dup {
			return true
		}
		place := "catalog"
		if len(ref.GroupPath) > 0 {
			place = "group " + strings.Join(ref.GroupPath, "/")
		}
		if ref.ParentID != "" {
			place = "control " + ref.ParentID
		}
		order = append(order, id)
		idx[id] = entry{ctl: ref.Control, placement: place}
		return true
	})
	return order, idx
}

// Compare diffs catalog a against catalog b.
func Compare(a, b *oscal.Catalog, opts Options) (*Report, error) {
	orderA, idxA := index(a)
	orderB, idxB := index(b)
	r := &Report{ControlsA: len(orderA), ControlsB: len(orderB)}
	var sharedA, sharedB []string
	for _, id := range orderA {
		if _, ok := idxB[id]; ok {
			sharedA = append(sharedA, id)
		} else {
			r.OnlyInA = append(r.OnlyInA, id)
		}
	}
	for _, id := range orderB {
		if _, ok := idxA[id]; ok {
			sharedB = append(sharedB, id)
		} else {
			r.OnlyInB = append(r.OnlyInB, id)
		}
	}
	r.OrderDiffers = !slices.Equal(sharedA, sharedB)
	for _, id := range sharedA {
		ea, eb := idxA[id], idxB[id]
		if ea.placement != eb.placement {
			r.Moved = append(r.Moved, Move{ID: id, A: ea.placement, B: eb.placement})
		}
		same, err := sameContent(ea.ctl.CloneShallow(), eb.ctl.CloneShallow(), opts)
		if err != nil {
			return nil, fmt.Errorf("catdiff: control %s: %w", id, err)
		}
		if !same {
			r.Changed = append(r.Changed, id)
		}
	}
	if err := compareGroups(r, a, b, opts); err != nil {
		return nil, err
	}
	r.ResourcesOnlyInA, r.ResourcesOnlyInB = diffIDs(resourceIDs(a), resourceIDs(b))
	return r, nil
}

func compareGroups(r *Report, a, b *oscal.Catalog, opts Options) error {
	ga, gb := groupIndex(a.Groups), groupIndex(b.Groups)
	idsA, idsB := sortedKeys(ga), sortedKeys(gb)
	r.GroupsOnlyInA, r.GroupsOnlyInB = diffIDs(idsA, idsB)
	for _, id := range idsA {
		g2, ok := gb[id]
		if !ok {
			continue
		}
		same, err := sameContent(ga[id].Clone(), g2.Clone(), opts)
		if err != nil {
			return fmt.Errorf("catdiff: group %s: %w", id, err)
		}
		if !same {
			r.ChangedGroups = append(r.ChangedGroups, id)
		}
	}
	return nil
}

func groupIndex(gs []oscal.Group) map[string]oscal.Group {
	out := map[string]oscal.Group{}
	var walk func(gs []oscal.Group)
	walk = func(gs []oscal.Group) {
		for _, g := range gs {
			if g.ID != "" {
				if _, dup := out[g.ID]; !dup {
					out[g.ID] = g
				}
			}
			walk(g.Groups)
		}
	}
	walk(gs)
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func resourceIDs(c *oscal.Catalog) []string {
	if c.BackMatter == nil {
		return nil
	}
	ids := make([]string, 0, len(c.BackMatter.Resources))
	for _, r := range c.BackMatter.Resources {
		ids = append(ids, r.UUID)
	}
	return ids
}

func diffIDs(a, b []string) (onlyA, onlyB []string) {
	inA := map[string]bool{}
	inB := map[string]bool{}
	for _, x := range a {
		inA[x] = true
	}
	for _, x := range b {
		inB[x] = true
	}
	for _, x := range a {
		if !inB[x] {
			onlyA = append(onlyA, x)
		}
	}
	for _, x := range b {
		if !inA[x] {
			onlyB = append(onlyB, x)
		}
	}
	return onlyA, onlyB
}

// sameContent compares two values by their JSON form, optionally with
// whitespace in every string normalized.
func sameContent(a, b any, opts Options) (bool, error) {
	na, err := normalized(a, opts)
	if err != nil {
		return false, err
	}
	nb, err := normalized(b, opts)
	if err != nil {
		return false, err
	}
	return na == nb, nil
}

func normalized(v any, opts Options) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if opts.ExactWhitespace {
		return string(data), nil
	}
	var generic any
	if err := json.Unmarshal(data, &generic); err != nil {
		return "", err
	}
	out, err := json.Marshal(normalizeValue(generic))
	return string(out), err
}

func normalizeValue(v any) any {
	switch x := v.(type) {
	case string:
		return strings.Join(strings.FieldsFunc(x, unicode.IsSpace), " ")
	case []any:
		for i := range x {
			x[i] = normalizeValue(x[i])
		}
		return x
	case map[string]any:
		for k := range x {
			x[k] = normalizeValue(x[k])
		}
		return x
	}
	return v
}
