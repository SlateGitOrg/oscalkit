package resolve

import (
	"slices"

	"github.com/archit2501/oscalkit/oscal"
)

// defaultNS is the namespace of props and parts that declare none.
const defaultNS = "http://csrc.nist.gov/ns/oscal"

// modify applies set-parameters, then alters, in profile order.
func (r *resolver) modify(cat *oscal.Catalog, m *oscal.Modify) {
	for _, sp := range m.SetParameters {
		r.setParameter(cat, sp)
	}
	for _, a := range m.Alters {
		r.alter(cat, a)
	}
}

// paramsByID returns pointers to every parameter with the given id: loose,
// group-level and control-level (including child controls).
func paramsByID(cat *oscal.Catalog, id string) []*oscal.Parameter {
	var out []*oscal.Parameter
	collect := func(ps []oscal.Parameter) {
		for i := range ps {
			if ps[i].ID == id {
				out = append(out, &ps[i])
			}
		}
	}
	collect(cat.Params)
	var walkGroups func(gs []oscal.Group)
	walkGroups = func(gs []oscal.Group) {
		for i := range gs {
			collect(gs[i].Params)
			walkGroups(gs[i].Groups)
		}
	}
	walkGroups(cat.Groups)
	cat.WalkControls(func(ref oscal.ControlRef) bool {
		collect(ref.Control.Params)
		return true
	})
	return out
}

// setParameter applies one set-parameter directive. Scalar-like members
// (class, depends-on, label, usage, values, select) replace the target's
// (values and select replace each other);
// props, links, constraints and guidelines are appended, with a prop that
// carries a uuid replacing any existing prop with that uuid.
func (r *resolver) setParameter(cat *oscal.Catalog, sp oscal.ProfileSetParameter) {
	targets := paramsByID(cat, sp.ParamID)
	if len(targets) == 0 {
		r.warn("unknown-param", "set-parameter %q matches no parameter in the resolved catalog", sp.ParamID)
		return
	}
	for _, t := range targets {
		if sp.Class != "" {
			t.Class = sp.Class
		}
		if sp.DependsOn != "" {
			t.DependsOn = sp.DependsOn
		}
		if sp.Label != "" {
			t.Label = sp.Label
		}
		if sp.Usage != "" {
			t.Usage = sp.Usage
		}
		// OSCAL allows a parameter to carry values or a selection, not
		// both, so setting one replaces the other: values set on a
		// selection parameter are the selected choices.
		if sp.Values != nil {
			t.Values = slices.Clone(sp.Values)
			t.Select = nil
		}
		if sp.Select != nil {
			sel := *sp.Select
			sel.Choice = slices.Clone(sel.Choice)
			t.Select = &sel
			t.Values = nil
		}
		for _, p := range sp.Props {
			if p.UUID != "" {
				t.Props = slices.DeleteFunc(t.Props, func(x oscal.Property) bool { return x.UUID == p.UUID })
			}
			t.Props = append(t.Props, p)
		}
		for _, l := range sp.Links {
			if !slices.Contains(t.Links, l) {
				t.Links = append(t.Links, l)
			}
		}
		for _, c := range sp.Constraints {
			c.Tests = slices.Clone(c.Tests)
			t.Constraints = append(t.Constraints, c)
		}
		t.Guidelines = append(t.Guidelines, sp.Guidelines...)
	}
}

// controlsByID returns pointers to every control with the given id.
func controlsByID(cat *oscal.Catalog, id string) []*oscal.Control {
	var out []*oscal.Control
	cat.WalkControls(func(ref oscal.ControlRef) bool {
		if ref.Control.ID == id {
			out = append(out, ref.Control)
		}
		return true
	})
	return out
}

// alter applies one alter directive: all removes first, then all adds.
func (r *resolver) alter(cat *oscal.Catalog, a oscal.Alter) {
	targets := controlsByID(cat, a.ControlID)
	if len(targets) == 0 {
		r.warn("unknown-control", "alter targets control %q, which is not in the resolved catalog", a.ControlID)
		return
	}
	for _, ctl := range targets {
		for _, rm := range a.Removes {
			if rm == (oscal.Remove{Remarks: rm.Remarks}) {
				r.warn("empty-remove", "alter %q: remove without criteria matches nothing", a.ControlID)
				continue
			}
			if n := removeFromControl(ctl, rm); n == 0 {
				r.warn("inoperative-remove", "alter %q: remove %+v matched nothing", a.ControlID, rm)
			}
		}
		for _, add := range a.Adds {
			r.applyAdd(ctl, add)
		}
	}
}

