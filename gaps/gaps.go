// Package gaps validates a System Security Plan against its resolved
// baseline and reports control gaps: baseline controls the SSP does not
// address, implementations that are planned or partial, unset
// organization-defined parameters, uncovered statement items and broken
// references.
package gaps

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/SlateGitOrg/oscalkit/oscal"
	"github.com/SlateGitOrg/oscalkit/param"
)

// Severity orders gaps.
type Severity int

// Severities, least to most severe.
const (
	Info Severity = iota
	Warning
	Error
)

func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	}
	return "info"
}

// MarshalText implements encoding.TextMarshaler.
func (s Severity) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// ParseSeverity parses "info", "warning" or "error".
func ParseSeverity(s string) (Severity, error) {
	switch strings.ToLower(s) {
	case "info":
		return Info, nil
	case "warning", "warn":
		return Warning, nil
	case "error":
		return Error, nil
	}
	return Info, fmt.Errorf("unknown severity %q", s)
}

// Kind classifies a gap.
type Kind string

// Gap kinds.
const (
	MissingControl     Kind = "missing-control"
	NotInBaseline      Kind = "not-in-baseline"
	NoImplementation   Kind = "no-implementation"
	StatusPlanned      Kind = "status-planned"
	StatusPartial      Kind = "status-partial"
	StatusAlternative  Kind = "status-alternative"
	StatusUnspecified  Kind = "status-unspecified"
	NotApplicableNoWhy Kind = "not-applicable-unjustified"
	UnsetParameter     Kind = "unset-parameter"
	UnknownParameter   Kind = "unknown-parameter"
	UncoveredStatement Kind = "uncovered-statement"
	UnknownStatement   Kind = "unknown-statement"
	UnknownComponent   Kind = "unknown-component"
	WithdrawnControl   Kind = "withdrawn-control"
	DuplicateControl   Kind = "duplicate-requirement"
)

// Gap is one finding against one control.
type Gap struct {
	ControlID string   `json:"control-id"`
	Kind      Kind     `json:"kind"`
	Severity  Severity `json:"severity"`
	Detail    string   `json:"detail"`
}

// Status is a control's effective implementation status.
type Status string

// Effective statuses.
const (
	StatImplemented   Status = "implemented"
	StatPartial       Status = "partial"
	StatPlanned       Status = "planned"
	StatAlternative   Status = "alternative"
	StatNotApplicable Status = "not-applicable"
	StatUnspecified   Status = "unspecified"
	StatMissing       Status = "missing"
)

// statusRank orders statuses from best to worst for aggregation.
var statusRank = map[Status]int{
	StatImplemented: 0, StatNotApplicable: 1, StatAlternative: 2,
	StatUnspecified: 3, StatPartial: 4, StatPlanned: 5, StatMissing: 6,
}

// ControlResult is the per-control outcome.
type ControlResult struct {
	ControlID string `json:"control-id"`
	Family    string `json:"family"`
	Status    Status `json:"status"`
	// Satisfied is true when the control is implemented (or justified as
	// not applicable / alternative) with no error- or warning-level gaps.
	Satisfied bool `json:"satisfied"`
}

// FamilySummary aggregates results for one control family (top group).
type FamilySummary struct {
	Family    string `json:"family"`
	Title     string `json:"title,omitempty"`
	Total     int    `json:"total"`
	Satisfied int    `json:"satisfied"`
	Missing   int    `json:"missing"`
}

// Summary holds headline numbers.
type Summary struct {
	BaselineControls int            `json:"baseline-controls"`
	Addressed        int            `json:"addressed"`
	Satisfied        int            `json:"satisfied"`
	ByStatus         map[Status]int `json:"by-status"`
	// CoveragePercent is Satisfied / BaselineControls * 100.
	CoveragePercent float64          `json:"coverage-percent"`
	GapsBySeverity  map[Severity]int `json:"gaps-by-severity"`
}

// Report is the full gap analysis.
type Report struct {
	SSPTitle      string          `json:"ssp-title"`
	BaselineTitle string          `json:"baseline-title"`
	Summary       Summary         `json:"summary"`
	Families      []FamilySummary `json:"families"`
	Controls      []ControlResult `json:"controls"`
	Gaps          []Gap           `json:"gaps"`
}

