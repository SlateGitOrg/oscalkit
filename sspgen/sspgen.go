// Package sspgen generates a System Security Plan skeleton from a resolved
// baseline and one or more component definitions. Every baseline control
// gets an implemented-requirement; controls that components address carry
// one by-component entry per contributing component (with its description,
// statements, parameter values and responsible roles), and the remaining
// controls are attributed to the "this-system" component as planned work.
package sspgen

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/SlateGitOrg/oscalkit/oscal"
	"github.com/SlateGitOrg/oscalkit/param"
)

// OSCALVersion is the OSCAL version declared by generated SSPs.
const OSCALVersion = "1.2.3"

// Placeholder prefixes text that a human must replace.
const Placeholder = "[Placeholder]"

// Options configures generation.
type Options struct {
	// ProfileHref is written to import-profile (required).
	ProfileHref string
	// SystemName names the system (default "Unnamed System").
	SystemName string
	// ImpactLevel, if set ("low", "moderate" or "high"), fills the
	// security-impact-level and security-sensitivity-level.
	ImpactLevel string
	// IncludeUncovered adds a planned this-system requirement for every
	// baseline control no component addresses.
	IncludeUncovered bool
	// UUIDs generates identifiers (default random v4).
	UUIDs oscal.UUIDGen
	// Now supplies last-modified (default time.Now).
	Now func() time.Time
}

// Diagnostic is a non-fatal generation issue.
type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (d Diagnostic) String() string { return d.Code + ": " + d.Message }

// Stats summarizes a generated SSP.
type Stats struct {
	Controls   int `json:"controls"`
	Covered    int `json:"covered"`
	Uncovered  int `json:"uncovered"`
	Components int `json:"components"`
}

// Result is the generated SSP plus diagnostics.
type Result struct {
	SSP         *oscal.SSP
	Diagnostics []Diagnostic
	Stats       Stats
}

// contribution is one component's implementation of one control. A
// component can claim the same control in several control-implementations
// (for example one per baseline revision); all of its claims are merged
// into a single contribution, so that the SSP carries one by-component per
// (control, component) pair.
type contribution struct {
	componentUUID string
	componentName string
	claims        []claim
}

// claim is one implemented-requirement of a component for a control, with
// the set-parameters of its enclosing control-implementation.
type claim struct {
	ir       oscal.ComponentImplementedRequirement
	ciParams []oscal.SetParameter
	source   string
}

type generator struct {
	opts     Options
	baseline *oscal.Catalog
	diags    []Diagnostic
	render   *param.Renderer
}