func removeMatches(rm oscal.Remove, item, id, name, ns, class string) bool {
	if rm.ByItemName != "" && rm.ByItemName != item {
		return false
	}
	if rm.ByID != "" && rm.ByID != id {
		return false
	}
	if rm.ByName != "" && rm.ByName != name {
		return false
	}
	if rm.ByClass != "" && rm.ByClass != class {
		return false
	}
	if rm.ByNS != "" {
		eff := ns
		if eff == "" {
			eff = defaultNS
		}
		if eff != rm.ByNS {
			return false
		}
	}
	return true
}

// removeFromControl removes matching params, props, links and parts from a
// control's own content (recursing into params and parts, but not into
// child controls, which are altered by their own alter directives). It
// returns the number of objects removed.
func removeFromControl(c *oscal.Control, rm oscal.Remove) int {
	n := 0
	c.Params = slices.DeleteFunc(c.Params, func(p oscal.Parameter) bool {
		if removeMatches(rm, "param", p.ID, "", "", p.Class) {
			n++
			return true
		}
		return false
	})
	for i := range c.Params {
		n += removeProps(&c.Params[i].Props, rm)
		n += removeLinks(&c.Params[i].Links, rm)
	}
	n += removeProps(&c.Props, rm)
	n += removeLinks(&c.Links, rm)
	n += removeParts(&c.Parts, rm)
	return n
}

func removeProps(props *[]oscal.Property, rm oscal.Remove) int {
	n := 0
	*props = slices.DeleteFunc(*props, func(p oscal.Property) bool {
		if removeMatches(rm, "prop", "", p.Name, p.NS, p.Class) {
			n++
			return true
		}
		return false
	})
	if len(*props) == 0 {
		*props = nil
	}
	return n
}

func removeLinks(links *[]oscal.Link, rm oscal.Remove) int {
	n := 0
	*links = slices.DeleteFunc(*links, func(l oscal.Link) bool {
		if removeMatches(rm, "link", "", l.Rel, "", "") {
			n++
			return true
		}
		return false
	})
	if len(*links) == 0 {
		*links = nil
	}
	return n
}

func removeParts(parts *[]oscal.Part, rm oscal.Remove) int {
	n := 0
	*parts = slices.DeleteFunc(*parts, func(p oscal.Part) bool {
		if removeMatches(rm, "part", p.ID, p.Name, p.NS, p.Class) {
			n++
			return true
		}
		return false
	})
	for i := range *parts {
		p := &(*parts)[i]
		n += removeProps(&p.Props, rm)
		n += removeLinks(&p.Links, rm)
		n += removeParts(&p.Parts, rm)
	}
	if len(*parts) == 0 {
		*parts = nil
	}
	return n
}

// targetKind identifies what an add's by-id points at.
type targetKind int

const (
	targetParam targetKind = iota
	targetPart
	targetControl
)

// location is where an object with a given id lives inside a control.
type location struct {
	kind targetKind
	// owner is the control directly holding the target (for params,
	// top-level parts and child controls).
	owner *oscal.Control
	// parentPart is set when the target is a part nested inside another part.
	parentPart *oscal.Part
	index      int
}

func findInControl(c *oscal.Control, id string) *location {
	for i := range c.Params {
		if c.Params[i].ID == id {
			return &location{kind: targetParam, owner: c, index: i}
		}
	}
	for i := range c.Parts {
		if c.Parts[i].ID == id {
			return &location{kind: targetPart, owner: c, index: i}
		}
		if loc := findInPart(&c.Parts[i], id); loc != nil {
			return loc
		}
	}
	for i := range c.Controls {
		if c.Controls[i].ID == id {
			return &location{kind: targetControl, owner: c, index: i}
		}
		if loc := findInControl(&c.Controls[i], id); loc != nil {
			return loc
		}
	}
	return nil
}

func findInPart(p *oscal.Part, id string) *location {
	for i := range p.Parts {
		if p.Parts[i].ID == id {
			return &location{kind: targetPart, parentPart: p, index: i}
		}
		if loc := findInPart(&p.Parts[i], id); loc != nil {
			return loc
		}
	}
	return nil
}

// applyAdd performs one add directive against a control.
func (r *resolver) applyAdd(ctl *oscal.Control, add oscal.Add) {
	pos := add.Position
	if pos == "" {
		pos = "ending"
	}
	if add.ByID == "" || add.ByID == ctl.ID {
		// Implicit binding: before/after behave as starting/ending.
		addIntoControl(ctl, add, pos == "starting" || pos == "before")
		return
	}
	loc := findInControl(ctl, add.ByID)
	if loc == nil {
		r.warn("inoperative-add", "alter %q: add by-id %q matches nothing in the control", ctl.ID, add.ByID)
		return
	}
	switch pos {
	case "starting", "ending":
		r.addInside(ctl.ID, loc, add, pos == "starting")
	case "before", "after":
		r.addBeside(ctl.ID, loc, add, pos == "before")
	default:
		r.warn("bad-position", "alter %q: unknown add position %q", ctl.ID, pos)
	}
}

