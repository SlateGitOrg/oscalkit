package schema

import (
	"strings"
	"sync"
	"testing"
)

const minimalCatalog = `{"catalog":{"uuid":"47dba700-ff2a-4e28-9c4a-ac0e1b37fa12","metadata":{"title":"t","last-modified":"2024-01-01T00:00:00Z","version":"1","oscal-version":"1.2.3"},
 "groups":[{"id":"g","title":"G","controls":[{"id":"g-1","title":"C"}]}]}}`

func TestValidDocument(t *testing.T) {
	vs, err := Validate(Catalog, []byte(minimalCatalog))
	if err != nil || len(vs) != 0 {
		t.Fatalf("expected valid, got %v %v", vs, err)
	}
}

func TestViolationsArePointed(t *testing.T) {
	bad := strings.Replace(minimalCatalog, `"title":"C"`, `"title":"C","bogus":1`, 1)
	bad = strings.Replace(bad, `"id":"g-1"`, `"id":"g 1"`, 1)
	bad = strings.Replace(bad, `"uuid":"47dba700-ff2a-4e28-9c4a-ac0e1b37fa12"`, `"uuid":"not-a-uuid"`, 1)
	vs, err := Validate(Catalog, []byte(bad))
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, v := range vs {
		joined += v.String() + "\n"
	}
	for _, want := range []string{"/catalog/uuid", "/catalog/groups/0/controls/0", "bogus", "/catalog/groups/0/controls/0/id"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
}

func TestRootViolation(t *testing.T) {
	vs, err := Validate(Profile, []byte(`{"profile":{}}`))
	if err != nil || len(vs) == 0 {
		t.Fatalf("empty profile must fail: %v %v", vs, err)
	}
	if (Violation{Message: "m"}).String() != "/: m" {
		t.Fatal("root pointer formatting")
	}
	if pointer([]string{"a/b", "c~d"}) != "/a~1b/c~0d" {
		t.Fatal("pointer escaping")
	}
}

// TestCombineMethodPatch documents the repair of the upstream OSCAL 1.2.3
// profile schema, which omits merge/combine's "method" flag.
func TestCombineMethodPatch(t *testing.T) {
	doc := func(method string) []byte {
		return []byte(`{"profile":{"uuid":"47dba700-ff2a-4e28-9c4a-ac0e1b37fa12","metadata":{"title":"x","last-modified":"2024-01-01T00:00:00Z","version":"1","oscal-version":"1.2.3"},
			"imports":[{"href":"a.json","include-all":{}}],"merge":{"combine":{"method":"` + method + `"},"as-is":true}}}`)
	}
	if vs, err := Validate(Profile, doc("use-first")); err != nil || len(vs) != 0 {
		t.Fatalf("use-first must be valid: %v %v", vs, err)
	}
	if vs, _ := Validate(Profile, doc("bogus")); len(vs) == 0 {
		t.Fatal("an unknown method must still be rejected")
	}
	patchCombineMethod("not a map") // must not panic
}

func TestErrors(t *testing.T) {
	if _, err := Validate(Model("poam"), []byte(`{}`)); err == nil {
		t.Fatal("unsupported model")
	}
	if _, err := Validate(Catalog, []byte(`{nope`)); err == nil {
		t.Fatal("bad json")
	}
}

func TestAllModelsCompileConcurrently(t *testing.T) {
	v := New()
	var wg sync.WaitGroup
	for _, m := range Models {
		for range 3 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := v.Validate(m, []byte(`{}`)); err != nil {
					t.Errorf("%s: %v", m, err)
				}
			}()
		}
	}
	wg.Wait()
}
