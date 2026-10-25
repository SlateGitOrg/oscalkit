package catdiff

import (
	"strings"
	"testing"

	"github.com/SlateGitOrg/oscalkit/oscal"
)

func base() *oscal.Catalog {
	return &oscal.Catalog{
		Groups: []oscal.Group{
			{ID: "a", Title: "A", Controls: []oscal.Control{
				{ID: "a-1", Title: "One", Parts: []oscal.Part{{Name: "statement", Prose: "Do  the\nthing."}},
					Controls: []oscal.Control{{ID: "a-1.1", Title: "Enh"}}},
				{ID: "a-2", Title: "Two"},
			}},
			{ID: "b", Title: "B", Groups: []oscal.Group{{ID: "b.x", Title: "BX", Controls: []oscal.Control{{ID: "b-1", Title: "B1"}}}}},
		},
		BackMatter: &oscal.BackMatter{Resources: []oscal.Resource{{UUID: "r1"}, {UUID: "r2"}}},
	}
}

func TestIdenticalModuloWhitespace(t *testing.T) {
	a, b := base(), base()
	b.Groups[0].Controls[0].Parts[0].Prose = " Do the thing. "
	r, err := Compare(a, b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Identical() || !r.SameControlSet() || r.ControlsA != 4 {
		t.Fatalf("expected identical: %+v", r)
	}
	r, _ = Compare(a, b, Options{ExactWhitespace: true})
	if r.Identical() || strings.Join(r.Changed, ",") != "a-1" {
		t.Fatalf("exact mode must see the whitespace change: %+v", r)
	}
}

func TestDifferences(t *testing.T) {
	a, b := base(), base()
	// b: drop a-2, add c-1, move a-1.1 to top of group a, swap order, change b-1, retitle group b, drop r2, add r3
	b.Groups[0].Controls = []oscal.Control{
		{ID: "a-1.1", Title: "Enh"},
		{ID: "a-1", Title: "One", Parts: a.Groups[0].Controls[0].Parts},
	}
	b.Groups[1].Title = "B renamed"
	b.Groups[1].Groups[0].Controls[0].Title = "B1 changed"
	b.Groups = append(b.Groups, oscal.Group{ID: "c", Title: "C", Controls: []oscal.Control{{ID: "c-1", Title: "C1"}}})
	b.BackMatter.Resources = []oscal.Resource{{UUID: "r1"}, {UUID: "r3"}}
	a.Groups = append(a.Groups, oscal.Group{ID: "only-a", Title: "OA"})
	r, err := Compare(a, b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, got []string, want string) {
		t.Helper()
		if strings.Join(got, ",") != want {
			t.Errorf("%s = %v, want %s", name, got, want)
		}
	}
	check("only A", r.OnlyInA, "a-2")
	check("only B", r.OnlyInB, "c-1")
	check("changed", r.Changed, "b-1")
	check("groups only A", r.GroupsOnlyInA, "only-a")
	check("groups only B", r.GroupsOnlyInB, "c")
	check("changed groups", r.ChangedGroups, "b")
	check("resources only A", r.ResourcesOnlyInA, "r2")
	check("resources only B", r.ResourcesOnlyInB, "r3")
	if !r.OrderDiffers || len(r.Moved) != 1 || r.Moved[0].ID != "a-1.1" || r.Moved[0].A != "control a-1" || r.Moved[0].B != "group a" {
		t.Errorf("order/moves: %+v %+v", r.OrderDiffers, r.Moved)
	}
	if r.Identical() || r.SameControlSet() {
		t.Error("catalogs differ")
	}
}

func TestTopLevelAndDuplicates(t *testing.T) {
	a := &oscal.Catalog{Controls: []oscal.Control{{ID: "x", Title: "X"}, {ID: "x", Title: "dup"}}}
	b := &oscal.Catalog{Controls: []oscal.Control{{ID: "x", Title: "X"}}}
	r, err := Compare(a, b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Identical() || r.ControlsA != 1 {
		t.Fatalf("first occurrence wins for duplicate ids: %+v", r)
	}
}
