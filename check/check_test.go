package check

import (
	"strings"
	"testing"

	"github.com/SlateGitOrg/oscalkit/oscal"
)

const (
	u1 = "11111111-1111-4111-8111-111111111111"
	u2 = "22222222-2222-4222-8222-222222222222"
	u3 = "33333333-3333-4333-8333-333333333333"
)

func decode(t *testing.T, v any) *oscal.Document {
	t.Helper()
	w, err := oscal.Wrap(v)
	if err != nil {
		t.Fatal(err)
	}
	b, err := oscal.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := oscal.Decode(b, "")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func codes(fs []Finding) string {
	var out []string
	for _, f := range fs {
		out = append(out, string(f.Severity)+":"+f.Code)
	}
	return strings.Join(out, ",")
}

func md() oscal.Metadata {
	return oscal.Metadata{Title: "t", LastModified: "x", Version: "1", OSCALVersion: "1.1.2",
		Roles:              []oscal.Role{{ID: "owner", Title: "Owner"}, {ID: "owner", Title: "Dup"}},
		Parties:            []oscal.Party{{UUID: u3, Type: "person"}},
		ResponsibleParties: []oscal.ResponsibleParty{{RoleID: "ghost", PartyUUIDs: []string{u3, u2}}},
	}
}

func TestCatalogChecks(t *testing.T) {
	cat := &oscal.Catalog{UUID: u1, Metadata: md(),
		Params: []oscal.Parameter{{ID: "p", Select: &oscal.ParameterSelection{Choice: []string{"{{ insert: param, ghost-sel }}"}}}},
		Groups: []oscal.Group{{ID: "g", Title: "G", Controls: []oscal.Control{
			{ID: "c-1", Title: "C1", Params: []oscal.Parameter{{ID: "p"}},
				Links: []oscal.Link{{Href: "#c-2"}, {Href: "#" + u2}, {Href: "#missing-control"}, {Href: "#44444444-4444-4444-8444-444444444444"}, {Href: "https://example.com"}},
				Parts: []oscal.Part{{ID: "c-1_smt", Name: "statement", Prose: "uses {{ insert: param, p }} and {{ insert: param, ghost }}"}}},
			{ID: "c-2", Title: "C2", Parts: []oscal.Part{{ID: "c-1_smt", Name: "statement"}}},
		}}, {ID: "g", Title: "dup"}},
		BackMatter: &oscal.BackMatter{Resources: []oscal.Resource{{UUID: u2}, {UUID: u2}}},
	}
	got := codes(Document(decode(t, cat)))
	for _, want := range []string{
		"error:duplicate-uuid", "error:duplicate-id", "error:unknown-param-insert", "error:broken-link",
		"warning:broken-link", "error:duplicate-role", "error:unknown-role", "error:unknown-party",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if strings.Index(got, "warning") < strings.LastIndex(got, "error") {
		t.Errorf("errors must sort before warnings: %s", got)
	}
}

func TestResolvedCatalogToleratesControlLinks(t *testing.T) {
	cat := &oscal.Catalog{UUID: u1, Metadata: oscal.Metadata{Title: "t", LastModified: "x", Version: "1", OSCALVersion: "1",
		Links: []oscal.Link{{Href: "p.json", Rel: "source-profile"}}},
		Controls: []oscal.Control{{ID: "c-1", Title: "C", Links: []oscal.Link{{Href: "#not-selected"}}}}}
	if fs := Document(decode(t, cat)); len(fs) != 0 {
		t.Fatalf("resolved catalog links to unselected controls are fine: %v", fs)
	}
	cat.Metadata.Links = nil
	cat.Metadata.Props = []oscal.Property{{Name: "source-profile", Value: "p"}}
	if fs := Document(decode(t, cat)); len(fs) != 0 {
		t.Fatalf("source-profile prop also marks a resolved catalog: %v", fs)
	}
}

func TestProfileChecks(t *testing.T) {
	yes, no := true, false
	p := &oscal.Profile{UUID: u1, Metadata: oscal.Metadata{Title: "t", LastModified: "x", Version: "1", OSCALVersion: "1"},
		Imports: []oscal.Import{
			{Href: "#" + u2, IncludeControls: []oscal.SelectControlByID{{Matching: []oscal.Matching{{}}}}},
			{Href: "cat.json", IncludeAll: &oscal.IncludeAll{}},
		},
		Merge: &oscal.Merge{Combine: &oscal.Combine{Method: "merge"}, Flat: &oscal.Flat{}, AsIs: &yes},
		Modify: &oscal.Modify{Alters: []oscal.Alter{
			{ControlID: "a", Adds: []oscal.Add{{Position: "middle"}}},
			{ControlID: "a"},
		}},
	}
	got := codes(Document(decode(t, p)))
	for _, want := range []string{"error:broken-import", "warning:empty-pattern", "error:multiple-structuring", "warning:deprecated-combine", "error:bad-position", "warning:repeated-alter"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	p.Merge = &oscal.Merge{AsIs: &no}
	p.Modify = nil
	p.Imports = p.Imports[1:]
	if fs := Document(decode(t, p)); len(fs) != 0 {
		t.Fatalf("clean profile: %v", fs)
	}
}

func TestComponentDefinitionChecks(t *testing.T) {
	cd := &oscal.ComponentDefinition{UUID: u1, Metadata: oscal.Metadata{Title: "t", LastModified: "x", Version: "1", OSCALVersion: "1", Roles: []oscal.Role{{ID: "admin", Title: "A"}}},
		Components: []oscal.DefinedComponent{{UUID: u2, Type: "software", Title: "S", Description: "d",
			ResponsibleRoles: []oscal.ResponsibleRole{{RoleID: "nobody"}},
			ControlImplementations: []oscal.ControlImplementation{
				{UUID: u3, Source: "p.json", Description: "d", ImplementedRequirements: []oscal.ComponentImplementedRequirement{
					{UUID: "44444444-4444-4444-8444-444444444444", ControlID: "ac-2", Description: "d",
						Statements: []oscal.ComponentStatement{{StatementID: "ac-3_smt.a", UUID: "55555555-5555-4555-8555-555555555555", Description: "d", ResponsibleRoles: []oscal.ResponsibleRole{{RoleID: "admin"}}}}},
					{UUID: "66666666-6666-4666-8666-666666666666", ControlID: "ac-2", Description: "again"},
				}},
				{UUID: "77777777-7777-4777-8777-777777777777", Source: "p.json", Description: "empty"},
			}}},
	}
	got := codes(Document(decode(t, cd)))
	for _, want := range []string{"error:unknown-role", "error:empty-control-implementation", "warning:duplicate-requirement", "warning:statement-mismatch"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}

func TestSSPChecks(t *testing.T) {
	s := &oscal.SSP{UUID: "nope", Metadata: oscal.Metadata{Title: "t", LastModified: "x", Version: "1", OSCALVersion: "1", Roles: []oscal.Role{{ID: "admin", Title: "A"}}},
		ImportProfile: oscal.ImportProfile{Href: "p.json"},
		SystemImplementation: oscal.SystemImplementation{
			Users:      []oscal.SystemUser{{UUID: u1, RoleIDs: []string{"admin", "ghost"}}},
			Components: []oscal.SystemComponent{{UUID: u2, Type: "software", Title: "S", Description: "d", Status: oscal.Status{State: "operational"}, ResponsibleRoles: []oscal.ResponsibleRole{{RoleID: "ghost2"}}}},
		},
		ControlImplementation: oscal.SSPControlImpl{Description: "d", ImplementedRequirements: []oscal.SSPImplementedRequirement{
			{UUID: u3, ControlID: "ac-1", ByComponents: []oscal.ByComponent{
				{ComponentUUID: "99999999-9999-4999-8999-999999999999", UUID: "44444444-4444-4444-8444-444444444444", Description: "d", ImplementationStatus: &oscal.ImplementationStatus{State: "done"}},
			}, Statements: []oscal.SSPStatement{{StatementID: "ac-2_smt.a", UUID: "55555555-5555-4555-8555-555555555555", ByComponents: []oscal.ByComponent{
				{ComponentUUID: u2, UUID: "66666666-6666-4666-8666-666666666666", Description: "d", ResponsibleRoles: []oscal.ResponsibleRole{{RoleID: "ghost3"}}},
			}}}},
			{UUID: "77777777-7777-4777-8777-777777777777", ControlID: "ac-1"},
		}},
	}
	got := codes(Document(decode(t, s)))
	for _, want := range []string{"error:unknown-component", "error:bad-implementation-status", "error:unknown-role", "error:bad-uuid", "warning:no-this-system", "warning:duplicate-requirement", "warning:statement-mismatch"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if Errors(Document(decode(t, s))) < 4 {
		t.Fatal("error count")
	}
	f := Finding{Severity: Error, Code: "c", Path: "/p", Message: "m"}
	if f.String() != "error c /p: m" {
		t.Fatal(f.String())
	}
}

func TestInvalidJSONInDocument(t *testing.T) {
	doc := &oscal.Document{Catalog: &oscal.Catalog{}, JSON: []byte("{")}
	if got := codes(Document(doc)); !strings.Contains(got, "invalid-json") {
		t.Fatalf("got %s", got)
	}
}
