package resolve

import (
	"fmt"
	"slices"
	"strings"

	"github.com/SlateGitOrg/oscalkit/oscal"
)

type structuring int

const (
	structFlat structuring = iota
	structAsIs
	structCustom
)

// mergeMode validates the merge directive and returns the combine method
// and structuring mode it selects (defaults: keep + flat).
func mergeMode(m *oscal.Merge) (string, structuring, error) {
	method := "keep"
	mode := structFlat
	if m == nil {
		return method, mode, nil
	}
	if m.Combine != nil && m.Combine.Method != "" {
		method = m.Combine.Method
	}
	switch method {
	case "keep", "use-first":
	case "merge":
		return "", 0, fmt.Errorf("%w: combine method \"merge\" is deprecated and its behaviour is undefined; use \"use-first\" or \"keep\"", ErrMergeDirective)
	default:
		return "", 0, fmt.Errorf("%w: unknown combine method %q", ErrMergeDirective, method)
	}
	n := 0
	if m.Flat != nil {
		n++
		mode = structFlat
	}
	if m.AsIs != nil {
		n++
		if *m.AsIs {
			mode = structAsIs
		}
	}
	if m.Custom != nil {
		n++
		mode = structCustom
	}
	if n > 1 {
		return "", 0, fmt.Errorf("%w: more than one structuring directive (flat, as-is, custom)", ErrMergeDirective)
	}
	return method, mode, nil
}

// merge runs the merge phase, producing the structure of the target catalog
// (controls, groups and loose params; metadata and back-matter come later).
func (r *resolver) merge(m *oscal.Merge, sets []importedSet) (*oscal.Catalog, error) {
	method, mode, err := mergeMode(m)
	if err != nil {
		return nil, err
	}
	if method == "use-first" {
		applyUseFirst(sets)
	}
	out := &oscal.Catalog{}
	switch mode {
	case structAsIs:
		for _, s := range sets {
			out.Params = append(out.Params, cloneParams(s.catalog.Params)...)
			out.Controls = append(out.Controls, asIsControls(s.catalog.Controls, s.included)...)
			out.Groups = mergeGroups(out.Groups, asIsGroups(s.catalog.Groups, s.included))
		}
	case structFlat:
		for _, s := range sets {
			out.Params = append(out.Params, looseParams(s.catalog)...)
			out.Controls = append(out.Controls, flatControls(s.catalog, s.included)...)
		}
	case structCustom:
		r.custom(out, m.Custom, sets)
	}
	return out, nil
}

// applyUseFirst removes, from every import's selection, controls already
// selected by an earlier import (top-down, depth-first import order).
func applyUseFirst(sets []importedSet) {
	seen := map[string]bool{}
	for i := range sets {
		var mine []string
		sets[i].catalog.WalkControls(func(ref oscal.ControlRef) bool {
			id := ref.Control.ID
			if sets[i].included[id] {
				mine = append(mine, id)
			}
			return true
		})
		filtered := make(map[string]bool, len(mine))
		for _, id := range mine {
			if seen[id] {
				continue
			}
			filtered[id] = true
			seen[id] = true
		}
		sets[i].included = filtered
	}
}

// asIsControls reproduces the source nesting for included controls. A
// control whose parent is not included is "up-leveled" into the position of
// that parent.
func asIsControls(cs []oscal.Control, inc map[string]bool) []oscal.Control {
	var out []oscal.Control
	for i := range cs {
		children := asIsControls(cs[i].Controls, inc)
		if inc[cs[i].ID] {
			c := cs[i].CloneShallow()
			c.Controls = children
			out = append(out, c)
			continue
		}
		out = append(out, children...)
	}
	return out
}

// asIsGroups keeps every group that (transitively) holds an included control
// or is marked keep=always, with its non-control content intact.
func asIsGroups(gs []oscal.Group, inc map[string]bool) []oscal.Group {
	var out []oscal.Group
	for i := range gs {
		g := gs[i].Clone()
		g.Controls = asIsControls(gs[i].Controls, inc)
		g.Groups = asIsGroups(gs[i].Groups, inc)
		if len(g.Controls) > 0 || len(g.Groups) > 0 || keepAlways(g.Props) {
			out = append(out, g)
		}
	}
	return out
}

// mergeGroups appends groups into acc, merging groups that share an id so
// that several imports from the same catalog do not duplicate families.
func mergeGroups(acc, add []oscal.Group) []oscal.Group {
	for _, g := range add {
		idx := -1
		if g.ID != "" {
			idx = slices.IndexFunc(acc, func(x oscal.Group) bool { return x.ID == g.ID })
		}
		if idx < 0 {
			acc = append(acc, g)
			continue
		}
		acc[idx].Controls = append(acc[idx].Controls, g.Controls...)
		acc[idx].Groups = mergeGroups(acc[idx].Groups, g.Groups)
	}
	return acc
}

// flatControls lists included controls in document order without nesting.
func flatControls(cat *oscal.Catalog, inc map[string]bool) []oscal.Control {
	var out []oscal.Control
	cat.WalkControls(func(ref oscal.ControlRef) bool {
		if inc[ref.Control.ID] {
			out = append(out, ref.Control.CloneShallow())
		}
		return true
	})
	return out
}

// looseParams returns the catalog-level params plus the params of every
// group (group params become loose once groups are discarded).
func looseParams(cat *oscal.Catalog) []oscal.Parameter {
	out := cloneParams(cat.Params)
	var walk func(gs []oscal.Group)
	walk = func(gs []oscal.Group) {
		for _, g := range gs {
			out = append(out, cloneParams(g.Params)...)
			walk(g.Groups)
		}
	}
	walk(cat.Groups)
	return out
}

