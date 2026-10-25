package resolve

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SlateGitOrg/oscalkit/oscal"
)

func TestIncludeWithIDsFlatDefault(t *testing.T) {
	p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeControls: withIDs("b1", "a3", "c3.a")})
	res := resolveWith(t, p, fixture(t, nil))
	// No merge directive: flat, document order, no groups, no nesting.
	assertIDs(t, ids(res.Catalog), "a3", "b1", "c3.a")
	if len(res.Catalog.Groups) != 0 {
		t.Fatalf("flat output must have no groups")
	}
	for _, c := range res.Catalog.Controls {
		if len(c.Controls) != 0 {
			t.Fatalf("flat output must not nest controls")
		}
	}
}

func TestWithChildControls(t *testing.T) {
	p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeControls: []oscal.SelectControlByID{
		{WithChildControls: "yes", WithIDs: []string{"a1", "b1", "c1", "c3"}},
	}})
	p.Merge = asIs()
	res := resolveWith(t, p, fixture(t, nil))
	// Mirrors the spec's include-call-with-children example.
	assertIDs(t, ids(res.Catalog), "a1", "b1", "c1", "c3", "c3.a", "c3.a-1")
	c3, _ := res.Catalog.Control("c3")
	if len(c3.Controls) != 1 || c3.Controls[0].ID != "c3.a" || c3.Controls[0].Controls[0].ID != "c3.a-1" {
		t.Fatalf("as-is must keep nesting, got %+v", c3.Controls)
	}
}

func TestMatchingGlob(t *testing.T) {
	p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeControls: []oscal.SelectControlByID{
		{Matching: []oscal.Matching{{Pattern: "a*"}, {Pattern: "c3.?-1"}, {Pattern: "zz*"}, {}}},
	}})
	res := resolveWith(t, p, fixture(t, nil))
	assertIDs(t, ids(res.Catalog), "a1", "a2", "a3", "c3.a-1")
	if !hasWarning(res, "unmatched-pattern") || !hasWarning(res, "empty-pattern") {
		t.Fatalf("expected pattern warnings, got %v", res.Warnings)
	}
}

func TestExcludeWinsAndChildren(t *testing.T) {
	p := newProfile(oscal.Import{
		Href:       "abc_catalog.json",
		IncludeAll: &oscal.IncludeAll{},
		ExcludeControls: []oscal.SelectControlByID{
			{Matching: []oscal.Matching{{Pattern: "b*"}}},
			{WithChildControls: "yes", WithIDs: []string{"c3.a"}},
			{WithIDs: []string{"nope"}},
		},
	})
	res := resolveWith(t, p, fixture(t, nil))
	assertIDs(t, ids(res.Catalog), "a1", "a2", "a3", "c1", "c2", "c3")
	if !hasWarning(res, "unknown-control") {
		t.Fatalf("expected unknown-control warning for 'nope'")
	}
}

func TestAsIsUpLevelsChildOfExcludedParent(t *testing.T) {
	p := newProfile(oscal.Import{
		Href:            "abc_catalog.json",
		IncludeControls: []oscal.SelectControlByID{{WithChildControls: "yes", WithIDs: []string{"c3"}}},
		ExcludeControls: withIDs("c3.a"),
	})
	p.Merge = asIs()
	res := resolveWith(t, p, fixture(t, nil))
	var c *oscal.Group
	for i := range res.Catalog.Groups {
		if res.Catalog.Groups[i].ID == "c" {
			c = &res.Catalog.Groups[i]
		}
	}
	if c == nil {
		t.Fatal("group c missing")
	}
	// c3.a is excluded, so c3.a-1 is up-leveled into c3.a's position.
	if len(c.Controls) != 1 || c.Controls[0].ID != "c3" || len(c.Controls[0].Controls) != 1 || c.Controls[0].Controls[0].ID != "c3.a-1" {
		t.Fatalf("unexpected structure: %+v", c.Controls)
	}
	if len(c.Parts) != 1 {
		t.Fatalf("group non-control content must be kept")
	}
}

