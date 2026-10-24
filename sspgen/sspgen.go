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

	"github.com/archit2501/oscalkit/oscal"
	"github.com/archit2501/oscalkit/param"
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

// contribution is one component's implementation of one control.
type contribution struct {
	componentUUID string
	componentName string
	ir            oscal.ComponentImplementedRequirement
	ciParams      []oscal.SetParameter
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
	baselineIdx := g.baseline.ControlIndex()
	usedUUIDs := map[string]bool{thisSystem.UUID: true}
	for _, cd := range cds {
		for _, comp := range cd.Components {
			uuid := comp.UUID
			if usedUUIDs[uuid] {
				g.warn("duplicate-component", "component %q reuses uuid %s; a new uuid was assigned", comp.Title, uuid)
				uuid = g.opts.UUIDs.New("component:" + comp.UUID + ":" + comp.Title)
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
					byControl[ir.ControlID] = append(byControl[ir.ControlID], contribution{
						componentUUID: uuid, componentName: comp.Title, ir: ir, ciParams: ci.SetParameters,
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
		bc := oscal.ByComponent{
			ComponentUUID:        c.componentUUID,
			UUID:                 g.opts.UUIDs.New("bc:" + ctl.ID + ":" + c.componentUUID),
			Description:          c.ir.Description,
			ImplementationStatus: &oscal.ImplementationStatus{State: statusOf(c.ir.Props)},
			ResponsibleRoles:     stripParties(c.ir.ResponsibleRoles),
		}
		// Control-implementation-level values apply to every control that
		// has the parameter; requirement-level values are specific to this
		// control and are reported when they do not fit it.
		params := make([]oscal.SetParameter, 0, len(c.ciParams)+len(c.ir.SetParameters))
		params = append(params, c.ciParams...)
		params = append(params, c.ir.SetParameters...)
		mine := map[string][]string{}
		var mineOrder []string
		for i, sp := range params {
			canon, ok := g.render.Canonical(sp.ParamID)
			if !ok || !ctlParams[canon] {
				if i >= len(c.ciParams) {
					g.warn("unknown-param", "component %q sets %s on %s, which has no such parameter", c.componentName, sp.ParamID, ctl.ID)
				}
				continue
			}
			if _, seen := mine[canon]; !seen {
				mineOrder = append(mineOrder, canon)
			}
			mine[canon] = slices.Clone(sp.Values) // requirement level overrides control-implementation level
		}
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
		for _, st := range c.ir.Statements {
			if !partIDs[st.StatementID] {
				g.warn("unknown-statement", "component %q describes %s, which is not a part of %s", c.componentName, st.StatementID, ctl.ID)
				continue
			}
			i, ok := stmtIdx[st.StatementID]
			if !ok {
				i = len(ir.Statements)
				stmtIdx[st.StatementID] = i
				ir.Statements = append(ir.Statements, oscal.SSPStatement{
					StatementID: st.StatementID,
					UUID:        g.opts.UUIDs.New("stmt:" + ctl.ID + ":" + st.StatementID),
				})
			}
			ir.Statements[i].ByComponents = append(ir.Statements[i].ByComponents, oscal.ByComponent{
				ComponentUUID:        c.componentUUID,
				UUID:                 g.opts.UUIDs.New("stmt-bc:" + st.StatementID + ":" + c.componentUUID),
				Description:          st.Description,
				ImplementationStatus: &oscal.ImplementationStatus{State: statusOf(st.Props, c.ir.Props)},
				ResponsibleRoles:     stripParties(st.ResponsibleRoles),
			})
		}
	}
	for _, id := range order {
		ir.SetParameters = append(ir.SetParameters, oscal.SetParameter{ParamID: id, Values: values[id]})
	}
	return ir
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
