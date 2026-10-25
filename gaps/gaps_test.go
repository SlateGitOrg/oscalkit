package gaps

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/SlateGitOrg/oscalkit/oscal"
)

const (
	compA = "aaaaaaaa-0000-4000-8000-000000000001"
	compB = "aaaaaaaa-0000-4000-8000-000000000002"
)

func baseline() *oscal.Catalog {
	stmt := func(id string, items ...string) []oscal.Part {
		p := oscal.Part{Name: "statement", ID: id + "_smt", Prose: id + " statement"}
		for _, it := range items {
			p.Parts = append(p.Parts, oscal.Part{Name: "item", ID: id + "_smt." + it, Prose: "item " + it})
		}
		return []oscal.Part{p}
	}
	x1 := oscal.Control{ID: "x-1", Title: "One", Parts: stmt("x-1", "a", "b"),
		Params: []oscal.Parameter{{ID: "x-01_odp.01", Label: "frequency", Props: []oscal.Property{{Name: "alt-identifier", Value: "x-1_prm_1"}}}, {ID: "x-01_odp.02", Select: &oscal.ParameterSelection{Choice: []string{"a", "b"}}}}}
	x1.Parts[0].Parts[0].Prose = "every {{ insert: param, x-01_odp.01 }}"
	x1.Parts[0].Parts[1].Prose = "choose {{ insert: param, x-01_odp.02 }}"
	return &oscal.Catalog{Metadata: oscal.Metadata{Title: "Baseline"}, Groups: []oscal.Group{
		{ID: "x", Title: "X family", Controls: []oscal.Control{
			x1,
			{ID: "x-2", Title: "Two", Parts: stmt("x-2")},
			{ID: "x-3", Title: "Withdrawn", Props: []oscal.Property{{Name: "status", Value: "withdrawn"}}},
			{ID: "x-4", Title: "Four"},
			{ID: "x-5", Title: "Five"},
			{ID: "x-6", Title: "Six"},
		}},
		{ID: "y", Title: "Y family", Controls: []oscal.Control{{ID: "y-1", Title: "Y1"}, {ID: "y-2", Title: "Y2"}}},
	}}
}

func status(s string) *oscal.ImplementationStatus { return &oscal.ImplementationStatus{State: s} }

func ssp() *oscal.SSP {
	bc := func(comp, desc string, st *oscal.ImplementationStatus) oscal.ByComponent {
		return oscal.ByComponent{ComponentUUID: comp, UUID: comp, Description: desc, ImplementationStatus: st}
	}
	return &oscal.SSP{
		Metadata: oscal.Metadata{Title: "Test SSP"},
		SystemImplementation: oscal.SystemImplementation{Components: []oscal.SystemComponent{
			{UUID: compA, Type: "this-system"}, {UUID: compB, Type: "software"},
		}},
		ControlImplementation: oscal.SSPControlImpl{
			SetParameters: []oscal.SetParameter{{ParamID: "x-1_prm_1", Values: []string{"monthly"}}},
			ImplementedRequirements: []oscal.SSPImplementedRequirement{
				{ControlID: "x-1", Statements: []oscal.SSPStatement{
					{StatementID: "x-1_smt.a", ByComponents: []oscal.ByComponent{bc(compA, "does a", status("implemented"))}},
					{StatementID: "x-1_smt.zz", ByComponents: []oscal.ByComponent{bc(compA, "?", status("implemented"))}},
				}, SetParameters: []oscal.SetParameter{{ParamID: "x-9_nope", Values: []string{"v"}}}},
				{ControlID: "x-2", ByComponents: []oscal.ByComponent{bc(compA, "planned", status("planned")), bc(compB, "done", status("implemented"))}},
				{ControlID: "x-3", ByComponents: []oscal.ByComponent{bc(compA, "withdrawn", status("implemented"))}},
				{ControlID: "x-4", ByComponents: []oscal.ByComponent{bc("99999999-0000-4000-8000-000000000009", "ghost", status("partial"))}},
				{ControlID: "x-5", ByComponents: []oscal.ByComponent{bc(compA, "[Placeholder] n/a", status("not-applicable"))}},
				{ControlID: "x-6"},
				{ControlID: "x-6", ByComponents: []oscal.ByComponent{{ComponentUUID: compA, Description: "no status", Props: []oscal.Property{{Name: "implementation-status", Value: "alternative"}}}}},
				{ControlID: "y-1", ByComponents: []oscal.ByComponent{bc(compA, "na", &oscal.ImplementationStatus{State: "not-applicable", Remarks: "no wireless"}), bc(compB, "unknown status", status("bogus"))}},
				{ControlID: "z-1", ByComponents: []oscal.ByComponent{bc(compA, "extra", status("implemented"))}},
			},
		},
	}
}

func kinds(r *Report, ctl string) []Kind {
	var out []Kind
	for _, g := range r.Gaps {
		if g.ControlID == ctl {
			out = append(out, g.Kind)
		}
	}
	return out
}

