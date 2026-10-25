package param

import (
	"strings"
	"testing"

	"github.com/SlateGitOrg/oscalkit/oscal"
)

func TestFindInserts(t *testing.T) {
	text := "a {{ insert: param, p-1 }} b {{insert:param,p.2}} c {{ insert: other, x }} {{ not an insert }}"
	ins := FindInserts(text)
	if len(ins) != 3 || ins[0].IDRef != "p-1" || ins[1].IDRef != "p.2" || ins[2].Type != "other" {
		t.Fatalf("inserts = %+v", ins)
	}
	if got := Refs(text); strings.Join(got, ",") != "p-1,p.2" {
		t.Fatalf("refs = %v", got)
	}
	if FindInserts("plain") != nil {
		t.Fatal("no inserts")
	}
}

func testCatalog() *oscal.Catalog {
	return &oscal.Catalog{Controls: []oscal.Control{{
		ID: "x-1", Title: "X",
		Params: []oscal.Parameter{
			{ID: "x-1_label", Label: "frequency"},
			{ID: "x-1_valued", Values: []string{"daily", "weekly"}},
			{ID: "x-1_sel", Select: &oscal.ParameterSelection{HowMany: "one-or-more", Choice: []string{"a", "b {{ insert: param, x-1_label }}"}}},
			{ID: "x-1_one", Select: &oscal.ParameterSelection{Choice: []string{"only"}}},
			{ID: "x-1_agg", Label: "organization-defined things", Props: []oscal.Property{{Name: "aggregates", Value: "x-1_label"}, {Name: "aggregates", Value: "x-1_valued"}}},
			{ID: "x-1_loop", Label: "{{ insert: param, x-1_loop }}"},
			{ID: "x-1_nolabel"},
			{ID: "x-01_odp.09", Label: "new id", Props: []oscal.Property{{Name: "alt-identifier", Value: "x-1_prm_9"}}},
		},
		Parts: []oscal.Part{{Name: "statement", ID: "x-1_smt", Prose: "Do {{ insert: param, x-1_agg }}:", Parts: []oscal.Part{
			{Name: "item", ID: "x-1_smt.a", Props: []oscal.Property{{Name: "label", Value: "a."}}, Prose: "every {{ insert: param, x-1_label }}", Parts: []oscal.Part{
				{Name: "item", ID: "x-1_smt.a.1", Props: []oscal.Property{{Name: "label", Value: "1."}}, Prose: "pick {{ insert: param, x-1_sel }} {{ insert: param, x-1_one }}"},
			}},
			{Name: "guidance", Prose: "ignored"},
			{Name: "item", ID: "x-1_smt.b", Prose: "{{ insert: param, nope }} {{ insert: param, x-1_nolabel }}"},
		}}},
	}}}
}

func TestTextRules(t *testing.T) {
	r := NewRenderer(testCatalog())
	cases := map[string]string{
		"x-1_label":   "[Assignment: frequency]",
		"x-1_valued":  "daily, weekly",
		"x-1_sel":     "[Selection (one or more): a; b [Assignment: frequency]]",
		"x-1_one":     "[Selection: only]",
		"x-1_agg":     "[Assignment: organization-defined things]",
		"x-1_nolabel": "[Assignment: x-1_nolabel]",
		"missing":     "[Unknown parameter: missing]",
	}
	for id, want := range cases {
		if got := r.Text(id); got != want {
			t.Errorf("Text(%s) = %q, want %q", id, got, want)
		}
	}
	if got := r.Text("x-1_loop"); !strings.Contains(got, "[...]") {
		t.Errorf("self-referential label must terminate, got %q", got)
	}
	r.SetValues("x-1_label", []string{"monthly"})
	if got := r.Text("x-1_agg"); got != "monthly; daily, weekly" {
		t.Errorf("aggregate with all parts set = %q", got)
	}
	if !r.IsSet("x-1_agg") || r.IsSet("x-1_sel") || r.IsSet("missing") {
		t.Error("IsSet")
	}
	r.SetValues("x-1_sel", []string{"b {{ insert: param, x-1_label }}"})
	if got := r.Text("x-1_sel"); got != "b monthly" {
		t.Errorf("value containing an insert = %q", got)
	}
}

func TestAliases(t *testing.T) {
	r := NewRenderer(testCatalog())
	if c, ok := r.Canonical("x-1_prm_9"); !ok || c != "x-01_odp.09" {
		t.Fatalf("alias = %q %v", c, ok)
	}
	if _, ok := r.Canonical("zzz"); ok {
		t.Fatal("unknown alias")
	}
	r.SetValues("x-1_prm_9", []string{"v"})
	if strings.Join(r.Values("x-01_odp.09"), "") != "v" {
		t.Fatal("override via alias")
	}
	if _, ok := r.Param("zzz"); ok {
		t.Fatal("unknown param")
	}
}

func TestStatementAndMarkdown(t *testing.T) {
	cat := testCatalog()
	r := NewRenderer(cat)
	ctl := &cat.Controls[0]
	lines := r.Statement(ctl)
	if len(lines) != 4 || lines[1].Label != "a." || lines[2].Depth != 2 || lines[3].PartID != "x-1_smt.b" {
		t.Fatalf("lines = %+v", lines)
	}
	md := FormatMarkdown(lines)
	want := "Do [Assignment: organization-defined things]:\n- a. every [Assignment: frequency]\n  - 1. pick [Selection (one or more): a; b [Assignment: frequency]] [Selection: only]\n- [Unknown parameter: nope] [Assignment: x-1_nolabel]\n"
	if md != want {
		t.Fatalf("markdown:\n%s\nwant:\n%s", md, want)
	}
	refs := StatementRefs(ctl)
	if strings.Join(refs, ",") != "x-1_agg,x-1_label,x-1_sel,x-1_one,nope,x-1_nolabel" {
		t.Fatalf("refs = %v", refs)
	}
	empty := &oscal.Control{ID: "e"}
	if r.Statement(empty) != nil || StatementRefs(empty) != nil {
		t.Fatal("no statement")
	}
	if r.Render("no inserts") != "no inserts" {
		t.Fatal("render passthrough")
	}
	if r.Render("{{ insert: other, x }}") != "{{ insert: other, x }}" {
		t.Fatal("non-param inserts are kept")
	}
}

// FuzzRender checks rendering never panics and is the identity on text
// without param inserts.
func FuzzRender(f *testing.F) {
	f.Add("a {{ insert: param, x-1_sel }} b")
	f.Add("{{ insert: param, x-1_loop }}")
	f.Add("{{")
	r := NewRenderer(testCatalog())
	f.Fuzz(func(t *testing.T, s string) {
		out := r.Render(s)
		if len(Refs(s)) == 0 && out != s {
			t.Fatalf("render changed text without param inserts: %q -> %q", s, out)
		}
	})
}
