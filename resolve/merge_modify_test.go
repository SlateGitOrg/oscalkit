package resolve

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/archit2501/oscalkit/oscal"
)

func TestCombineUseFirstVersusKeep(t *testing.T) {
	imports := []oscal.Import{
		{Href: "abc_catalog.json", IncludeControls: withIDs("a1", "a3")},
		{Href: "abc_catalog.json", IncludeControls: withIDs("a1", "a2")},
	}
	p := newProfile(imports...)
	p.Merge = &oscal.Merge{Combine: &oscal.Combine{Method: "use-first"}, Flat: &oscal.Flat{}}
	res := resolveWith(t, p, fixture(t, nil))
	// Spec example: use-first keeps the first a1 and appends a2.
	assertIDs(t, ids(res.Catalog), "a1", "a3", "a2")
	if hasWarning(res, "duplicate-control") {
		t.Fatal("use-first must not leave duplicates")
	}

	p.Merge = &oscal.Merge{Combine: &oscal.Combine{Method: "keep"}}
	res = resolveWith(t, p, fixture(t, nil))
	assertIDs(t, ids(res.Catalog), "a1", "a3", "a1", "a2")
	if !hasWarning(res, "duplicate-control") {
		t.Fatal("keep with clashing ids must warn")
	}
}

func TestAsIsMergesGroupsAcrossImports(t *testing.T) {
	p := newProfile(
		oscal.Import{Href: "abc_catalog.json", IncludeControls: withIDs("a1", "b1")},
		oscal.Import{Href: "abc_catalog.json", IncludeControls: withIDs("a2", "a1")},
	)
	p.Merge = asIs()
	p.Merge.Combine = &oscal.Combine{Method: "use-first"}
	res := resolveWith(t, p, fixture(t, nil))
	var groups []string
	for _, g := range res.Catalog.Groups {
		groups = append(groups, g.ID)
	}
	assertIDs(t, groups, "a", "b", "z")
	assertIDs(t, ids(res.Catalog), "a1", "a2", "b1")
}

func TestMergeDirectiveErrors(t *testing.T) {
	yes := true
	cases := map[string]*oscal.Merge{
		"two structures":   {Flat: &oscal.Flat{}, AsIs: &yes},
		"deprecated merge": {Combine: &oscal.Combine{Method: "merge"}},
		"unknown method":   {Combine: &oscal.Combine{Method: "zip"}},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeAll: &oscal.IncludeAll{}})
			p.Merge = m
			opts := fixedOptions()
			opts.Fetcher = fixture(t, nil)
			if _, err := Resolve(context.Background(), p, profURI, opts); !errors.Is(err, ErrMergeDirective) {
				t.Fatalf("want ErrMergeDirective, got %v", err)
			}
		})
	}
}

func TestAsIsFalseIsFlat(t *testing.T) {
	no := false
	p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeControls: withIDs("c3", "c3.a")})
	p.Merge = &oscal.Merge{AsIs: &no}
	res := resolveWith(t, p, fixture(t, nil))
	if len(res.Catalog.Groups) != 0 || len(res.Catalog.Controls) != 2 {
		t.Fatalf("as-is=false should flatten: %+v", res.Catalog.Controls)
	}
}

func TestCustomStructure(t *testing.T) {
	p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeAll: &oscal.IncludeAll{}})
	p.Merge = &oscal.Merge{Custom: &oscal.Custom{
		Groups: []oscal.CustomGroup{
			{ID: "c", Title: "Custom C", Props: []oscal.Property{{Name: "custom", Value: "yes"}},
				InsertControls: []oscal.InsertControls{{Order: "descending", IncludeControls: []oscal.SelectControlByID{{WithChildControls: "yes", WithIDs: []string{"c3"}}}}}},
			{ID: "mixed", Title: "Mixed", Groups: []oscal.CustomGroup{
				{Title: "Inner", InsertControls: []oscal.InsertControls{{Order: "ascending", IncludeControls: []oscal.SelectControlByID{{Matching: []oscal.Matching{{Pattern: "?2"}}}}}}},
			}},
			{Title: "Nothing", InsertControls: []oscal.InsertControls{{IncludeControls: withIDs("nope")}}},
		},
		InsertControls: []oscal.InsertControls{{IncludeControls: withIDs("b1", "a1")}},
	}}
	res := resolveWith(t, p, fixture(t, nil))
	cat := res.Catalog
	// Root-level inserts keep pool (document) order.
	if len(cat.Controls) != 2 || cat.Controls[0].ID != "a1" || cat.Controls[1].ID != "b1" {
		t.Fatalf("root inserts = %+v", cat.Controls)
	}
	c := cat.Groups[0]
	var cids []string
	for _, x := range c.Controls {
		cids = append(cids, x.ID)
		if len(x.Controls) != 0 {
			t.Fatalf("custom inserts must not nest")
		}
	}
	assertIDs(t, cids, "c3.a-1", "c3.a", "c3")
	if c.Title != "Custom C" || len(c.Parts) != 1 || len(c.Props) != 1 {
		t.Fatalf("custom group should keep its own title/props and inherit source parts: %+v", c)
	}
	inner := cat.Groups[1].Groups[0]
	assertIDs(t, []string{inner.Controls[0].ID, inner.Controls[1].ID, inner.Controls[2].ID}, "a2", "b2", "c2")
	if !hasWarning(res, "empty-insert") {
		t.Fatal("an insert that selects nothing should warn")
	}
	// Loose params survive pruning only when referenced (b1 uses loose_used).
	var params []string
	for _, prm := range cat.Params {
		params = append(params, prm.ID)
	}
	assertIDs(t, params, "loose_used", "loose_keep")
}