func TestAnalyzeKinds(t *testing.T) {
	r := Analyze(ssp(), baseline(), Options{})
	expect := map[string][]Kind{
		"x-1": {UnknownStatement, UncoveredStatement, UnsetParameter, UnknownParameter},
		"x-2": {StatusPlanned},
		"x-3": {WithdrawnControl},
		"x-4": {UnknownComponent, StatusPartial},
		"x-5": {NotApplicableNoWhy},
		"x-6": {DuplicateControl, StatusAlternative},
		"y-1": {StatusUnspecified},
		"y-2": {MissingControl},
		"z-1": {NotInBaseline},
	}
	for ctl, want := range expect {
		got := kinds(r, ctl)
		for _, k := range want {
			if !slices.Contains(got, k) {
				t.Errorf("%s: missing %s in %v", ctl, k, got)
			}
		}
	}
	// x-01_odp.01 is set through its legacy alias at the global level.
	for _, g := range r.Gaps {
		if g.Kind == UnsetParameter && strings.Contains(g.Detail, "x-01_odp.01") {
			t.Errorf("alias-set parameter reported unset: %s", g.Detail)
		}
	}
	if !slices.ContainsFunc(r.Gaps, func(g Gap) bool { return g.Kind == UnsetParameter && strings.Contains(g.Detail, "[Selection: a; b]") }) {
		t.Error("unset selection parameter should describe its choices")
	}
	// Severity ordering and helpers.
	for i := 1; i < len(r.Gaps); i++ {
		if r.Gaps[i].Severity > r.Gaps[i-1].Severity {
			t.Fatal("gaps must be sorted by severity")
		}
	}
	if r.Worst() != Error || r.Count(Info) != len(r.Gaps) || r.Count(Error) == 0 {
		t.Error("Worst/Count")
	}
}

func TestAnalyzeSummary(t *testing.T) {
	r := Analyze(ssp(), baseline(), Options{})
	s := r.Summary
	// 7 active controls (x-3 withdrawn); y-2 missing.
	if s.BaselineControls != 7 || s.Addressed != 6 || s.ByStatus[StatMissing] != 1 {
		t.Fatalf("summary = %+v", s)
	}
	// Only x-6 (alternative) is satisfied: everything else has a warning/error.
	if s.Satisfied != 1 || s.CoveragePercent < 14.2 || s.CoveragePercent > 14.3 {
		t.Fatalf("satisfied = %d (%.2f%%)", s.Satisfied, s.CoveragePercent)
	}
	if len(r.Families) != 2 || r.Families[0].Family != "x" || r.Families[0].Title != "X family" || r.Families[1].Missing != 1 {
		t.Fatalf("families = %+v", r.Families)
	}
	if r.SSPTitle != "Test SSP" || r.BaselineTitle != "Baseline" {
		t.Fatal("titles")
	}
	data, err := json.Marshal(r)
	if err != nil || !strings.Contains(string(data), `"severity":"error"`) {
		t.Fatalf("severity must marshal as text: %v", err)
	}
}

func TestIgnoreParametersAndCleanControl(t *testing.T) {
	s := ssp()
	s.ControlImplementation.ImplementedRequirements[0] = oscal.SSPImplementedRequirement{ControlID: "x-1",
		ByComponents: []oscal.ByComponent{{ComponentUUID: compA, Description: "all of it", ImplementationStatus: status("implemented"),
			SetParameters: []oscal.SetParameter{{ParamID: "x-01_odp.02", Values: []string{"a"}}}}}}
	r := Analyze(s, baseline(), Options{})
	if got := kinds(r, "x-1"); len(got) != 0 {
		t.Fatalf("fully implemented x-1 should have no gaps, got %v", got)
	}
	s.ControlImplementation.ImplementedRequirements[0].ByComponents[0].SetParameters = nil
	if got := kinds(Analyze(s, baseline(), Options{IgnoreParameters: true}), "x-1"); len(got) != 0 {
		t.Fatalf("IgnoreParameters: %v", got)
	}
}

func TestFamilyFallbackAndSeverityParsing(t *testing.T) {
	cat := &oscal.Catalog{Controls: []oscal.Control{{ID: "ab-1", Title: "A"}, {ID: "solo", Title: "S"}}}
	r := Analyze(&oscal.SSP{}, cat, Options{})
	if r.Families[0].Family != "ab" || r.Families[1].Family != "solo" {
		t.Fatalf("families = %+v", r.Families)
	}
	for in, want := range map[string]Severity{"info": Info, "WARN": Warning, "warning": Warning, "error": Error} {
		if got, err := ParseSeverity(in); err != nil || got != want {
			t.Errorf("ParseSeverity(%q) = %v %v", in, got, err)
		}
	}
	if _, err := ParseSeverity("fatal"); err == nil {
		t.Error("unknown severity")
	}
	if Info.String() != "info" || Warning.String() != "warning" || Error.String() != "error" {
		t.Error("String")
	}
	empty := Analyze(&oscal.SSP{}, &oscal.Catalog{}, Options{})
	if empty.Summary.CoveragePercent != 0 || empty.Worst() != Info {
		t.Error("empty baseline")
	}
}
