package oscal

import (
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const yamlProfile = `
profile:
  uuid: 47dba700-ff2a-4e28-9c4a-ac0e1b37fa12
  metadata:
    title: YAML profile
    last-modified: 2024-01-01T00:00:00Z
    version: "1.0"
    oscal-version: 1.1.2
  imports:
    - href: catalog.json
      include-controls:
        - with-ids: [ac-1, ac-2]
          with-child-controls: yes
  merge:
    as-is: true
  modify:
    set-parameters:
      - param-id: ac-1_prm_1
        values: ["3"]
      - param-id: ac-1_prm_2
        label: ~
`

func TestDecodeYAMLKeepsTimestampsAsStrings(t *testing.T) {
	doc, err := Decode([]byte(yamlProfile), "p.yaml")
	if err != nil {
		t.Fatal(err)
	}
	p := doc.Profile
	if doc.Model() != ModelProfile || p.Metadata.LastModified != "2024-01-01T00:00:00Z" {
		t.Fatalf("timestamp must survive as text: %+v", p.Metadata)
	}
	if p.Imports[0].IncludeControls[0].WithChildControls != "yes" {
		t.Fatalf("yes must stay a string token, got %q", p.Imports[0].IncludeControls[0].WithChildControls)
	}
	if p.Merge.AsIs == nil || !*p.Merge.AsIs {
		t.Fatal("as-is should decode as boolean true")
	}
	if !bytes.Contains(doc.JSON, []byte(`"oscal-version":"1.1.2"`)) {
		t.Fatalf("version must stay textual: %s", doc.JSON)
	}
}

func TestDecodeErrors(t *testing.T) {
	cases := map[string]string{
		"not json":      `{nope`,
		"no model":      `{"foo": {}}`,
		"two models":    `{"catalog": {}, "profile": {}}`,
		"array":         `[1,2]`,
		"bad yaml":      "a: [1,\n",
		"type mismatch": `{"catalog": {"uuid": 5}}`,
		"empty yaml":    "   \n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(in), ""); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	if _, err := DetectModel([]byte(`{"x":1}`)); !errors.Is(err, ErrUnknownModel) {
		t.Fatalf("want ErrUnknownModel, got %v", err)
	}
	if _, err := ReadFile("does-not-exist.json"); err == nil {
		t.Fatal("missing file")
	}
	if _, err := Normalize([]byte{0x1f, 0x8b, 0, 0}, "x.gz"); err == nil {
		t.Fatal("corrupt gzip must fail")
	}
}

func TestGzipAndSchemaKeyAndBOM(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte(`{"$schema":"x","catalog":{"uuid":"u","metadata":{"title":"t","last-modified":"x","version":"1","oscal-version":"1.1.2"}}}`))
	_ = zw.Close()
	doc, err := Decode(buf.Bytes(), "c.json.gz")
	if err != nil || doc.Catalog == nil || doc.Catalog.Metadata.Title != "t" {
		t.Fatalf("gzip decode: %v", err)
	}
	if !isYAML([]byte("\xef\xbb\xbfcatalog: {}"), "") || isYAML([]byte(" {"), "") {
		t.Fatal("content sniffing")
	}
}

func TestWriteFileAndWrap(t *testing.T) {
	dir := t.TempDir()
	for _, v := range []any{&Catalog{}, &Profile{}, &ComponentDefinition{}, &SSP{}} {
		w, err := Wrap(v)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "out.json")
		if err := WriteFile(path, w); err != nil {
			t.Fatal(err)
		}
		doc, err := ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if doc.Source != path || doc.Model() == "" {
			t.Fatalf("round trip lost model for %T", v)
		}
	}
	if _, err := Wrap(42); err == nil {
		t.Fatal("wrap of non-model must fail")
	}
	if err := WriteFile(filepath.Join(dir, "missing", "x.json"), Document{}); err == nil {
		t.Fatal("unwritable path must fail")
	}
	data, _ := Marshal(Document{Catalog: &Catalog{Metadata: Metadata{Title: "<b>&</b>"}}})
	if !strings.Contains(string(data), "<b>&</b>") {
		t.Fatal("HTML must not be escaped")
	}
	if (&Document{}).Model() != "" {
		t.Fatal("empty document has no model")
	}
}

func TestUUIDGenerators(t *testing.T) {
	v4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	v5 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	r := RandomUUIDs{}
	a, b := r.New("x"), r.New("x")
	if !v4.MatchString(a) || a == b {
		t.Fatalf("random uuids: %s %s", a, b)
	}
	d := DeterministicUUIDs{Seed: "s"}
	k1, k2, other := d.New("k"), d.New("k"), d.New("k2")
	if k1 != k2 || k1 == other || !v5.MatchString(k1) {
		t.Fatal("deterministic uuids must be stable, distinct per key and v5")
	}
	if (DeterministicUUIDs{Seed: "other"}).New("k") == d.New("k") {
		t.Fatal("seed must matter")
	}
}