func TestAsIsPrunesEmptyGroupsButKeepsKeepAlways(t *testing.T) {
	p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeControls: withIDs("a1")})
	p.Merge = asIs()
	res := resolveWith(t, p, fixture(t, nil))
	var got []string
	for _, g := range res.Catalog.Groups {
		got = append(got, g.ID)
	}
	assertIDs(t, got, "a", "z")
}

func TestEnhancementWithoutParent(t *testing.T) {
	p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeControls: withIDs("c3.a")})
	p.Merge = asIs()
	// Default (NIST reference behaviour): the parent is not pulled in.
	res := resolveWith(t, p, fixture(t, nil))
	assertIDs(t, ids(res.Catalog), "c3.a")
	// Spec-prose behaviour: ancestors are included.
	res = resolveWith(t, p, fixture(t, nil), func(o *Options) { o.IncludeAncestors = true })
	assertIDs(t, ids(res.Catalog), "c3", "c3.a")
}

func TestNestedProfileImport(t *testing.T) {
	inner := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeControls: withIDs("a1", "a2", "b1", "c1")})
	inner.Modify = &oscal.Modify{SetParameters: []oscal.ProfileSetParameter{{ParamID: "a1_prm1", Values: []string{"inner"}}}}
	outer := newProfile(oscal.Import{Href: "inner_profile.json", IncludeAll: &oscal.IncludeAll{}, ExcludeControls: withIDs("b1")})
	res := resolveWith(t, outer, fixture(t, map[string]any{base + "inner_profile.json": inner}))
	assertIDs(t, ids(res.Catalog), "a1", "a2", "c1")
	a1, _ := res.Catalog.Control("a1")
	if got := a1.Params[0].Values; len(got) != 1 || got[0] != "inner" {
		t.Fatalf("inner profile modifications must carry through, got %v", got)
	}
	if len(res.Sources) != 2 {
		t.Fatalf("sources = %v", res.Sources)
	}
}

func TestCircularImports(t *testing.T) {
	a := newProfile(oscal.Import{Href: "b_profile.json", IncludeAll: &oscal.IncludeAll{}})
	b := newProfile(oscal.Import{Href: "profile.json", IncludeAll: &oscal.IncludeAll{}})
	f := fixture(t, map[string]any{base + "b_profile.json": b, profURI: a})
	opts := fixedOptions()
	opts.Fetcher = f
	_, err := Resolve(context.Background(), a, profURI, opts)
	if !errors.Is(err, ErrCircularImport) {
		t.Fatalf("want circular import error, got %v", err)
	}
	if !strings.Contains(err.Error(), "profile.json -> "+base+"b_profile.json -> "+profURI) {
		t.Fatalf("error should show the chain: %v", err)
	}
	self := newProfile(oscal.Import{Href: "profile.json", IncludeAll: &oscal.IncludeAll{}})
	if _, err := Resolve(context.Background(), self, profURI, opts); !errors.Is(err, ErrCircularImport) {
		t.Fatalf("self import must be circular, got %v", err)
	}
}

func TestMaxDepth(t *testing.T) {
	docs := map[string]any{}
	for i := range 5 {
		docs[base+"p"+string(rune('0'+i))+".json"] = newProfile(oscal.Import{Href: "p" + string(rune('1'+i)) + ".json", IncludeAll: &oscal.IncludeAll{}})
	}
	docs[base+"p5.json"] = newProfile(oscal.Import{Href: "abc_catalog.json", IncludeAll: &oscal.IncludeAll{}})
	p := newProfile(oscal.Import{Href: "p0.json", IncludeAll: &oscal.IncludeAll{}})
	opts := fixedOptions()
	opts.Fetcher = fixture(t, docs)
	opts.MaxDepth = 3
	if _, err := Resolve(context.Background(), p, profURI, opts); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("want nesting error, got %v", err)
	}
	opts.MaxDepth = 0
	if _, err := Resolve(context.Background(), p, profURI, opts); err != nil {
		t.Fatalf("default depth should suffice: %v", err)
	}
}

