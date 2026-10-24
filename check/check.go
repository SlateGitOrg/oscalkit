// Package check performs semantic validation of OSCAL documents: the
// referential-integrity and allowed-value rules that the OSCAL metaschema
// defines but the JSON schemas cannot express (unique ids and uuids,
// resolvable "#fragment" links and parameter inserts, references between
// components, roles, parties and controls).
package check

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/archit2501/oscalkit/oscal"
	"github.com/archit2501/oscalkit/param"
)

// Severity of a finding.
type Severity string

// Severities, most severe first.
const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

// Finding is one semantic problem.
type Finding struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	// Path locates the problem (a JSON Pointer or a descriptive path).
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (f Finding) String() string {
	return fmt.Sprintf("%s %s %s: %s", f.Severity, f.Code, f.Path, f.Message)
}

// ImplementationStates are the allowed implementation-status values.
var ImplementationStates = []string{"implemented", "partial", "planned", "alternative", "not-applicable"}

var uuidPattern = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)

type checker struct {
	findings []Finding
}

func (c *checker) add(sev Severity, code, path, format string, args ...any) {
	c.findings = append(c.findings, Finding{Severity: sev, Code: code, Path: path, Message: fmt.Sprintf(format, args...)})
}

// Document runs every semantic check applicable to the document's model.
// Findings are sorted by severity, then path.
func Document(doc *oscal.Document) []Finding {
	c := &checker{}
	g := c.generic(doc)
	c.metadata(doc)
	switch {
	case doc.Catalog != nil:
		c.catalog(doc.Catalog, g)
	case doc.Profile != nil:
		c.profile(doc.Profile)
	case doc.ComponentDefinition != nil:
		c.componentDefinition(doc.ComponentDefinition)
	case doc.SSP != nil:
		c.ssp(doc.SSP)
	}
	sort.SliceStable(c.findings, func(i, j int) bool {
		a, b := c.findings[i], c.findings[j]
		if a.Severity != b.Severity {
			return a.Severity == Error
		}
		return a.Path < b.Path
	})
	return c.findings
}

// Errors counts findings with error severity.
func Errors(fs []Finding) int {
	n := 0
	for _, f := range fs {
		if f.Severity == Error {
			n++
		}
	}
	return n
}

// genericInfo is what the model-independent pass collects.
type genericInfo struct {
	uuids     map[string]string // uuid -> first path
	fragments []fragmentRef     // "#x" link targets
}

type fragmentRef struct {
	target string
	path   string
}

// generic walks the raw JSON tree: it flags duplicate "uuid" definitions and
// collects "#fragment" link targets for later resolution.
func (c *checker) generic(doc *oscal.Document) genericInfo {
	g := genericInfo{uuids: map[string]string{}}
	var root any
	if err := json.Unmarshal(doc.JSON, &root); err != nil {
		c.add(Error, "invalid-json", "", "document is not valid JSON: %v", err)
		return g
	}
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch x := v.(type) {
		case map[string]any:
			if u, ok := x["uuid"].(string); ok {
				if first, dup := g.uuids[u]; dup {
					c.add(Error, "duplicate-uuid", path, "uuid %s is already defined at %s", u, first)
				} else {
					g.uuids[u] = path
				}
			}
			if strings.HasSuffix(path, "/links") || strings.Contains(path, "/links/") {
				if h, ok := x["href"].(string); ok && strings.HasPrefix(h, "#") {
					g.fragments = append(g.fragments, fragmentRef{target: strings.TrimPrefix(h, "#"), path: path})
				}
			}
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(x[k], path+"/"+escape(k))
			}
		case []any:
			for i, e := range x {
				walk(e, fmt.Sprintf("%s/%d", path, i))
			}
		}
	}
	walk(root, "")
	return g
}

func escape(tok string) string {
	return strings.ReplaceAll(strings.ReplaceAll(tok, "~", "~0"), "/", "~1")
}