// TestModifyAddsSpecExample reproduces the "modify-adds" example of the
// NIST profile resolution specification (expected output translated from
// the spec's XML to JSON).
func TestModifyAddsSpecExample(t *testing.T) {
	fedramp := "https://fedramp.gov/ns/oscal"
	p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeControls: withIDs("a1", "a3")})
	p.Merge = asIs()
	p.Merge.Combine = &oscal.Combine{Method: "keep"}
	p.Modify = &oscal.Modify{
		SetParameters: []oscal.ProfileSetParameter{
			{ParamID: "a1_prm1", Constraints: []oscal.ParameterConstraint{{Description: "at least every 3 years"}}},
			{ParamID: "a3_prm1", Constraints: []oscal.ParameterConstraint{{Description: "at least annually"}}},
		},
		Alters: []oscal.Alter{{ControlID: "a1", Adds: []oscal.Add{
			{Position: "starting", ByID: "a1", Props: []oscal.Property{{Name: "CORE", NS: fedramp, Value: "core"}}},
			{Position: "starting", ByID: "a1_prm1", Props: []oscal.Property{{Name: "annotated", NS: fedramp, Value: "param-prop"}}},
			{Position: "before", ByID: "a1_prm1", Params: []oscal.Parameter{{ID: "new_prm", Props: []oscal.Property{{Name: "annotated", NS: fedramp, Value: "new-param"}}, Label: "A1 Parameter New"}}},
			{Position: "starting", ByID: "a1-stmt", Props: []oscal.Property{{Name: "conformity", NS: fedramp, Value: "assessment-objective"}, {Name: "method", Class: "fedramp", Value: "EXAMINE"}}},
			{Position: "after", ByID: "a1-stmt", Parts: []oscal.Part{{Name: "statement", ID: "new-part", Prose: "A1 bbbbb {{ insert: param, new_prm }} bbbbb"}}},
			{Position: "ending", ByID: "a1-stmt", Parts: []oscal.Part{{Name: "statement", ID: "new-part2", Prose: "A1 ccc {{ insert: param, new_prm }} ccc"}}},
		}}},
	}
	res := resolveWith(t, p, fixture(t, nil))
	got, _ := res.Catalog.Control("a1")
	want := oscal.Control{
		ID: "a1", Title: "Control a1",
		Params: []oscal.Parameter{
			{ID: "new_prm", Props: []oscal.Property{{Name: "annotated", NS: fedramp, Value: "new-param"}}, Label: "A1 Parameter New"},
			{ID: "a1_prm1", Props: []oscal.Property{{Name: "annotated", NS: fedramp, Value: "param-prop"}}, Label: "A1 Parameter 1",
				Constraints: []oscal.ParameterConstraint{{Description: "at least every 3 years"}}},
		},
		Props: []oscal.Property{{Name: "CORE", NS: fedramp, Value: "core"}, {Name: "label", Value: "a1"}},
		Links: []oscal.Link{{Href: "#" + resUsed, Rel: "reference"}},
		Parts: []oscal.Part{
			{ID: "a1-stmt", Name: "statement",
				Props: []oscal.Property{{Name: "conformity", NS: fedramp, Value: "assessment-objective"}, {Name: "method", Class: "fedramp", Value: "EXAMINE"}},
				Prose: "A1 aaaaa {{ insert: param, a1_prm1 }} aaaaaaaaaa",
				Parts: []oscal.Part{{Name: "statement", ID: "new-part2", Prose: "A1 ccc {{ insert: param, new_prm }} ccc"}}},
			{Name: "statement", ID: "new-part", Prose: "A1 bbbbb {{ insert: param, new_prm }} bbbbb"},
		},
	}
	gj, _ := json.Marshal(got)
	wj, _ := json.Marshal(want)
	if string(gj) != string(wj) {
		t.Fatalf("a1 mismatch\n got: %s\nwant: %s", gj, wj)
	}
	a3, _ := res.Catalog.Control("a3")
	if len(a3.Params[0].Constraints) != 1 || a3.Params[0].Constraints[0].Description != "at least annually" {
		t.Fatalf("a3 constraint not set: %+v", a3.Params[0])
	}
}

