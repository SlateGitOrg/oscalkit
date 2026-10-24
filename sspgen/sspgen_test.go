package sspgen

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/archit2501/oscalkit/check"
	"github.com/archit2501/oscalkit/oscal"
	"github.com/archit2501/oscalkit/resolve"
	"github.com/archit2501/oscalkit/schema"
)

const examples = "../examples"

func fixedOpts() Options {
	return Options{
		ProfileHref:      "../profiles/acme-low-tailored_profile.json",
		SystemName:       "Acme Customer Portal",
		ImpactLevel:      "low",
		IncludeUncovered: true,
		UUIDs:            oscal.DeterministicUUIDs{Seed: "t"},
		Now:              func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	}
}

func acmeInputs(t *testing.T) (*oscal.Catalog, []*oscal.ComponentDefinition) {
	t.Helper()
	res, err := resolve.ResolveFile(context.Background(), filepath.Join(examples, "profiles", "acme-low-tailored_profile.json"),
		resolve.Options{UUIDs: oscal.DeterministicUUIDs{Seed: "t"}})
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(examples, "components", "*.json"))
	slices.Sort(files)
	var cds []*oscal.ComponentDefinition
	for _, f := range files {
		doc, err := oscal.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		cds = append(cds, doc.ComponentDefinition)
	}
	return res.Catalog, cds
}

func TestGenerateAcmeIsValidAndDeterministic(t *testing.T) {
	cat, cds := acmeInputs(t)
	res, err := Generate(cat, cds, fixedOpts())
	if err != nil {
		t.Fatal(err)
	}
	data, err := oscal.Marshal(oscal.Document{SSP: res.SSP})
	if err != nil {
		t.Fatal(err)
	}
	vs, err := schema.Validate(schema.SSP, data)
	if err != nil || len(vs) != 0 {
		t.Fatalf("generated SSP is not schema-valid: %v %v", vs, err)
	}
	doc, err := oscal.Decode(data, "")
	if err != nil {
		t.Fatal(err)
	}
	if fs := check.Document(doc); check.Errors(fs) != 0 {
		t.Fatalf("generated SSP has semantic errors: %v", fs)
	}
	if res.Stats.Controls != len(res.SSP.ControlImplementation.ImplementedRequirements) || res.Stats.Covered+res.Stats.Uncovered != res.Stats.Controls || res.Stats.Components != 4 {
		t.Fatalf("stats = %+v", res.Stats)
	}
	again, _ := Generate(cat, cds, fixedOpts())
	data2, _ := oscal.Marshal(oscal.Document{SSP: again.SSP})
	if !bytes.Equal(data, data2) {
		t.Fatal("deterministic options must give byte-identical output")
	}
	sc := res.SSP.SystemCharacteristics
	if sc.SecurityImpactLevel == nil || sc.SecuritySensitivityLevel != "fips-199-low" {
		t.Fatal("impact level")
	}
}

func findIR(s *oscal.SSP, id string) *oscal.SSPImplementedRequirement {
	for i := range s.ControlImplementation.ImplementedRequirements {
		if s.ControlImplementation.ImplementedRequirements[i].ControlID == id {
			return &s.ControlImplementation.ImplementedRequirements[i]
		}
	}
	return nil
}