func metadataOf(doc *oscal.Document) (*oscal.Metadata, string) {
	switch {
	case doc.Catalog != nil:
		return &doc.Catalog.Metadata, "/catalog/metadata"
	case doc.Profile != nil:
		return &doc.Profile.Metadata, "/profile/metadata"
	case doc.ComponentDefinition != nil:
		return &doc.ComponentDefinition.Metadata, "/component-definition/metadata"
	case doc.SSP != nil:
		return &doc.SSP.Metadata, "/system-security-plan/metadata"
	}
	return nil, ""
}

// metadata checks role/party definitions and responsible-party references.
func (c *checker) metadata(doc *oscal.Document) {
	md, base := metadataOf(doc)
	if md == nil {
		return
	}
	roles := map[string]bool{}
	for i, r := range md.Roles {
		if roles[r.ID] {
			c.add(Error, "duplicate-role", fmt.Sprintf("%s/roles/%d", base, i), "role id %q is defined twice", r.ID)
		}
		roles[r.ID] = true
	}
	parties := map[string]bool{}
	for _, p := range md.Parties {
		parties[p.UUID] = true
	}
	for i, rp := range md.ResponsibleParties {
		path := fmt.Sprintf("%s/responsible-parties/%d", base, i)
		if !roles[rp.RoleID] {
			c.add(Error, "unknown-role", path, "responsible-party references undefined role %q", rp.RoleID)
		}
		for _, pu := range rp.PartyUUIDs {
			if !parties[pu] {
				c.add(Error, "unknown-party", path, "responsible-party references undefined party %s", pu)
			}
		}
	}
}

// catalog checks id uniqueness, link fragments and parameter inserts.
func (c *checker) catalog(cat *oscal.Catalog, g genericInfo) {
	ids := map[string]string{}
	define := func(kind, id, path string) {
		if id == "" {
			return
		}
		if first, dup := ids[id]; dup {
			c.add(Error, "duplicate-id", path, "%s id %q is already used by %s", kind, id, first)
			return
		}
		ids[id] = kind + " at " + path
	}
	params := map[string]bool{}
	var inserts []fragmentRef
	scanText := func(text, path string) {
		for _, id := range param.Refs(text) {
			inserts = append(inserts, fragmentRef{target: id, path: path})
		}
	}
	var walkParts func(ps []oscal.Part, path string)
	walkParts = func(ps []oscal.Part, path string) {
		for i, p := range ps {
			pp := fmt.Sprintf("%s/parts/%d", path, i)
			define("part", p.ID, pp)
			scanText(p.Prose, pp)
			walkParts(p.Parts, pp)
		}
	}
	walkParams := func(ps []oscal.Parameter, path string) {
		for i, p := range ps {
			pp := fmt.Sprintf("%s/params/%d", path, i)
			define("param", p.ID, pp)
			params[p.ID] = true
			if p.Select != nil {
				for _, ch := range p.Select.Choice {
					scanText(ch, pp)
				}
			}
			scanText(p.Label, pp)
		}
	}
	var walkControls func(cs []oscal.Control, path string)
	walkControls = func(cs []oscal.Control, path string) {
		for i, ctl := range cs {
			cp := fmt.Sprintf("%s/controls/%d", path, i)
			define("control", ctl.ID, cp)
			scanText(ctl.Title, cp)
			walkParams(ctl.Params, cp)
			walkParts(ctl.Parts, cp)
			walkControls(ctl.Controls, cp)
		}
	}
	var walkGroups func(gs []oscal.Group, path string)
	walkGroups = func(gs []oscal.Group, path string) {
		for i, gr := range gs {
			gp := fmt.Sprintf("%s/groups/%d", path, i)
			define("group", gr.ID, gp)
			walkParams(gr.Params, gp)
			walkParts(gr.Parts, gp)
			walkControls(gr.Controls, gp)
			walkGroups(gr.Groups, gp)
		}
	}
	walkParams(cat.Params, "/catalog")
	walkControls(cat.Controls, "/catalog")
	walkGroups(cat.Groups, "/catalog")

	for _, in := range inserts {
		if !params[in.target] {
			c.add(Error, "unknown-param-insert", in.path, "insert references undefined parameter %q", in.target)
		}
	}
	resolved := isResolved(cat)
	for _, f := range g.fragments {
		if _, ok := ids[f.target]; ok {
			continue
		}
		if _, ok := g.uuids[f.target]; ok {
			continue
		}
		switch {
		case uuidPattern.MatchString(f.target):
			c.add(Error, "broken-link", f.path, "link target #%s does not match any uuid in the catalog", f.target)
		case resolved:
			// A resolved profile keeps links to related controls that the
			// baseline did not select; they point into the source catalog.
		default:
			c.add(Warning, "broken-link", f.path, "link target #%s is not defined in the catalog", f.target)
		}
	}
}