// Options tunes the analysis.
type Options struct {
	// IgnoreParameters disables unset-parameter gaps.
	IgnoreParameters bool
}

// Worst returns the highest severity among the gaps (Info if none).
func (r *Report) Worst() Severity {
	w := Info
	for _, g := range r.Gaps {
		if g.Severity > w {
			w = g.Severity
		}
	}
	return w
}

// Count returns the number of gaps at or above sev.
func (r *Report) Count(sev Severity) int {
	n := 0
	for _, g := range r.Gaps {
		if g.Severity >= sev {
			n++
		}
	}
	return n
}

type analyzer struct {
	ssp      *oscal.SSP
	baseline *oscal.Catalog
	opts     Options
	render   *param.Renderer
	comps    map[string]bool
	gaps     []Gap
}

func (a *analyzer) add(ctl string, kind Kind, sev Severity, format string, args ...any) {
	a.gaps = append(a.gaps, Gap{ControlID: ctl, Kind: kind, Severity: sev, Detail: fmt.Sprintf(format, args...)})
}

// Analyze compares ssp against the resolved baseline catalog.
func Analyze(ssp *oscal.SSP, baseline *oscal.Catalog, opts Options) *Report {
	a := &analyzer{
		ssp:      ssp,
		baseline: baseline,
		opts:     opts,
		render:   param.NewRenderer(baseline),
		comps:    map[string]bool{},
	}
	for _, c := range ssp.SystemImplementation.Components {
		a.comps[c.UUID] = true
	}
	return a.run()
}

func (a *analyzer) run() *Report {
	// Global set-parameters apply to every control.
	for _, sp := range a.ssp.ControlImplementation.SetParameters {
		a.render.SetValues(sp.ParamID, sp.Values)
	}
	reqs := map[string][]*oscal.SSPImplementedRequirement{}
	var reqOrder []string
	for i := range a.ssp.ControlImplementation.ImplementedRequirements {
		ir := &a.ssp.ControlImplementation.ImplementedRequirements[i]
		if _, seen := reqs[ir.ControlID]; !seen {
			reqOrder = append(reqOrder, ir.ControlID)
		} else {
			a.add(ir.ControlID, DuplicateControl, Warning, "control has more than one implemented-requirement; they are analysed together")
		}
		reqs[ir.ControlID] = append(reqs[ir.ControlID], ir)
	}

	rep := &Report{
		SSPTitle:      a.ssp.Metadata.Title,
		BaselineTitle: a.baseline.Metadata.Title,
	}
	rep.Summary.ByStatus = map[Status]int{}
	families := map[string]*FamilySummary{}
	var familyOrder []string
	groupTitles := groupTitles(a.baseline)
	inBaseline := map[string]bool{}

	a.baseline.WalkControls(func(ref oscal.ControlRef) bool {
		ctl := ref.Control
		inBaseline[ctl.ID] = true
		if ctl.IsWithdrawn() {
			return true
		}
		fam := family(ref)
		fs, ok := families[fam]
		if !ok {
			fs = &FamilySummary{Family: fam, Title: groupTitles[fam]}
			families[fam] = fs
			familyOrder = append(familyOrder, fam)
		}
		before := len(a.gaps)
		status := a.control(ctl, reqs[ctl.ID])
		blocking := false
		for _, g := range a.gaps[before:] {
			if g.Severity >= Warning {
				blocking = true
			}
		}
		satisfied := !blocking && (status == StatImplemented || status == StatNotApplicable || status == StatAlternative)
		rep.Controls = append(rep.Controls, ControlResult{ControlID: ctl.ID, Family: fam, Status: status, Satisfied: satisfied})
		rep.Summary.ByStatus[status]++
		rep.Summary.BaselineControls++
		fs.Total++
		if status != StatMissing {
			rep.Summary.Addressed++
		} else {
			fs.Missing++
		}
		if satisfied {
			rep.Summary.Satisfied++
			fs.Satisfied++
		}
		return true
	})

	idx := a.baseline.ControlIndex()
	for _, id := range reqOrder {
		if !inBaseline[id] {
			a.add(id, NotInBaseline, Warning, "the SSP implements a control that is not part of the baseline")
			continue
		}
		if ref := idx[id]; ref.Control.IsWithdrawn() {
			a.add(id, WithdrawnControl, Warning, "the SSP implements a control that is withdrawn in the baseline")
		}
	}

	for _, f := range familyOrder {
		rep.Families = append(rep.Families, *families[f])
	}
	if rep.Summary.BaselineControls > 0 {
		rep.Summary.CoveragePercent = 100 * float64(rep.Summary.Satisfied) / float64(rep.Summary.BaselineControls)
	}
	sort.SliceStable(a.gaps, func(i, j int) bool { return a.gaps[i].Severity > a.gaps[j].Severity })
	rep.Gaps = a.gaps
	rep.Summary.GapsBySeverity = map[Severity]int{}
	for _, g := range a.gaps {
		rep.Summary.GapsBySeverity[g.Severity]++
	}
	return rep
}

