package resolve

import (
	"context"
	"encoding/base64"
	"slices"
	"testing"

	"github.com/archit2501/oscalkit/oscal"
)

const (
	base      = "file:///fx/"
	catURI    = base + "abc_catalog.json"
	profURI   = base + "profile.json"
	resUsed   = "11111111-1111-4111-8111-111111111111"
	resUnused = "22222222-2222-4222-8222-222222222222"
	resKeep   = "33333333-3333-4333-8333-333333333333"
	catalogID = "44444444-4444-4444-8444-444444444444"
)

func md(title string) oscal.Metadata {
	return oscal.Metadata{Title: title, LastModified: "2024-01-01T00:00:00Z", Version: "1.0", OSCALVersion: "1.1.2"}
}

func ctl(id string, children ...oscal.Control) oscal.Control {
	return oscal.Control{
		ID:       id,
		Title:    "Control " + id,
		Props:    []oscal.Property{{Name: "label", Value: id}},
		Parts:    []oscal.Part{{ID: id + "-stmt", Name: "statement", Prose: "Statement of " + id + "."}},
		Controls: children,
	}
}

// abcCatalog mirrors the "abc" catalog used by the NIST profile-resolution
// specification examples: three groups, nested controls under c3, params
// and back-matter for pruning tests.
func abcCatalog() *oscal.Catalog {
	a1 := ctl("a1")
	a1.Params = []oscal.Parameter{{ID: "a1_prm1", Label: "A1 Parameter 1"}}
	a1.Parts[0].Prose = "A1 aaaaa {{ insert: param, a1_prm1 }} aaaaaaaaaa"
	a1.Links = []oscal.Link{{Href: "#" + resUsed, Rel: "reference"}}
	a3 := ctl("a3")
	a3.Params = []oscal.Parameter{{ID: "a3_prm1", Label: "A3 Parameter 1", Select: &oscal.ParameterSelection{Choice: []string{"x", "y"}}}}
	a3.Parts[0].Prose = "A3 aaaaa {{ insert: param, a3_prm1 }} aaaaaaaaaa"
	b1 := ctl("b1")
	b1.Parts[0].Prose = "B1 uses {{ insert: param, loose_used }}."
	return &oscal.Catalog{
		UUID:     catalogID,
		Metadata: md("Alphabet Catalog"),
		Params: []oscal.Parameter{
			{ID: "loose_used", Label: "used loose param"},
			{ID: "loose_unused", Label: "unused loose param"},
			{ID: "loose_keep", Label: "kept", Props: []oscal.Property{{Name: "keep", Value: "always"}}},
		},
		Groups: []oscal.Group{
			{ID: "a", Title: "Group A of C", Controls: []oscal.Control{a1, ctl("a2"), a3}},
			{ID: "b", Title: "Group B of C", Controls: []oscal.Control{b1, ctl("b2"), ctl("b3")}},
			{ID: "c", Title: "Group C of C", Parts: []oscal.Part{{Name: "overview", Prose: "Group C"}},
				Controls: []oscal.Control{ctl("c1"), ctl("c2"), ctl("c3", ctl("c3.a", ctl("c3.a-1")))}},
			{ID: "z", Title: "Kept group", Props: []oscal.Property{{Name: "keep", Value: "always"}}},
		},
		BackMatter: &oscal.BackMatter{Resources: []oscal.Resource{
			{UUID: resUsed, Title: "used"},
			{UUID: resUnused, Title: "unused"},
			{UUID: resKeep, Title: "kept", Props: []oscal.Property{{Name: "keep", Value: "always"}}},
		}},
	}
}

func encode(t *testing.T, v any) []byte {
	t.Helper()
	w, err := oscal.Wrap(v)
	if err != nil {
		t.Fatal(err)
	}
	b, err := oscal.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newProfile(imports ...oscal.Import) *oscal.Profile {
	return &oscal.Profile{UUID: "55555555-5555-4555-8555-555555555555", Metadata: md("Test Profile"), Imports: imports}
}

func withIDs(ids ...string) []oscal.SelectControlByID {
	return []oscal.SelectControlByID{{WithIDs: ids}}
}

func asIs() *oscal.Merge {
	t := true
	return &oscal.Merge{AsIs: &t}
}

// fixture builds a fetcher serving the abc catalog plus extra documents.
func fixture(t *testing.T, extra map[string]any) MapFetcher {
	t.Helper()
	m := MapFetcher{catURI: encode(t, abcCatalog())}
	for k, v := range extra {
		m[k] = encode(t, v)
	}
	return m
}

func resolveWith(t *testing.T, p *oscal.Profile, f Fetcher, mutate ...func(*Options)) *Result {
	t.Helper()
	opts := fixedOptions()
	opts.Fetcher = f
	for _, m := range mutate {
		m(&opts)
	}
	res, err := Resolve(context.Background(), p, profURI, opts)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return res
}

func ids(c *oscal.Catalog) []string { return c.ControlIDs() }

func hasWarning(res *Result, code string) bool {
	return slices.ContainsFunc(res.Warnings, func(d Diagnostic) bool { return d.Code == code })
}

func b64(t *testing.T, v any) string {
	return base64.StdEncoding.EncodeToString(encode(t, v))
}

func assertIDs(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("controls = %v, want %v", got, want)
	}
}