func TestSetParameterSemantics(t *testing.T) {
	p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeControls: withIDs("a1", "a3")})
	p.Modify = &oscal.Modify{SetParameters: []oscal.ProfileSetParameter{
		{ParamID: "a1_prm1", Label: "new label", Class: "k", Usage: "u", DependsOn: "d", Values: []string{"v1"},
			Props: []oscal.Property{{Name: "p", UUID: "dddddddd-0000-4000-8000-000000000001", Value: "1"}},
			Links: []oscal.Link{{Href: "#x"}}, Guidelines: []oscal.ParameterGuideline{{Prose: "g"}}},
		{ParamID: "a1_prm1", Values: []string{"v2"},
			Props: []oscal.Property{{Name: "p", UUID: "dddddddd-0000-4000-8000-000000000001", Value: "2"}},
			Links: []oscal.Link{{Href: "#x"}}},
		{ParamID: "a3_prm1", Values: []string{"x"}},
		{ParamID: "a1_prm1", Select: &oscal.ParameterSelection{Choice: []string{"s"}}},
		{ParamID: "missing", Values: []string{"z"}},
	}}
	res := resolveWith(t, p, fixture(t, nil))
	a1, _ := res.Catalog.Control("a1")
	prm := a1.Params[0]
	if prm.Label != "new label" || prm.Class != "k" || prm.Usage != "u" || prm.DependsOn != "d" {
		t.Fatalf("scalar members not replaced: %+v", prm)
	}
	if prm.Values != nil || prm.Select == nil || prm.Select.Choice[0] != "s" {
		t.Fatalf("select must replace values: %+v", prm)
	}
	if len(prm.Props) != 1 || prm.Props[0].Value != "2" || len(prm.Links) != 1 || len(prm.Guidelines) != 1 {
		t.Fatalf("props by uuid replaced, identical links deduplicated: %+v", prm)
	}
	a3, _ := res.Catalog.Control("a3")
	if a3.Params[0].Select != nil || a3.Params[0].Values[0] != "x" {
		t.Fatalf("values must replace select: %+v", a3.Params[0])
	}
	if !hasWarning(res, "unknown-param") {
		t.Fatal("unknown param must warn")
	}
}

func TestAlterRemoves(t *testing.T) {
	cat := abcCatalog()
	c1 := &cat.Groups[2].Controls[0]
	c1.Props = append(c1.Props,
		oscal.Property{Name: "status", NS: "https://example.com/ns", Value: "x", Class: "k1"},
		oscal.Property{Name: "status", Value: "y"},
	)
	c1.Parts = append(c1.Parts, oscal.Part{ID: "c1-guid", Name: "guidance", Class: "k1", Prose: "g",
		Parts: []oscal.Part{{ID: "c1-guid-sub", Name: "item", Props: []oscal.Property{{Name: "status", Value: "z"}}}}})
	c1.Links = []oscal.Link{{Href: "#a1", Rel: "related"}, {Href: "#a2", Rel: "reference"}}
	c1.Params = []oscal.Parameter{{ID: "c1_prm", Class: "k1"}}
	f := MapFetcher{catURI: encode(t, cat)}
	p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeControls: withIDs("c1")})
	p.Modify = &oscal.Modify{Alters: []oscal.Alter{{ControlID: "c1", Removes: []oscal.Remove{
		{ByName: "status", ByNS: "https://example.com/ns"},
		{ByName: "status", ByNS: defaultNS, ByItemName: "prop"},
		{ByID: "c1-guid-sub"},
		{ByItemName: "link", ByName: "related"},
		{ByClass: "k1", ByItemName: "param"},
		{ByName: "nothing-matches"},
		{},
	}}, {ControlID: "zz"}}}
	res := resolveWith(t, p, f)
	got, _ := res.Catalog.Control("c1")
	if len(got.Props) != 1 || got.Props[0].Name != "label" {
		t.Fatalf("props after remove: %+v", got.Props)
	}
	if len(got.Parts) != 2 || len(got.Parts[1].Parts) != 0 {
		t.Fatalf("nested part should be removed by id: %+v", got.Parts)
	}
	if len(got.Links) != 1 || got.Links[0].Rel != "reference" || len(got.Params) != 0 {
		t.Fatalf("links/params after remove: %+v %+v", got.Links, got.Params)
	}
	for _, code := range []string{"inoperative-remove", "empty-remove", "unknown-control"} {
		if !hasWarning(res, code) {
			t.Errorf("missing %s warning in %v", code, res.Warnings)
		}
	}
}