func cloneParams(ps []oscal.Parameter) []oscal.Parameter {
	if len(ps) == 0 {
		return nil
	}
	out := make([]oscal.Parameter, len(ps))
	for i := range ps {
		out[i] = ps[i].Clone()
	}
	return out
}

// nearestIncludedAncestor returns the closest ancestor of id that is itself
// included, or "".
func nearestIncludedAncestor(h *hierarchy, id string, inc map[string]bool) string {
	for p := h.parent[id]; p != ""; p = h.parent[p] {
		if inc[p] {
			return p
		}
	}
	return ""
}

type customCtx struct {
	pool       []oscal.Control // included controls available to insert-controls
	h          *hierarchy      // hierarchy over the pool's controls only
	srcGroups  map[string]oscal.Group
	usedParams map[string]bool
}

// custom builds the target structure declared by a custom merge directive.
func (r *resolver) custom(out *oscal.Catalog, c *oscal.Custom, sets []importedSet) {
	ctx := &customCtx{srcGroups: map[string]oscal.Group{}, usedParams: map[string]bool{}}
	ctx.h = &hierarchy{exists: map[string]bool{}, parent: map[string]string{}, children: map[string][]string{}}
	for _, s := range sets {
		src := newHierarchy(s.catalog)
		for _, ctl := range flatControls(s.catalog, s.included) {
			ctx.pool = append(ctx.pool, ctl)
			ctx.h.add(ctl.ID, nearestIncludedAncestor(src, ctl.ID, s.included))
		}
		indexSourceGroups(s.catalog.Groups, s.included, ctx.srcGroups)
	}
	out.Groups = r.customGroups(c.Groups, ctx)
	for _, ic := range c.InsertControls {
		out.Controls = append(out.Controls, r.insertControls(ic, ctx)...)
	}
	for _, s := range sets {
		for _, p := range looseParams(s.catalog) {
			if !ctx.usedParams[p.ID] {
				out.Params = append(out.Params, p)
			}
		}
	}
}

// indexSourceGroups records source groups that hold included controls, so
// custom groups with the same id can inherit their content.
func indexSourceGroups(gs []oscal.Group, inc map[string]bool, into map[string]oscal.Group) bool {
	found := false
	for _, g := range gs {
		has := false
		for _, c := range g.Controls {
			if subtreeIncluded(c, inc) {
				has = true
				break
			}
		}
		if indexSourceGroups(g.Groups, inc, into) {
			has = true
		}
		if has && g.ID != "" {
			if _, dup := into[g.ID]; !dup {
				into[g.ID] = g
			}
		}
		found = found || has
	}
	return found
}

func subtreeIncluded(c oscal.Control, inc map[string]bool) bool {
	if inc[c.ID] {
		return true
	}
	for _, ch := range c.Controls {
		if subtreeIncluded(ch, inc) {
			return true
		}
	}
	return false
}

func (r *resolver) customGroups(gs []oscal.CustomGroup, ctx *customCtx) []oscal.Group {
	var out []oscal.Group
	for _, cg := range gs {
		g := oscal.Group{
			ID:     cg.ID,
			Class:  cg.Class,
			Title:  cg.Title,
			Params: cloneParams(cg.Params),
			Props:  slices.Clone(cg.Props),
			Links:  slices.Clone(cg.Links),
			Parts:  slices.Clone(cg.Parts),
		}
		if src, ok := ctx.srcGroups[cg.ID]; ok {
			inheritGroupContent(&g, src)
		}
		for _, p := range g.Params {
			ctx.usedParams[p.ID] = true
		}
		g.Groups = r.customGroups(cg.Groups, ctx)
		for _, ic := range cg.InsertControls {
			g.Controls = append(g.Controls, r.insertControls(ic, ctx)...)
		}
		out = append(out, g)
	}
	return out
}

// inheritGroupContent copies a matching source group's params, props, links
// and parts into a custom group, after any content the profile declared and
// skipping params/parts whose id is already present.
func inheritGroupContent(g *oscal.Group, src oscal.Group) {
	have := map[string]bool{}
	for _, p := range g.Params {
		have["param:"+p.ID] = true
	}
	for _, p := range g.Parts {
		if p.ID != "" {
			have["part:"+p.ID] = true
		}
	}
	for _, p := range src.Params {
		if !have["param:"+p.ID] {
			g.Params = append(g.Params, p.Clone())
		}
	}
	for _, p := range src.Props {
		if !slices.Contains(g.Props, p) {
			g.Props = append(g.Props, p)
		}
	}
	for _, l := range src.Links {
		if !slices.Contains(g.Links, l) {
			g.Links = append(g.Links, l)
		}
	}
	for _, p := range src.Parts {
		if p.ID == "" || !have["part:"+p.ID] {
			g.Parts = append(g.Parts, p.Clone())
		}
	}
}

// insertControls selects controls from the pool for one insert-controls
// directive. Selected controls are emitted without nesting, in pool order
// ("keep") or sorted by id.
func (r *resolver) insertControls(ic oscal.InsertControls, ctx *customCtx) []oscal.Control {
	var picked []oscal.Control
	selected := r.selectSet(ctx.h, ic.IncludeAll != nil, ic.IncludeControls, ic.ExcludeControls, false, "custom insert-controls")
	for _, c := range ctx.pool {
		if selected[c.ID] {
			picked = append(picked, c.Clone())
		}
	}
	switch ic.Order {
	case "ascending":
		slices.SortStableFunc(picked, func(a, b oscal.Control) int { return strings.Compare(a.ID, b.ID) })
	case "descending":
		slices.SortStableFunc(picked, func(a, b oscal.Control) int { return strings.Compare(b.ID, a.ID) })
	}
	if len(picked) == 0 {
		r.warn("empty-insert", "an insert-controls directive selected no included controls")
	}
	return picked
}