func testCatalog() *Catalog {
	return &Catalog{
		Params:   []Parameter{{ID: "loose"}},
		Controls: []Control{{ID: "x-1", Title: "X1", Props: []Property{{Name: "label", Class: "zero", Value: "X-01"}, {Name: "label", Value: "X-1"}}}},
		Groups: []Group{{ID: "g", Params: []Parameter{{ID: "gp"}}, Controls: []Control{
			{ID: "g-1", Params: []Parameter{{ID: "g-1_p"}}, Parts: []Part{{Name: "statement", ID: "g-1_smt"}},
				Controls: []Control{{ID: "g-1.1", Props: []Property{{Name: "status", Value: "withdrawn"}}}}},
		}, Groups: []Group{{ID: "sub", Controls: []Control{{ID: "s-1", Props: []Property{{Name: "label", Class: "only", Value: "S1"}}}}}}}},
	}
}

func TestWalkAndIndex(t *testing.T) {
	c := testCatalog()
	if got := strings.Join(c.ControlIDs(), ","); got != "x-1,g-1,g-1.1,s-1" {
		t.Fatalf("order = %s", got)
	}
	idx := c.ControlIndex()
	if idx["g-1.1"].ParentID != "g-1" || strings.Join(idx["s-1"].GroupPath, "/") != "g/sub" || idx["g-1.1"].Order != 2 {
		t.Fatalf("index = %+v", idx["g-1.1"])
	}
	if _, ok := c.Control("nope"); ok {
		t.Fatal("unknown control")
	}
	n := 0
	c.WalkControls(func(ControlRef) bool { n++; return n < 2 })
	if n != 2 {
		t.Fatalf("walk must stop early, visited %d", n)
	}
	ps := c.AllParams()
	for _, id := range []string{"loose", "gp", "g-1_p"} {
		if ps[id] == nil {
			t.Fatalf("missing param %s", id)
		}
	}
	x, _ := c.Control("x-1")
	s, _ := c.Control("s-1")
	w, _ := c.Control("g-1.1")
	g1, _ := c.Control("g-1")
	if x.Label() != "X-1" || s.Label() != "S1" || w.Label() != "g-1.1" || !w.IsWithdrawn() || x.IsWithdrawn() {
		t.Fatal("labels / withdrawn")
	}
	if st, ok := g1.StatementPart(); !ok || st.ID != "g-1_smt" {
		t.Fatal("statement part")
	}
	if _, ok := x.StatementPart(); ok {
		t.Fatal("x-1 has no statement")
	}
	if _, ok := (*BackMatter)(nil).Resource("x"); ok {
		t.Fatal("nil back-matter")
	}
}

func TestCloneIsDeep(t *testing.T) {
	orig := Control{ID: "a", Params: []Parameter{{ID: "p", Values: []string{"v"}, Select: &ParameterSelection{Choice: []string{"c"}},
		Constraints: []ParameterConstraint{{Tests: []ConstraintTest{{Expression: "e"}}}}}},
		Parts:    []Part{{Name: "s", Parts: []Part{{Name: "i", Props: []Property{{Name: "n", Value: "v"}}}}}},
		Controls: []Control{{ID: "a.1"}}}
	c := orig.Clone()
	c.Params[0].Values[0] = "changed"
	c.Params[0].Select.Choice[0] = "changed"
	c.Params[0].Constraints[0].Tests[0].Expression = "changed"
	c.Parts[0].Parts[0].Props[0].Value = "changed"
	c.Controls[0].ID = "changed"
	if orig.Params[0].Values[0] != "v" || orig.Params[0].Select.Choice[0] != "c" || orig.Params[0].Constraints[0].Tests[0].Expression != "e" ||
		orig.Parts[0].Parts[0].Props[0].Value != "v" || orig.Controls[0].ID != "a.1" {
		t.Fatal("clone shares memory with the original")
	}
	if len(orig.CloneShallow().Controls) != 0 {
		t.Fatal("shallow clone drops children")
	}
	g := Group{ID: "g", Controls: []Control{{ID: "x"}}, Groups: []Group{{ID: "y"}}}.Clone()
	if g.Controls != nil || g.Groups != nil {
		t.Fatal("group clone drops nested content")
	}
}

func TestReadFileLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	if err := os.WriteFile(path, []byte(`{"catalog":{"uuid":"u","metadata":{"title":"t","last-modified":"x","version":"1","oscal-version":"1"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(path); err != nil {
		t.Fatal(err)
	}
}

func TestYAMLAliasBomb(t *testing.T) {
	var b strings.Builder
	b.WriteString("a0: &a0 [x, x, x, x, x, x, x, x, x, x]\n")
	for i := 1; i < 10; i++ {
		prev := "*a" + string(rune('0'+i-1))
		b.WriteString("a" + string(rune('0'+i)) + ": &a" + string(rune('0'+i)) + " [" + strings.Repeat(prev+", ", 9) + prev + "]\n")
	}
	if _, err := Normalize([]byte(b.String()), "bomb.yaml"); err == nil {
		t.Fatal("alias bomb must be rejected")
	}
}