func TestAlterAddsVariants(t *testing.T) {
	p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeControls: []oscal.SelectControlByID{{WithChildControls: "yes", WithIDs: []string{"c3", "a1"}}}})
	p.Merge = asIs()
	prop := func(v string) []oscal.Property { return []oscal.Property{{Name: "added", Value: v}} }
	p.Modify = &oscal.Modify{Alters: []oscal.Alter{
		{ControlID: "c3", Adds: []oscal.Add{
			{Title: "Renamed C3", Props: prop("implicit-ending")},
			{Position: "before", Props: prop("implicit-before")},
			{Position: "ending", ByID: "c3.a", Props: prop("child-control")},
			{Position: "after", ByID: "c3.a", Parts: []oscal.Part{{Name: "note", Prose: "beside child"}}},
			{Position: "before", ByID: "c3-stmt", Parts: []oscal.Part{{Name: "intro", ID: "c3-intro"}}, Params: []oscal.Parameter{{ID: "c3_new"}}},
			{Position: "ending", ByID: "c3-stmt", Title: "Statement title", Params: []oscal.Parameter{{ID: "bad"}}, Links: []oscal.Link{{Href: "#a1"}}},
			{Position: "after", ByID: "c3-intro", Parts: []oscal.Part{{Name: "x", ID: "c3-x"}}},
			{ByID: "missing", Props: prop("never")},
			{Position: "sideways", ByID: "c3-stmt"},
		}},
		{ControlID: "a1", Adds: []oscal.Add{
			{Position: "ending", ByID: "a1_prm1", Props: prop("on-param"), Parts: []oscal.Part{{Name: "bad"}}},
			{Position: "after", ByID: "a1_prm1", Params: []oscal.Parameter{{ID: "a1_after"}}, Props: prop("after-param"), Title: "ignored"},
		}},
	}}
	// c3.a-stmt is nested in a child control; add a nested-part target too.
	cat := abcCatalog()
	c3 := &cat.Groups[2].Controls[2]
	c3.Parts[0].Parts = []oscal.Part{{ID: "c3-stmt.a", Name: "item"}}
	p.Modify.Alters[0].Adds = append(p.Modify.Alters[0].Adds,
		oscal.Add{Position: "before", ByID: "c3-stmt.a", Parts: []oscal.Part{{Name: "item", ID: "c3-stmt.0"}}, Props: prop("nested"), Params: []oscal.Parameter{{ID: "bad2"}}},
		oscal.Add{Position: "starting", ByID: "c3-stmt.a", Props: prop("in-nested")},
	)
	res := resolveWith(t, p, MapFetcher{catURI: encode(t, cat)})
	got, _ := res.Catalog.Control("c3")
	if got.Title != "Renamed C3" {
		t.Errorf("title not replaced: %q", got.Title)
	}
	wantProps := []string{"implicit-before", "label", "implicit-ending"}
	var props []string
	for _, pr := range got.Props {
		if pr.Name == "added" {
			props = append(props, pr.Value)
		} else {
			props = append(props, pr.Name)
		}
	}
	assertIDs(t, props, wantProps...)
	var parts []string
	for _, pt := range got.Parts {
		parts = append(parts, pt.Name)
	}
	assertIDs(t, parts, "intro", "x", "statement", "note")
	st := got.Parts[2]
	if st.Title != "Statement title" || len(st.Links) != 1 || len(st.Parts) != 2 || st.Parts[0].ID != "c3-stmt.0" || st.Props[0].Value != "nested" {
		t.Errorf("statement part = %+v", st)
	}
	if st.Parts[1].Props[0].Value != "in-nested" {
		t.Errorf("starting inside nested part failed: %+v", st.Parts[1])
	}
	if got.Params[0].ID != "c3_new" {
		t.Errorf("param added before statement should go to control params: %+v", got.Params)
	}
	child := got.Controls[0]
	if child.Props[len(child.Props)-1].Value != "child-control" {
		t.Errorf("add into child control failed: %+v", child.Props)
	}
	a1, _ := res.Catalog.Control("a1")
	if a1.Params[0].Props[0].Value != "on-param" || a1.Params[1].ID != "a1_after" || a1.Props[len(a1.Props)-1].Value != "after-param" {
		t.Errorf("param-targeted adds wrong: %+v", a1)
	}
	for _, code := range []string{"inoperative-add", "bad-position", "invalid-add"} {
		if !hasWarning(res, code) {
			t.Errorf("missing %s warning: %v", code, res.Warnings)
		}
	}
}