// isResolved reports whether a catalog is the output of profile resolution.
func isResolved(cat *oscal.Catalog) bool {
	if _, ok := oscal.Prop(cat.Metadata.Props, "source-profile"); ok {
		return true
	}
	for _, l := range cat.Metadata.Links {
		if l.Rel == "source-profile" {
			return true
		}
	}
	return false
}

// profile checks import references and merge directives.
func (c *checker) profile(p *oscal.Profile) {
	for i, imp := range p.Imports {
		path := fmt.Sprintf("/profile/imports/%d", i)
		if strings.HasPrefix(imp.Href, "#") {
			if _, ok := p.BackMatter.Resource(strings.TrimPrefix(imp.Href, "#")); !ok {
				c.add(Error, "broken-import", path, "import href %s does not match a back-matter resource", imp.Href)
			}
		}
		for j, sel := range append(slices.Clone(imp.IncludeControls), imp.ExcludeControls...) {
			for k, m := range sel.Matching {
				if m.Pattern == "" {
					c.add(Warning, "empty-pattern", fmt.Sprintf("%s/selectors/%d/matching/%d", path, j, k), "matching without a pattern selects nothing")
				}
			}
		}
	}
	if m := p.Merge; m != nil {
		n := 0
		if m.Flat != nil {
			n++
		}
		if m.AsIs != nil {
			n++
		}
		if m.Custom != nil {
			n++
		}
		if n > 1 {
			c.add(Error, "multiple-structuring", "/profile/merge", "only one of flat, as-is and custom may be given")
		}
		if m.Combine != nil && m.Combine.Method == "merge" {
			c.add(Warning, "deprecated-combine", "/profile/merge/combine", "combine method \"merge\" is deprecated and undefined")
		}
	}
	if p.Modify != nil {
		seen := map[string]bool{}
		for i, a := range p.Modify.Alters {
			for j, add := range a.Adds {
				switch add.Position {
				case "", "before", "after", "starting", "ending":
				default:
					c.add(Error, "bad-position", fmt.Sprintf("/profile/modify/alters/%d/adds/%d", i, j), "unknown position %q", add.Position)
				}
			}
			if seen[a.ControlID] {
				c.add(Warning, "repeated-alter", fmt.Sprintf("/profile/modify/alters/%d", i), "control %q is altered more than once", a.ControlID)
			}
			seen[a.ControlID] = true
		}
	}
}

func checkRoles(c *checker, roles map[string]bool, rrs []oscal.ResponsibleRole, path string) {
	for _, rr := range rrs {
		if !roles[rr.RoleID] {
			c.add(Error, "unknown-role", path, "responsible-role references undefined role %q", rr.RoleID)
		}
	}
}

func roleSet(md oscal.Metadata) map[string]bool {
	roles := map[string]bool{}
	for _, r := range md.Roles {
		roles[r.ID] = true
	}
	return roles
}

