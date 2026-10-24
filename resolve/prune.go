package resolve

import (
	"encoding/json"

	"github.com/archit2501/oscalkit/oscal"
	"github.com/archit2501/oscalkit/param"
)

// combineBackMatter merges back-matter from the imported sources (in import
// order) and then the source profile. A later resource replaces an earlier
// one with the same uuid (moving to the later position) unless the earlier
// one is marked keep=always and the later one is not.
func combineBackMatter(sources []*oscal.BackMatter) *oscal.BackMatter {
	var out []oscal.Resource
	pos := map[string]int{}
	removed := map[int]bool{}
	for _, bm := range sources {
		if bm == nil {
			continue
		}
		for _, res := range bm.Resources {
			if i, ok := pos[res.UUID]; ok {
				if keepAlways(out[i].Props) && !keepAlways(res.Props) {
					continue
				}
				removed[i] = true
			}
			pos[res.UUID] = len(out)
			out = append(out, res)
		}
	}
	var kept []oscal.Resource
	for i, res := range out {
		if !removed[i] {
			kept = append(kept, res)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return &oscal.BackMatter{Resources: kept}
}

// prune removes unreferenced back-matter resources and loose (catalog-level)
// params. An object survives if it is marked keep=always, if keep lists it
// (params named by set-parameters), or if it is referenced from content
// that itself survives: "#uuid" for resources, insert directives or
// aggregation props for params.
func prune(cat *oscal.Catalog, keep map[string]bool) {
	loose := cat.Params
	resources := []oscal.Resource(nil)
	if cat.BackMatter != nil {
		resources = cat.BackMatter.Resources
	}
	// Scan everything except the prunable objects themselves.
	cat.Params = nil
	bm := cat.BackMatter
	cat.BackMatter = nil
	refs := newRefSet()
	refs.scan(cat)
	cat.BackMatter = bm

	keptParam := make([]bool, len(loose))
	keptRes := make([]bool, len(resources))
	for changed := true; changed; {
		changed = false
		for i, p := range loose {
			if !keptParam[i] && (keep[p.ID] || keepAlways(p.Props) || refs.params[p.ID]) {
				keptParam[i] = true
				refs.scan(p)
				changed = true
			}
		}
		for i, res := range resources {
			if !keptRes[i] && (keepAlways(res.Props) || refs.fragments[res.UUID]) {
				keptRes[i] = true
				refs.scan(res)
				changed = true
			}
		}
	}
	for i, p := range loose {
		if keptParam[i] {
			cat.Params = append(cat.Params, p)
		}
	}
	var res []oscal.Resource
	for i, r := range resources {
		if keptRes[i] {
			res = append(res, r)
		}
	}
	if len(res) == 0 {
		cat.BackMatter = nil
	} else {
		cat.BackMatter = &oscal.BackMatter{Resources: res}
	}
}

// refSet accumulates references found in JSON-encoded content.
type refSet struct {
	fragments map[string]bool // ids referenced as "#id"
	params    map[string]bool // param ids referenced by insert directives or aggregates props
}

func newRefSet() *refSet {
	return &refSet{fragments: map[string]bool{}, params: map[string]bool{}}
}

// scan records every reference in v. Encoding to JSON and scanning the text
// once is O(size) and catches references in any field (links, prose,
// remarks), as the specification's "appears anywhere" rule requires.
func (s *refSet) scan(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	text := string(data)
	for i := 0; i < len(text); i++ {
		if text[i] != '#' {
			continue
		}
		j := i + 1
		for j < len(text) && isTokenChar(text[j]) {
			j++
		}
		if j > i+1 {
			s.fragments[text[i+1:j]] = true
		}
	}
	for _, id := range param.ParamRefs(text) {
		s.params[id] = true
	}
	s.scanAggregates(v)
}

// scanAggregates records params named by "aggregates" props (SP 800-53
// uses these to tie a summary param to its organization-defined parts).
func (s *refSet) scanAggregates(v any) {
	var visit func(ps []oscal.Parameter)
	visit = func(ps []oscal.Parameter) {
		for _, p := range ps {
			for _, pr := range p.Props {
				if pr.Name == "aggregates" {
					s.params[pr.Value] = true
				}
			}
		}
	}
	switch x := v.(type) {
	case *oscal.Catalog:
		x.WalkControls(func(ref oscal.ControlRef) bool {
			visit(ref.Control.Params)
			return true
		})
	case oscal.Parameter:
		visit([]oscal.Parameter{x})
	}
}

func isTokenChar(c byte) bool {
	return c == '-' || c == '_' || c == '.' ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