// control analyses one baseline control and returns its effective status.
func (a *analyzer) control(ctl *oscal.Control, irs []*oscal.SSPImplementedRequirement) Status {
	if len(irs) == 0 {
		a.add(ctl.ID, MissingControl, Error, "baseline control %s (%s) has no implemented-requirement", strings.ToUpper(ctl.Label()), ctl.Title)
		return StatMissing
	}
	local := a.renderFor(irs)
	var states []Status
	stmtCovered := map[string]bool{}
	hasStatements := false
	partIDs := partIDs(ctl)
	for _, ir := range irs {
		states = append(states, a.byComponents(ctl.ID, ir.ByComponents, ir.Remarks)...)
		for _, st := range ir.Statements {
			hasStatements = true
			if !partIDs[st.StatementID] {
				a.add(ctl.ID, UnknownStatement, Error, "statement %q is not a part of %s", st.StatementID, ctl.ID)
				continue
			}
			stmtCovered[st.StatementID] = true
			states = append(states, a.byComponents(ctl.ID, st.ByComponents, st.Remarks)...)
		}
		a.checkParams(ctl, ir)
	}
	if len(states) == 0 {
		a.add(ctl.ID, NoImplementation, Error, "implemented-requirement has no by-component descriptions")
		return StatMissing
	}
	status := worst(states)
	switch status {
	case StatPlanned:
		a.add(ctl.ID, StatusPlanned, Warning, "implementation is planned, not in place")
	case StatPartial:
		a.add(ctl.ID, StatusPartial, Warning, "implementation is only partial")
	case StatAlternative:
		a.add(ctl.ID, StatusAlternative, Info, "an alternative implementation is claimed")
	case StatUnspecified:
		a.add(ctl.ID, StatusUnspecified, Warning, "no implementation-status is given for one or more components")
	}
	if hasStatements {
		for _, item := range statementItems(ctl) {
			if !coveredBy(item, stmtCovered) {
				a.add(ctl.ID, UncoveredStatement, Warning, "statement item %s is not addressed", item)
			}
		}
	}
	if !a.opts.IgnoreParameters && status != StatNotApplicable {
		for _, id := range param.StatementRefs(ctl) {
			if !local.IsSet(id) {
				a.add(ctl.ID, UnsetParameter, Warning, "parameter %s (%s) has no value", id, paramLabel(local, id))
			}
		}
	}
	return status
}

// renderFor returns a renderer with this control's set-parameters applied
// on top of the global ones (by-component values included).
func (a *analyzer) renderFor(irs []*oscal.SSPImplementedRequirement) *param.Renderer {
	r := param.NewRenderer(a.baseline)
	for _, sp := range a.ssp.ControlImplementation.SetParameters {
		r.SetValues(sp.ParamID, sp.Values)
	}
	apply := func(sps []oscal.SetParameter) {
		for _, sp := range sps {
			r.SetValues(sp.ParamID, sp.Values)
		}
	}
	for _, ir := range irs {
		for _, bc := range ir.ByComponents {
			apply(bc.SetParameters)
		}
		for _, st := range ir.Statements {
			for _, bc := range st.ByComponents {
				apply(bc.SetParameters)
			}
		}
		apply(ir.SetParameters)
	}
	return r
}