// Generate builds the SSP skeleton.
func Generate(baseline *oscal.Catalog, cds []*oscal.ComponentDefinition, opts Options) (*Result, error) {
	if baseline == nil {
		return nil, fmt.Errorf("sspgen: baseline catalog is required")
	}
	if opts.ProfileHref == "" {
		return nil, fmt.Errorf("sspgen: profile href is required")
	}
	switch opts.ImpactLevel {
	case "", "low", "moderate", "high":
	default:
		return nil, fmt.Errorf("sspgen: impact level must be low, moderate or high, not %q", opts.ImpactLevel)
	}
	if opts.SystemName == "" {
		opts.SystemName = "Unnamed System"
	}
	if opts.UUIDs == nil {
		opts.UUIDs = oscal.RandomUUIDs{}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	g := &generator{opts: opts, baseline: baseline, render: param.NewRenderer(baseline)}
	return g.run(cds)
}

func (g *generator) warn(code, format string, args ...any) {
	g.diags = append(g.diags, Diagnostic{Code: code, Message: fmt.Sprintf(format, args...)})
}

func (g *generator) run(cds []*oscal.ComponentDefinition) (*Result, error) {
	thisSystem := oscal.SystemComponent{
		UUID:        g.opts.UUIDs.New("component:this-system"),
		Type:        "this-system",
		Title:       g.opts.SystemName,
		Description: Placeholder + " Describe the system as a whole.",
		Status:      oscal.Status{State: "under-development"},
	}
	components := []oscal.SystemComponent{thisSystem}
	md := oscal.Metadata{
		Title:        "System Security Plan: " + g.opts.SystemName,
		LastModified: g.opts.Now().UTC().Format(time.RFC3339Nano),
		Version:      "0.1.0-draft",
		OSCALVersion: OSCALVersion,
	}
	roleIDs := []string{}
	addRoles := func(rrs []oscal.ResponsibleRole) {
		for _, rr := range rrs {
			if !slices.Contains(roleIDs, rr.RoleID) {
				roleIDs = append(roleIDs, rr.RoleID)
			}
		}
	}

	byControl := map[string][]contribution{}
	contribIdx := map[string]int{} // control id + NUL + component uuid → index in byControl
	baselineIdx := g.baseline.ControlIndex()
	usedUUIDs := map[string]bool{thisSystem.UUID: true}
	for _, cd := range cds {
		for _, comp := range cd.Components {
			uuid := comp.UUID
			if usedUUIDs[uuid] {
				g.warn("duplicate-component", "component %q reuses uuid %s; a new uuid was assigned", comp.Title, uuid)
				// The name is deterministic, so repeat it with a counter
				// until it is fresh (the same component can be passed
				// more than twice).
				for n := 0; usedUUIDs[uuid]; n++ {
					uuid = g.opts.UUIDs.New(fmt.Sprintf("component:%s:%s:%d", comp.UUID, comp.Title, n))
				}
			}
			usedUUIDs[uuid] = true
			components = append(components, oscal.SystemComponent{
				UUID:             uuid,
				Type:             comp.Type,
				Title:            comp.Title,
				Description:      comp.Description,
				Purpose:          comp.Purpose,
				Props:            slices.Clone(comp.Props),
				Links:            slices.Clone(comp.Links),
				Status:           oscal.Status{State: "operational"},
				ResponsibleRoles: stripParties(comp.ResponsibleRoles),
				Protocols:        comp.Protocols,
			})
			addRoles(comp.ResponsibleRoles)
			for _, ci := range comp.ControlImplementations {
				for _, ir := range ci.ImplementedRequirements {
					if _, ok := baselineIdx[ir.ControlID]; !ok {
						g.warn("not-in-baseline", "component %q implements %s, which is not in the baseline", comp.Title, ir.ControlID)
						continue
					}
					addRoles(ir.ResponsibleRoles)
					for _, st := range ir.Statements {
						addRoles(st.ResponsibleRoles)
					}
					cl := claim{ir: ir, ciParams: ci.SetParameters, source: ci.Source}
					key := ir.ControlID + "\x00" + uuid
					if i, ok := contribIdx[key]; ok {
						byControl[ir.ControlID][i].claims = append(byControl[ir.ControlID][i].claims, cl)
						continue
					}
					contribIdx[key] = len(byControl[ir.ControlID])
					byControl[ir.ControlID] = append(byControl[ir.ControlID], contribution{
						componentUUID: uuid, componentName: comp.Title, claims: []claim{cl},
					})
				}
			}
		}
	}
	for _, id := range roleIDs {
		md.Roles = append(md.Roles, oscal.Role{ID: id, Title: titleFromID(id)})
	}

	stats := Stats{Components: len(components) - 1}
	var reqs []oscal.SSPImplementedRequirement
	g.baseline.WalkControls(func(ref oscal.ControlRef) bool {
		ctl := ref.Control
		if ctl.IsWithdrawn() {
			return true
		}
		stats.Controls++
		contribs := byControl[ctl.ID]
		if len(contribs) == 0 {
			stats.Uncovered++
			if g.opts.IncludeUncovered {
				reqs = append(reqs, g.uncovered(ctl, thisSystem.UUID))
			}
			return true
		}
		stats.Covered++
		reqs = append(reqs, g.covered(ctl, contribs))
		return true
	})

	ssp := &oscal.SSP{
		UUID:          g.opts.UUIDs.New("ssp"),
		Metadata:      md,
		ImportProfile: oscal.ImportProfile{Href: g.opts.ProfileHref},
		SystemCharacteristics: oscal.SystemCharacteristics{
			SystemIDs:   []oscal.SystemID{{IdentifierType: "http://ietf.org/rfc/rfc4122", ID: g.opts.UUIDs.New("system-id")}},
			SystemName:  g.opts.SystemName,
			Description: Placeholder + " Describe the purpose and function of " + g.opts.SystemName + ".",
			SystemInformation: oscal.SystemInformation{InformationTypes: []oscal.InformationType{{
				UUID:        g.opts.UUIDs.New("information-type"),
				Title:       Placeholder + " Information type",
				Description: Placeholder + " Describe the information the system processes, stores or transmits.",
			}}},
			Status:                oscal.Status{State: "under-development"},
			AuthorizationBoundary: oscal.AuthorizationBoundary{Description: Placeholder + " Describe the authorization boundary."},
		},
		SystemImplementation: oscal.SystemImplementation{Components: components},
		ControlImplementation: oscal.SSPControlImpl{
			Description:             fmt.Sprintf("Control implementation generated from %d component definition(s) against %s.", len(cds), g.opts.ProfileHref),
			ImplementedRequirements: reqs,
		},
	}
	if lvl := g.opts.ImpactLevel; lvl != "" {
		ssp.SystemCharacteristics.SecuritySensitivityLevel = "fips-199-" + lvl
		ssp.SystemCharacteristics.SecurityImpactLevel = &oscal.SecurityImpactLevel{
			Confidentiality: "fips-199-" + lvl,
			Integrity:       "fips-199-" + lvl,
			Availability:    "fips-199-" + lvl,
		}
	}
	return &Result{SSP: ssp, Diagnostics: g.diags, Stats: stats}, nil
}

// covered builds the implemented-requirement for a control that at least
// one component addresses.
func (g *generator) covered(ctl *oscal.Control, contribs []contribution) oscal.SSPImplementedRequirement {
	ir := oscal.SSPImplementedRequirement{
		UUID:      g.opts.UUIDs.New("ir:" + ctl.ID),
		ControlID: ctl.ID,
	}
	ctlParams := controlParamIDs(ctl)
	values := map[string][]string{}
	valueFrom := map[string]string{}
	var order []string
	stmtIdx := map[string]int{}
	partIDs := partIDSet(ctl)
	for _, c := range contribs {
		if len(c.claims) > 1 {
			sources := make([]string, len(c.claims))
			for i, cl := range c.claims {
				sources[i] = cl.source
			}
			g.warn("merged-implementations", "component %q implements %s in %d requirements (sources %q); they were merged into one by-component",
				c.componentName, ctl.ID, len(c.claims), sources)
		}
		bc := oscal.ByComponent{
			ComponentUUID:        c.componentUUID,
			UUID:                 g.opts.UUIDs.New("bc:" + ctl.ID + ":" + c.componentUUID),
			Description:          joinDistinct(c.claims, func(cl claim) string { return cl.ir.Description }),
			ImplementationStatus: &oscal.ImplementationStatus{State: mergedStatus(g, ctl.ID, c, c.claims, func(cl claim) string { return statusOf(cl.ir.Props) })},
			ResponsibleRoles:     mergeRoles(c.claims, func(cl claim) []oscal.ResponsibleRole { return cl.ir.ResponsibleRoles }),
		}
		mine, mineOrder := g.componentParams(ctl, ctlParams, c)
		for _, canon := range mineOrder {
			vals := mine[canon]
			bc.SetParameters = append(bc.SetParameters, oscal.SetParameter{ParamID: canon, Values: vals})
			if prev, seen := values[canon]; seen {
				if !slices.Equal(prev, vals) {
					g.warn("conflicting-param", "%s: %s set to %q by %s and %q by %s; keeping the first", ctl.ID, canon, prev, valueFrom[canon], vals, c.componentName)
				}
				continue
			}
			values[canon] = vals
			valueFrom[canon] = c.componentName
			order = append(order, canon)
		}
		ir.ByComponents = append(ir.ByComponents, bc)

		// Statement-level answers, grouped by statement id so that this
		// component gets one by-component per statement.
		stmtClaims := map[string][]stmtClaim{}
		var stmtOrder []string
		for _, cl := range c.claims {
			for _, st := range cl.ir.Statements {
				if !partIDs[st.StatementID] {
					g.warn("unknown-statement", "component %q describes %s, which is not a part of %s", c.componentName, st.StatementID, ctl.ID)
					continue
				}
				if _, seen := stmtClaims[st.StatementID]; !seen {
					stmtOrder = append(stmtOrder, st.StatementID)
				}
				stmtClaims[st.StatementID] = append(stmtClaims[st.StatementID], stmtClaim{st: st, irProps: cl.ir.Props})
			}
		}
		for _, sid := range stmtOrder {
			scs := stmtClaims[sid]
			i, ok := stmtIdx[sid]
			if !ok {
				i = len(ir.Statements)
				stmtIdx[sid] = i
				ir.Statements = append(ir.Statements, oscal.SSPStatement{
					StatementID: sid,
					UUID:        g.opts.UUIDs.New("stmt:" + ctl.ID + ":" + sid),
				})
			}
			ir.Statements[i].ByComponents = append(ir.Statements[i].ByComponents, oscal.ByComponent{
				ComponentUUID:        c.componentUUID,
				UUID:                 g.opts.UUIDs.New("stmt-bc:" + sid + ":" + c.componentUUID),
				Description:          joinDistinct(scs, func(sc stmtClaim) string { return sc.st.Description }),
				ImplementationStatus: &oscal.ImplementationStatus{State: mergedStatus(g, sid, c, scs, func(sc stmtClaim) string { return statusOf(sc.st.Props, sc.irProps) })},
				ResponsibleRoles:     mergeRoles(scs, func(sc stmtClaim) []oscal.ResponsibleRole { return sc.st.ResponsibleRoles }),
			})
		}
	}
	for _, id := range order {
		ir.SetParameters = append(ir.SetParameters, oscal.SetParameter{ParamID: id, Values: values[id]})
	}
	return ir
}

// stmtClaim is one statement answer of a component, with the props of its
// implemented-requirement (which supply the default status).
type stmtClaim struct {
	st      oscal.ComponentStatement
	irProps []oscal.Property
}

// componentParams returns the parameter values one component sets on ctl,
// canonicalized and in first-seen order. Within a requirement,
// requirement-level values override control-implementation-level values;
// across the component's requirements for the same control, the first
// value wins and a different later value is reported.
func (g *generator) componentParams(ctl *oscal.Control, ctlParams map[string]bool, c contribution) (map[string][]string, []string) {
	mine := map[string][]string{}
	var mineOrder []string
	for _, cl := range c.claims {
		// Control-implementation-level values apply to every control that
		// has the parameter; requirement-level values are specific to this
		// control and are reported when they do not fit it.
		params := make([]oscal.SetParameter, 0, len(cl.ciParams)+len(cl.ir.SetParameters))
		params = append(params, cl.ciParams...)
		params = append(params, cl.ir.SetParameters...)
		claimVals := map[string][]string{}
		var claimOrder []string
		for i, sp := range params {
			canon, ok := g.render.Canonical(sp.ParamID)
			if !ok || !ctlParams[canon] {
				if i >= len(cl.ciParams) {
					g.warn("unknown-param", "component %q sets %s on %s, which has no such parameter", c.componentName, sp.ParamID, ctl.ID)
				}
				continue
			}
			if _, seen := claimVals[canon]; !seen {
				claimOrder = append(claimOrder, canon)
			}
			claimVals[canon] = slices.Clone(sp.Values) // requirement level overrides control-implementation level
		}
		for _, canon := range claimOrder {
			vals := claimVals[canon]
			if prev, seen := mine[canon]; seen {
				if !slices.Equal(prev, vals) {
					g.warn("conflicting-param", "%s: %s set to %q and %q by %s (source %s); keeping the first", ctl.ID, canon, prev, vals, c.componentName, cl.source)
				}
				continue
			}
			mine[canon] = vals
			mineOrder = append(mineOrder, canon)
		}
	}
	return mine, mineOrder
}

// statusRank orders implementation states from most to least complete, for
// merging one component's claims; values outside OSCAL's allowed set rank
// last (the semantic checker reports them).
var statusRank = map[string]int{"implemented": 0, "not-applicable": 1, "alternative": 2, "partial": 3, "planned": 4}

func rankOf(s string) int {
	if r, ok := statusRank[s]; ok {
		return r
	}
	return len(statusRank)
}

// mergedStatus returns the least complete status among one component's
// claims for the same control or statement, reporting a disagreement.
func mergedStatus[T any](g *generator, target string, c contribution, items []T, status func(T) string) string {
	w := status(items[0])
	differ := false
	for _, it := range items[1:] {
		s := status(it)
		if s != w {
			differ = true
		}
		if rankOf(s) > rankOf(w) {
			w = s
		}
	}
	if differ {
		g.warn("conflicting-status", "%s: component %q gives different implementation statuses; keeping the least complete, %q", target, c.componentName, w)
	}
	return w
}

// joinDistinct joins the distinct non-empty descriptions of items, in
// order, separated by a blank line.
func joinDistinct[T any](items []T, desc func(T) string) string {
	var out []string
	for _, it := range items {
		if d := desc(it); d != "" && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return strings.Join(out, "\n\n")
}

// mergeRoles unions the responsible roles of items by role id (the first
// occurrence of a role wins), without party references.
func mergeRoles[T any](items []T, roles func(T) []oscal.ResponsibleRole) []oscal.ResponsibleRole {
	var out []oscal.ResponsibleRole
	for _, it := range items {
		for _, rr := range stripParties(roles(it)) {
			if !slices.ContainsFunc(out, func(o oscal.ResponsibleRole) bool { return o.RoleID == rr.RoleID }) {
				out = append(out, rr)
			}
		}
	}
	return out
}

// uncovered builds a planned this-system requirement for an unaddressed
// control.
func (g *generator) uncovered(ctl *oscal.Control, thisSystem string) oscal.SSPImplementedRequirement {
	return oscal.SSPImplementedRequirement{
		UUID:      g.opts.UUIDs.New("ir:" + ctl.ID),
		ControlID: ctl.ID,
		ByComponents: []oscal.ByComponent{{
			ComponentUUID:        thisSystem,
			UUID:                 g.opts.UUIDs.New("bc:" + ctl.ID + ":this-system"),
			Description:          fmt.Sprintf("%s Describe how the system implements %s (%s).", Placeholder, strings.ToUpper(ctl.Label()), ctl.Title),
			ImplementationStatus: &oscal.ImplementationStatus{State: "planned"},
		}},
	}
}

// statusOf reads an "implementation-status" prop (the first prop list that
// has one wins), defaulting to "implemented".
func statusOf(propLists ...[]oscal.Property) string {
	for _, props := range propLists {
		if v, ok := oscal.Prop(props, "implementation-status"); ok && v != "" {
			return v
		}
	}
	return "implemented"
}

func controlParamIDs(ctl *oscal.Control) map[string]bool {
	ids := map[string]bool{}
	for _, p := range ctl.Params {
		ids[p.ID] = true
	}
	return ids
}

func partIDSet(ctl *oscal.Control) map[string]bool {
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

// stripParties drops party references, since the parties of a component
// definition are not carried into the SSP skeleton.
func stripParties(rrs []oscal.ResponsibleRole) []oscal.ResponsibleRole {
	if len(rrs) == 0 {
		return nil
	}
	out := make([]oscal.ResponsibleRole, len(rrs))
	for i, rr := range rrs {
		rr.PartyUUIDs = nil
		rr.Props = slices.Clone(rr.Props)
		rr.Links = slices.Clone(rr.Links)
		out[i] = rr
	}
	return out
}

// titleFromID turns "system-owner" into "System Owner".
func titleFromID(id string) string {
	words := strings.FieldsFunc(id, func(r rune) bool { return r == '-' || r == '_' })
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	if len(words) == 0 {
		return id
	}
	return strings.Join(words, " ")
}