func TestBackMatterImportsRlinkAndBase64(t *testing.T) {
	cat := abcCatalog()
	p := newProfile(
		oscal.Import{Href: "#aaaaaaaa-0000-4000-8000-000000000001", IncludeControls: withIDs("a1")},
		oscal.Import{Href: "#aaaaaaaa-0000-4000-8000-000000000002", IncludeControls: withIDs("b1")},
	)
	p.BackMatter = &oscal.BackMatter{Resources: []oscal.Resource{
		{UUID: "aaaaaaaa-0000-4000-8000-000000000001", RLinks: []oscal.RLink{
			{Href: "abc_catalog.xml", MediaType: "application/oscal.catalog+xml"},
			{Href: "abc_catalog.yaml", MediaType: "application/oscal.catalog+yaml"},
			{Href: "abc_catalog.json", MediaType: "application/oscal.catalog+json"},
		}},
		{UUID: "aaaaaaaa-0000-4000-8000-000000000002", Base64: &oscal.Base64{Filename: "abc.json", Value: b64(t, cat)}},
	}}
	res := resolveWith(t, p, fixture(t, nil))
	assertIDs(t, ids(res.Catalog), "a1", "b1")
}

func TestImportErrors(t *testing.T) {
	cases := map[string]oscal.Import{
		"missing resource": {Href: "#bbbbbbbb-0000-4000-8000-000000000009", IncludeAll: &oscal.IncludeAll{}},
		"missing file":     {Href: "nope.json", IncludeAll: &oscal.IncludeAll{}},
		"empty href":       {Href: "", IncludeAll: &oscal.IncludeAll{}},
		"xml only":         {Href: "#cccccccc-0000-4000-8000-000000000001", IncludeAll: &oscal.IncludeAll{}},
		"bad base64":       {Href: "#cccccccc-0000-4000-8000-000000000002", IncludeAll: &oscal.IncludeAll{}},
		"not a catalog":    {Href: "cd.json", IncludeAll: &oscal.IncludeAll{}},
	}
	cd := &oscal.ComponentDefinition{UUID: catalogID, Metadata: md("cd")}
	for name, imp := range cases {
		t.Run(name, func(t *testing.T) {
			p := newProfile(imp)
			p.BackMatter = &oscal.BackMatter{Resources: []oscal.Resource{
				{UUID: "cccccccc-0000-4000-8000-000000000001", RLinks: []oscal.RLink{{Href: "x.xml"}}},
				{UUID: "cccccccc-0000-4000-8000-000000000002", Base64: &oscal.Base64{Value: "!!!"}},
			}}
			opts := fixedOptions()
			opts.Fetcher = fixture(t, map[string]any{base + "cd.json": cd})
			_, err := Resolve(context.Background(), p, profURI, opts)
			if !errors.Is(err, ErrBadImport) {
				t.Fatalf("want ErrBadImport, got %v", err)
			}
		})
	}
	if _, err := Resolve(context.Background(), newProfile(), profURI, fixedOptions()); !errors.Is(err, ErrNoImports) {
		t.Fatalf("want ErrNoImports, got %v", err)
	}
}

func TestEmptyImportWarns(t *testing.T) {
	p := newProfile(oscal.Import{Href: "abc_catalog.json"})
	res := resolveWith(t, p, fixture(t, nil))
	if len(ids(res.Catalog)) != 0 || !hasWarning(res, "empty-import") {
		t.Fatalf("empty import should select nothing and warn: %v", res.Warnings)
	}
}

func TestContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts := fixedOptions()
	opts.Fetcher = fixture(t, nil)
	p := newProfile(oscal.Import{Href: "abc_catalog.json", IncludeAll: &oscal.IncludeAll{}})
	if _, err := Resolve(ctx, p, profURI, opts); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}