func TestGenerateContent(t *testing.T) {
	cat, cds := acmeInputs(t)
	res, err := Generate(cat, cds, fixedOpts())
	if err != nil {
		t.Fatal(err)
	}
	s := res.SSP
	ac2 := findIR(s, "ac-2")
	if ac2 == nil || len(ac2.ByComponents) != 1 || len(ac2.Statements) != 5 || ac2.ByComponents[0].ImplementationStatus.State != "implemented" {
		t.Fatalf("ac-2 = %+v", ac2)
	}
	ac7 := findIR(s, "ac-7")
	if ac7.SetParameters[0].ParamID != "ac-07_odp.01" {
		t.Fatalf("legacy param ids must be canonicalized: %+v", ac7.SetParameters)
	}
	if ia5 := findIR(s, "ia-5"); ia5.ByComponents[0].ImplementationStatus.State != "partial" {
		t.Fatal("implementation-status prop must carry over")
	}
	ac3 := findIR(s, "ac-3")
	if ac3 == nil || ac3.ByComponents[0].ImplementationStatus.State != "planned" || !strings.HasPrefix(ac3.ByComponents[0].Description, Placeholder) {
		t.Fatalf("uncovered control should be a planned placeholder: %+v", ac3)
	}
	if ac3.ByComponents[0].ComponentUUID != s.SystemImplementation.Components[0].UUID {
		t.Fatal("uncovered controls belong to this-system")
	}
	var roles []string
	for _, r := range s.Metadata.Roles {
		roles = append(roles, r.ID)
	}
	if !slices.Contains(roles, "identity-team") || !slices.Contains(roles, "ciso") {
		t.Fatalf("roles = %v", roles)
	}

	opts := fixedOpts()
	opts.IncludeUncovered = false
	res2, _ := Generate(cat, cds, opts)
	if len(res2.SSP.ControlImplementation.ImplementedRequirements) != res2.Stats.Covered {
		t.Fatal("without uncovered, only covered controls are emitted")
	}
}

