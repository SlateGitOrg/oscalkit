package resolve

import (
	"github.com/SlateGitOrg/oscalkit/oscal"
)

// hierarchy is the parent/child structure of the controls in one catalog.
type hierarchy struct {
	order    []string            // control ids in document order (first occurrence)
	exists   map[string]bool     // id -> present
	parent   map[string]string   // id -> parent control id ("" if none)
	children map[string][]string // id -> direct child ids
}

func newHierarchy(cat *oscal.Catalog) *hierarchy {
	h := &hierarchy{
		exists:   map[string]bool{},
		parent:   map[string]string{},
		children: map[string][]string{},
	}
	cat.WalkControls(func(ref oscal.ControlRef) bool {
		h.add(ref.Control.ID, ref.ParentID)
		return true
	})
	return h
}

// add registers a control under parent ("" for none), ignoring repeats.
func (h *hierarchy) add(id, parent string) {
	if h.exists[id] {
		return
	}
	h.exists[id] = true
	h.order = append(h.order, id)
	h.parent[id] = parent
	if parent != "" {
		h.children[parent] = append(h.children[parent], id)
	}
}

func (h *hierarchy) addDescendants(id string, into map[string]bool) {
	for _, c := range h.children[id] {
		if !into[c] {
			into[c] = true
			h.addDescendants(c, into)
		}
	}
}

func (h *hierarchy) addAncestors(id string, into map[string]bool) {
	for p := h.parent[id]; p != ""; p = h.parent[p] {
		into[p] = true
	}
}

// matchSelector returns the ids selected by one include/exclude directive
// (without descendant expansion), warning about selectors that match nothing.
func (r *resolver) matchSelector(h *hierarchy, sel oscal.SelectControlByID, where string) []string {
	var ids []string
	for _, id := range sel.WithIDs {
		if h.exists[id] {
			ids = append(ids, id)
		} else {
			r.warn("unknown-control", "%s: with-id %q matches no control", where, id)
		}
	}
	for _, m := range sel.Matching {
		if m.Pattern == "" {
			r.warn("empty-pattern", "%s: matching directive without a pattern matches nothing", where)
			continue
		}
		n := 0
		for _, id := range h.order {
			if globMatch(m.Pattern, id) {
				ids = append(ids, id)
				n++
			}
		}
		if n == 0 {
			r.warn("unmatched-pattern", "%s: pattern %q matches no control", where, m.Pattern)
		}
	}
	return ids
}

// selectSet evaluates include-all / include-controls / exclude-controls
// against a hierarchy. With withAncestors, inclusion also pulls in the
// control's ancestors; with with-child-controls=yes it pulls in
// descendants. Exclusion always wins.
func (r *resolver) selectSet(h *hierarchy, includeAll bool, include, exclude []oscal.SelectControlByID, withAncestors bool, where string) map[string]bool {
	inc := map[string]bool{}
	if includeAll {
		for _, id := range h.order {
			inc[id] = true
		}
	}
	for _, sel := range include {
		for _, id := range r.matchSelector(h, sel, where+" include-controls") {
			inc[id] = true
			if withAncestors {
				h.addAncestors(id, inc)
			}
			if sel.WithChildControls == "yes" {
				h.addDescendants(id, inc)
			}
		}
	}
	for _, sel := range exclude {
		for _, id := range r.matchSelector(h, sel, where+" exclude-controls") {
			delete(inc, id)
			if sel.WithChildControls == "yes" {
				drop := map[string]bool{}
				h.addDescendants(id, drop)
				for d := range drop {
					delete(inc, d)
				}
			}
		}
	}
	return inc
}

// selectImport runs the import-phase selection for one import directive.
func (r *resolver) selectImport(cat *oscal.Catalog, imp oscal.Import) map[string]bool {
	h := newHierarchy(cat)
	if imp.IncludeAll == nil && len(imp.IncludeControls) == 0 {
		r.warn("empty-import", "import %q has neither include-all nor include-controls; nothing is imported", imp.Href)
	}
	return r.selectSet(h, imp.IncludeAll != nil, imp.IncludeControls, imp.ExcludeControls, r.opts.IncludeAncestors, "import "+imp.Href)
}