func addIntoControl(c *oscal.Control, add oscal.Add, atStart bool) {
	if add.Title != "" {
		c.Title = add.Title
	}
	c.Params = place(c.Params, cloneParams(add.Params), atStart)
	c.Props = place(c.Props, slices.Clone(add.Props), atStart)
	c.Links = place(c.Links, slices.Clone(add.Links), atStart)
	c.Parts = place(c.Parts, cloneParts(add.Parts), atStart)
}

// addInside adds content inside the target object (starting/ending).
func (r *resolver) addInside(ctlID string, loc *location, add oscal.Add, atStart bool) {
	switch loc.kind {
	case targetControl:
		addIntoControl(&loc.owner.Controls[loc.index], add, atStart)
	case targetPart:
		p := loc.part()
		if add.Title != "" {
			p.Title = add.Title
		}
		if len(add.Params) > 0 {
			r.warn("invalid-add", "alter %q: params cannot be added inside part %q", ctlID, p.ID)
		}
		p.Props = place(p.Props, slices.Clone(add.Props), atStart)
		p.Links = place(p.Links, slices.Clone(add.Links), atStart)
		p.Parts = place(p.Parts, cloneParts(add.Parts), atStart)
	case targetParam:
		prm := &loc.owner.Params[loc.index]
		if add.Title != "" || len(add.Params) > 0 || len(add.Parts) > 0 {
			r.warn("invalid-add", "alter %q: only props and links can be added inside param %q", ctlID, prm.ID)
		}
		prm.Props = place(prm.Props, slices.Clone(add.Props), atStart)
		prm.Links = place(prm.Links, slices.Clone(add.Links), atStart)
	}
}

// addBeside adds content next to the target (before/after). Objects of the
// target's own type are inserted adjacent to it; other objects go to the
// start (before) or end (after) of the container's list for their type.
func (r *resolver) addBeside(ctlID string, loc *location, add oscal.Add, before bool) {
	if add.Title != "" {
		r.warn("invalid-add", "alter %q: a title cannot be added before/after %q", ctlID, loc.describe())
	}
	at := loc.index + 1
	if before {
		at = loc.index
	}
	if loc.parentPart != nil {
		pp := loc.parentPart
		if len(add.Params) > 0 {
			r.warn("invalid-add", "alter %q: params cannot be added inside a part", ctlID)
		}
		pp.Parts = slices.Insert(pp.Parts, at, cloneParts(add.Parts)...)
		pp.Props = place(pp.Props, slices.Clone(add.Props), before)
		pp.Links = place(pp.Links, slices.Clone(add.Links), before)
		return
	}
	c := loc.owner
	switch loc.kind {
	case targetParam:
		c.Params = slices.Insert(c.Params, at, cloneParams(add.Params)...)
		c.Parts = place(c.Parts, cloneParts(add.Parts), before)
	case targetPart:
		c.Parts = slices.Insert(c.Parts, at, cloneParts(add.Parts)...)
		c.Params = place(c.Params, cloneParams(add.Params), before)
	case targetControl:
		c.Params = place(c.Params, cloneParams(add.Params), before)
		c.Parts = place(c.Parts, cloneParts(add.Parts), before)
	}
	c.Props = place(c.Props, slices.Clone(add.Props), before)
	c.Links = place(c.Links, slices.Clone(add.Links), before)
}

func (l *location) part() *oscal.Part {
	if l.parentPart != nil {
		return &l.parentPart.Parts[l.index]
	}
	return &l.owner.Parts[l.index]
}

func (l *location) describe() string {
	switch l.kind {
	case targetParam:
		return "param " + l.owner.Params[l.index].ID
	case targetControl:
		return "control " + l.owner.Controls[l.index].ID
	}
	return "part " + l.part().ID
}

// place prepends or appends items, returning nil for an empty result so
// that empty arrays are omitted from the JSON output.
func place[T any](dst, items []T, atStart bool) []T {
	if len(items) == 0 {
		return dst
	}
	if atStart {
		return append(items, dst...)
	}
	return append(dst, items...)
}

func cloneParts(ps []oscal.Part) []oscal.Part {
	if len(ps) == 0 {
		return nil
	}
	out := make([]oscal.Part, len(ps))
	for i := range ps {
		out[i] = ps[i].Clone()
	}
	return out
}