func TestMetadataAndBackMatter(t *testing.T) {
	cat := abcCatalog()
	cat.Metadata.OSCALVersion = "1.0.4"
	cat.Metadata.Roles = []oscal.Role{{ID: "keeper", Title: "K", Props: []oscal.Property{{Name: "keep", Value: "always"}}}, {ID: "dropped", Title: "D"}}
	cat.Metadata.Parties = []oscal.Party{{UUID: "eeeeeeee-0000-4000-8000-000000000001", Type: "organization", Name: "Kept", Props: []oscal.Property{{Name: "keep", Value: "always"}}}}
	p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeControls: withIDs("a1")})
	p.Metadata.OSCALVersion = "1.1.2"
	p.BackMatter = &oscal.BackMatter{Resources: []oscal.Resource{
		{UUID: resUsed, Title: "profile version of used"},
		{UUID: resKeep, Title: "profile copy without keep"},
		{UUID: "ffffffff-0000-4000-8000-000000000001", Title: "unreferenced profile resource"},
	}}
	res := resolveWith(t, p, MapFetcher{catURI: encode(t, cat)})
	out := res.Catalog
	if out.Metadata.OSCALVersion != "1.1.2" || out.Metadata.LastModified != "2026-01-02T03:04:05Z" {
		t.Errorf("metadata version/timestamp: %+v", out.Metadata)
	}
	if v, _ := oscal.Prop(out.Metadata.Props, "source-profile"); v != "profile.json" {
		t.Errorf("source-profile prop = %q", v)
	}
	if len(out.Metadata.Roles) != 1 || out.Metadata.Roles[0].ID != "keeper" || len(out.Metadata.Parties) != 1 {
		t.Errorf("keep=always roles/parties not copied: %+v", out.Metadata)
	}
	var titles []string
	for _, r := range out.BackMatter.Resources {
		titles = append(titles, r.Title)
	}
	// used: replaced by the profile's copy; keep: the catalog's keep=always
	// copy wins; unreferenced resources are pruned.
	assertIDs(t, titles, "kept", "profile version of used")
	if out.UUID != (oscal.DeterministicUUIDs{Seed: "test"}).New("resolved-catalog:"+profURI) {
		t.Errorf("uuid should come from the injected generator")
	}
}

func TestOSCALVersionRules(t *testing.T) {
	cases := []struct {
		in   []string
		want string
		err  bool
	}{
		{[]string{"1.0.0", "1.1.2"}, "1.1.2", false},
		{[]string{"1.0.0", "1.9.0"}, ToolOSCALVersion, false},
		{[]string{"1.0.0-rc2", "garbage"}, "1.0.0", false},
		{[]string{"x"}, ToolOSCALVersion, false},
		{[]string{"1.1.2", "2.0.0"}, "", true},
		{[]string{"v1.2.0"}, "1.2.0", false},
	}
	for _, c := range cases {
		got, err := resolveOSCALVersion(c.in)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("resolveOSCALVersion(%v) = %q, %v; want %q, err=%v", c.in, got, err, c.want, c.err)
		}
	}
}

func TestDefaultsAreApplied(t *testing.T) {
	r := newResolver(Options{})
	if r.opts.Fetcher == nil || r.opts.UUIDs == nil || r.opts.Now == nil || r.opts.Logger == nil || r.opts.ToolName != "oscalkit" || r.opts.MaxDepth != DefaultMaxDepth {
		t.Fatalf("defaults not applied: %+v", r.opts)
	}
	if time.Since(r.opts.Now()) > time.Minute {
		t.Fatal("default clock")
	}
}