func (a *analyzer) checkParams(ctl *oscal.Control, ir *oscal.SSPImplementedRequirement) {
	own := map[string]bool{}
	for _, p := range ctl.Params {
		own[p.ID] = true
	}
	check := func(sps []oscal.SetParameter) {
		for _, sp := range sps {
			canon, known := a.render.Canonical(sp.ParamID)
			if !known || !own[canon] {
				a.add(ctl.ID, UnknownParameter, Error, "set-parameter %q is not a parameter of %s", sp.ParamID, ctl.ID)
			}
		}
	}
	check(ir.SetParameters)
	for _, bc := range ir.ByComponents {
		check(bc.SetParameters)
	}
}

// byComponents validates component references and returns each entry's
// status. A not-applicable claim needs a justification in remarks or in the
// description.
func (a *analyzer) byComponents(ctlID string, bcs []oscal.ByComponent, remarks string) []Status {
	var out []Status
	for _, bc := range bcs {
		if !a.comps[bc.ComponentUUID] {
			a.add(ctlID, UnknownComponent, Error, "by-component references unknown component %s", bc.ComponentUUID)
		}
		st := StatUnspecified
		why := bc.Remarks + remarks
		if bc.ImplementationStatus != nil {
			why += bc.ImplementationStatus.Remarks
			st = Status(bc.ImplementationStatus.State)
			if _, ok := statusRank[st]; !ok || st == StatMissing || st == StatUnspecified {
				st = StatUnspecified
			}
		} else if v, ok := oscal.Prop(bc.Props, "implementation-status"); ok {
			if _, known := statusRank[Status(v)]; known {
				st = Status(v)
			}
		}
		if st == StatNotApplicable && strings.TrimSpace(why) == "" && isPlaceholder(bc.Description) {
			a.add(ctlID, NotApplicableNoWhy, Warning, "not-applicable without a justification")
		}
		out = append(out, st)
	}
	return out
}

func isPlaceholder(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || strings.HasPrefix(s, "[Placeholder]")
}

func worst(states []Status) Status {
	w := states[0]
	for _, s := range states[1:] {
		if statusRank[s] > statusRank[w] {
			w = s
		}
	}
	// Mixed not-applicable and implemented components count as implemented.
	if w == StatNotApplicable && slices.Contains(states, StatImplemented) {
		return StatImplemented
	}
	return w
}

// statementItems lists the ids of the top-level items of a control's
// statement (e.g. ac-2_smt.a ... ac-2_smt.l). A statement without items is
// represented by its own id.
func statementItems(ctl *oscal.Control) []string {
	st, ok := ctl.StatementPart()
	if !ok {
		return nil
	}
	var items []string
	for _, p := range st.Parts {
		if p.ID != "" {
			items = append(items, p.ID)
		}
	}
	if len(items) == 0 && st.ID != "" {
		items = append(items, st.ID)
	}
	return items
}

// coveredBy reports whether item, the whole statement it belongs to, or one
// of its sub-items is addressed.
func coveredBy(item string, covered map[string]bool) bool {
	if covered[item] {
		return true
	}
	if i := strings.LastIndex(item, "."); i > 0 && covered[item[:i]] {
		return true
	}
	for id := range covered {
		if strings.HasPrefix(id, item+".") {
			return true
		}
	}
	return false
}

func partIDs(ctl *oscal.Control) map[string]bool {
	ids := map[string]bool{}
	var walk func(ps []oscal.Part)
	walk = func(ps []oscal.Part) {
		for _, p := range ps {
			if p.ID != "" {
				ids[p.ID] = true
			}
			walk(p.Parts)
		}
	}
	walk(ctl.Parts)
	return ids
}

// paramLabel describes a parameter for gap messages: its label, or for a
// selection parameter the rendered choices.
func paramLabel(r *param.Renderer, id string) string {
	p, ok := r.Param(id)
	if !ok {
		return "unknown"
	}
	if p.Label != "" {
		return r.Render(p.Label)
	}
	return r.Text(id)
}

// family names the control's top-level group, or the id prefix before "-".
func family(ref oscal.ControlRef) string {
	if len(ref.GroupPath) > 0 && ref.GroupPath[0] != "" {
		return ref.GroupPath[0]
	}
	if i := strings.Index(ref.Control.ID, "-"); i > 0 {
		return ref.Control.ID[:i]
	}
	return ref.Control.ID
}

func groupTitles(cat *oscal.Catalog) map[string]string {
	out := map[string]string{}
	for _, g := range cat.Groups {
		if g.ID != "" {
			out[g.ID] = g.Title
		}
	}
	return out
}