// componentDefinition checks role references and per-requirement consistency.
func (c *checker) componentDefinition(cd *oscal.ComponentDefinition) {
	roles := roleSet(cd.Metadata)
	for i, comp := range cd.Components {
		cp := fmt.Sprintf("/component-definition/components/%d", i)
		checkRoles(c, roles, comp.ResponsibleRoles, cp)
		for j, ci := range comp.ControlImplementations {
			cip := fmt.Sprintf("%s/control-implementations/%d", cp, j)
			if len(ci.ImplementedRequirements) == 0 {
				c.add(Error, "empty-control-implementation", cip, "control-implementation has no implemented-requirements")
			}
			seen := map[string]bool{}
			for k, ir := range ci.ImplementedRequirements {
				irp := fmt.Sprintf("%s/implemented-requirements/%d", cip, k)
				if seen[ir.ControlID] {
					c.add(Warning, "duplicate-requirement", irp, "control %q is implemented twice in one control-implementation", ir.ControlID)
				}
				seen[ir.ControlID] = true
				checkRoles(c, roles, ir.ResponsibleRoles, irp)
				for l, st := range ir.Statements {
					if !strings.HasPrefix(st.StatementID, ir.ControlID+"_") {
						c.add(Warning, "statement-mismatch", fmt.Sprintf("%s/statements/%d", irp, l), "statement %q does not belong to control %q", st.StatementID, ir.ControlID)
					}
					checkRoles(c, roles, st.ResponsibleRoles, fmt.Sprintf("%s/statements/%d", irp, l))
				}
			}
		}
	}
}

// ssp checks component, role and user references and status values.
func (c *checker) ssp(s *oscal.SSP) {
	roles := roleSet(s.Metadata)
	comps := map[string]bool{}
	thisSystem := 0
	for i, comp := range s.SystemImplementation.Components {
		comps[comp.UUID] = true
		if comp.Type == "this-system" {
			thisSystem++
		}
		checkRoles(c, roles, comp.ResponsibleRoles, fmt.Sprintf("/system-security-plan/system-implementation/components/%d", i))
	}
	if thisSystem == 0 {
		c.add(Warning, "no-this-system", "/system-security-plan/system-implementation/components", "no component of type \"this-system\" is defined")
	}
	for i, u := range s.SystemImplementation.Users {
		for _, r := range u.RoleIDs {
			if !roles[r] {
				c.add(Error, "unknown-role", fmt.Sprintf("/system-security-plan/system-implementation/users/%d", i), "user references undefined role %q", r)
			}
		}
	}
	seen := map[string]int{}
	for i, ir := range s.ControlImplementation.ImplementedRequirements {
		irp := fmt.Sprintf("/system-security-plan/control-implementation/implemented-requirements/%d", i)
		if j, dup := seen[ir.ControlID]; dup {
			c.add(Warning, "duplicate-requirement", irp, "control %q is also implemented by requirement %d", ir.ControlID, j)
		} else {
			seen[ir.ControlID] = i
		}
		checkRoles(c, roles, ir.ResponsibleRoles, irp)
		c.byComponents(ir.ByComponents, comps, roles, irp)
		for j, st := range ir.Statements {
			sp := fmt.Sprintf("%s/statements/%d", irp, j)
			if !strings.HasPrefix(st.StatementID, ir.ControlID+"_") {
				c.add(Warning, "statement-mismatch", sp, "statement %q does not belong to control %q", st.StatementID, ir.ControlID)
			}
			checkRoles(c, roles, st.ResponsibleRoles, sp)
			c.byComponents(st.ByComponents, comps, roles, sp)
		}
	}
	if !uuidPattern.MatchString(s.UUID) {
		c.add(Error, "bad-uuid", "/system-security-plan/uuid", "%q is not a UUID", s.UUID)
	}
}

func (c *checker) byComponents(bcs []oscal.ByComponent, comps, roles map[string]bool, path string) {
	for k, bc := range bcs {
		bp := fmt.Sprintf("%s/by-components/%d", path, k)
		if !comps[bc.ComponentUUID] {
			c.add(Error, "unknown-component", bp, "by-component references component %s, which is not in system-implementation", bc.ComponentUUID)
		}
		if bc.ImplementationStatus != nil && !slices.Contains(ImplementationStates, bc.ImplementationStatus.State) {
			c.add(Error, "bad-implementation-status", bp, "implementation-status %q is not one of %v", bc.ImplementationStatus.State, ImplementationStates)
		}
		checkRoles(c, roles, bc.ResponsibleRoles, bp)
	}
}