func TestGenerateDiagnostics(t *testing.T) {
	cat := &oscal.Catalog{Controls: []oscal.Control{
		{ID: "x-1", Title: "X", Params: []oscal.Parameter{{ID: "x-1_p"}}, Parts: []oscal.Part{{Name: "statement", ID: "x-1_smt", Parts: []oscal.Part{{Name: "item", ID: "x-1_smt.a"}}}}},
		{ID: "x-2", Title: "Withdrawn", Props: []oscal.Property{{Name: "status", Value: "withdrawn"}}},
	}}
	comp := func(uuid, title, value string) oscal.DefinedComponent {
		return oscal.DefinedComponent{UUID: uuid, Type: "software", Title: title, Description: "d",
			ResponsibleRoles: []oscal.ResponsibleRole{{RoleID: "admin_team", PartyUUIDs: []string{"p"}}},
			ControlImplementations: []oscal.ControlImplementation{{UUID: uuid, Source: "p", Description: "d",
				SetParameters: []oscal.SetParameter{{ParamID: "x-1_p", Values: []string{"ci-level"}}, {ParamID: "elsewhere", Values: []string{"silently ignored"}}},
				ImplementedRequirements: []oscal.ComponentImplementedRequirement{
					{UUID: uuid, ControlID: "x-1", Description: title + " does x-1",
						SetParameters: []oscal.SetParameter{{ParamID: "x-1_p", Values: []string{value}}, {ParamID: "bogus", Values: []string{"v"}}},
						Statements:    []oscal.ComponentStatement{{StatementID: "x-1_smt.a", UUID: uuid, Description: "a"}, {StatementID: "x-1_smt.zz", UUID: uuid, Description: "?"}}},
					{UUID: uuid, ControlID: "y-9", Description: "not in baseline"},
				}}}}
	}
	cds := []*oscal.ComponentDefinition{
		{Components: []oscal.DefinedComponent{comp("aaaaaaaa-0000-4000-8000-000000000001", "First", "one")}},
		{Components: []oscal.DefinedComponent{comp("aaaaaaaa-0000-4000-8000-000000000001", "Second", "two")}},
	}
	res, err := Generate(cat, cds, Options{ProfileHref: "p.json"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range res.Diagnostics {
		got = append(got, d.Code)
	}
	for _, want := range []string{"duplicate-component", "not-in-baseline", "unknown-param", "unknown-statement", "conflicting-param"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	ir := res.SSP.ControlImplementation.ImplementedRequirements
	if len(ir) != 1 || len(ir[0].ByComponents) != 2 || len(ir[0].Statements) != 1 || ir[0].SetParameters[0].Values[0] != "one" {
		t.Fatalf("x-1 should carry both components, one known statement and the first value: %+v", ir)
	}
	if res.SSP.SystemImplementation.Components[1].ResponsibleRoles[0].PartyUUIDs != nil {
		t.Fatal("party references are not carried into the skeleton")
	}
}

func TestGenerateDiagnosticsCovered(t *testing.T) {
	cat := &oscal.Catalog{Controls: []oscal.Control{{ID: "x-1", Title: "X", Params: []oscal.Parameter{{ID: "x-1_p"}}}}}
	cd := &oscal.ComponentDefinition{Components: []oscal.DefinedComponent{{UUID: "aaaaaaaa-0000-4000-8000-000000000001", Type: "software", Title: "C", Description: "d",
		ControlImplementations: []oscal.ControlImplementation{{UUID: "u", Source: "p", Description: "d",
			SetParameters:           []oscal.SetParameter{{ParamID: "x-1_p", Values: []string{"ci-level"}}},
			ImplementedRequirements: []oscal.ComponentImplementedRequirement{{UUID: "u", ControlID: "x-1", Description: "d", SetParameters: []oscal.SetParameter{{ParamID: "x-1_p", Values: []string{"ir-level"}}}}}}}}}}
	res, err := Generate(cat, []*oscal.ComponentDefinition{cd}, Options{ProfileHref: "p.json"})
	if err != nil {
		t.Fatal(err)
	}
	ir := res.SSP.ControlImplementation.ImplementedRequirements[0]
	if len(ir.SetParameters) != 1 || ir.SetParameters[0].Values[0] != "ir-level" || len(ir.ByComponents[0].SetParameters) != 1 {
		t.Fatalf("requirement-level values override control-implementation level: %+v", ir)
	}
	if res.SSP.SystemCharacteristics.SystemName != "Unnamed System" {
		t.Fatal("default system name")
	}
}

func TestGenerateOptionErrors(t *testing.T) {
	cat := &oscal.Catalog{}
	if _, err := Generate(nil, nil, Options{ProfileHref: "p"}); err == nil {
		t.Fatal("nil baseline")
	}
	if _, err := Generate(cat, nil, Options{}); err == nil {
		t.Fatal("missing href")
	}
	if _, err := Generate(cat, nil, Options{ProfileHref: "p", ImpactLevel: "extreme"}); err == nil {
		t.Fatal("bad impact level")
	}
	if titleFromID("system-owner_x") != "System Owner X" || titleFromID("--") != "--" {
		t.Fatal("titleFromID")
	}
	if rankOf("mostly-done") <= rankOf("planned") || rankOf("implemented") >= rankOf("partial") {
		t.Fatal("unknown states rank as least complete")
	}
	if (Diagnostic{Code: "c", Message: "m"}).String() != "c: m" {
		t.Fatal("diagnostic string")
	}
}

// A component that claims the same controls in two control-implementations
// (e.g. one per baseline revision) must still give a valid SSP: one
// by-component per (control, component), with unique uuids.
func TestGenerateMergesRepeatedControlImplementations(t *testing.T) {
	cat, _ := acmeInputs(t)
	doc, err := oscal.ReadFile(filepath.Join(examples, "components", "acme-sso_component.json"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := oscal.ReadFile(filepath.Join(examples, "components", "acme-sso_component.json"))
	if err != nil {
		t.Fatal(err)
	}
	comp := &doc.ComponentDefinition.Components[0]
	second := again.ComponentDefinition.Components[0].ControlImplementations[0]
	second.UUID = "bbbbbbbb-0000-4000-8000-000000000002"
	second.Source = "https://example.org/NIST_SP-800-53_rev5_LOW-baseline_profile.json"
	for i := range second.ImplementedRequirements {
		ir := &second.ImplementedRequirements[i]
		if ir.ControlID == "ac-2" {
			ir.Description = "A second, revision-specific narrative."
			ir.Props = append(ir.Props, oscal.Property{Name: "implementation-status", Value: "partial"})
			ir.SetParameters = append(ir.SetParameters[:0:0], oscal.SetParameter{ParamID: "ac-02_odp.01", Values: []string{"a different value"}})
			ir.ResponsibleRoles = append(ir.ResponsibleRoles, oscal.ResponsibleRole{RoleID: "security-officer"})
		}
	}
	comp.ControlImplementations = append(comp.ControlImplementations, second)

	res, err := Generate(cat, []*oscal.ComponentDefinition{doc.ComponentDefinition}, fixedOpts())
	if err != nil {
		t.Fatal(err)
	}
	data, err := oscal.Marshal(oscal.Document{SSP: res.SSP})
	if err != nil {
		t.Fatal(err)
	}
	if vs, err := schema.Validate(schema.SSP, data); err != nil || len(vs) != 0 {
		t.Fatalf("not schema-valid: %v %v", vs, err)
	}
	decoded, err := oscal.Decode(data, "")
	if err != nil {
		t.Fatal(err)
	}
	if fs := check.Document(decoded); check.Errors(fs) != 0 {
		t.Fatalf("semantic errors (duplicate uuids?): %v", fs)
	}
	var codes []string
	for _, d := range res.Diagnostics {
		codes = append(codes, d.Code)
	}
	for _, want := range []string{"merged-implementations", "conflicting-status", "conflicting-param"} {
		if !slices.Contains(codes, want) {
			t.Errorf("missing %s in %v", want, codes)
		}
	}
	if res.Stats.Covered != 7 {
		t.Fatalf("covered = %d, want 7", res.Stats.Covered)
	}
	for _, ir := range res.SSP.ControlImplementation.ImplementedRequirements {
		if len(ir.ByComponents) != 1 {
			t.Fatalf("%s: %d by-components, want one per component", ir.ControlID, len(ir.ByComponents))
		}
		for _, st := range ir.Statements {
			if len(st.ByComponents) != 1 {
				t.Fatalf("%s: %d by-components, want one per component", st.StatementID, len(st.ByComponents))
			}
		}
		if ir.ControlID != "ac-2" {
			continue
		}
		bc := ir.ByComponents[0]
		if !strings.HasSuffix(bc.Description, "\n\nA second, revision-specific narrative.") || strings.Count(bc.Description, "\n\n") != 1 {
			t.Errorf("descriptions are joined, distinct ones only: %q", bc.Description)
		}
		if bc.ImplementationStatus.State != "partial" {
			t.Errorf("the least complete status wins, got %q", bc.ImplementationStatus.State)
		}
		if len(bc.ResponsibleRoles) != 2 {
			t.Errorf("roles are unioned by id: %+v", bc.ResponsibleRoles)
		}
		for _, sp := range ir.SetParameters {
			if sp.ParamID == "ac-02_odp.01" && sp.Values[0] == "a different value" {
				t.Error("the first claim's parameter value wins")
			}
		}
		if ir.Statements[0].ByComponents[0].ImplementationStatus.State != "partial" {
			t.Error("statement status defaults to the merged requirement's status")
		}
	}
	// Identical duplicate claims (the same control-implementation twice)
	// merge silently apart from the merged-implementations notice.
	dup := *doc.ComponentDefinition
	dup.Components = slices.Clone(dup.Components)
	dup.Components[0].ControlImplementations = []oscal.ControlImplementation{comp.ControlImplementations[0], comp.ControlImplementations[0]}
	res, err = Generate(cat, []*oscal.ComponentDefinition{&dup}, fixedOpts())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range res.Diagnostics {
		if d.Code != "merged-implementations" && d.Code != "unknown-param" {
			t.Errorf("unexpected diagnostic for identical claims: %s", d)
		}
	}
}

// The same component passed three times still gets three distinct
// system-component uuids under a deterministic generator.
func TestGenerateRepeatedComponentGetsFreshUUIDs(t *testing.T) {
	cat, cds := acmeInputs(t)
	sso := cds[len(cds)-1]
	res, err := Generate(cat, []*oscal.ComponentDefinition{sso, sso, sso}, fixedOpts())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range res.SSP.SystemImplementation.Components {
		if seen[c.UUID] {
			t.Fatalf("duplicate system-component uuid %s", c.UUID)
		}
		seen[c.UUID] = true
	}
	data, _ := oscal.Marshal(oscal.Document{SSP: res.SSP})
	decoded, err := oscal.Decode(data, "")
	if err != nil {
		t.Fatal(err)
	}
	if fs := check.Document(decoded); check.Errors(fs) != 0 {
		t.Fatalf("semantic errors: %v", fs)
	}
}
