package oscal

import "slices"

// Catalog is an OSCAL catalog: a structured set of controls.
type Catalog struct {
	UUID       string      `json:"uuid"`
	Metadata   Metadata    `json:"metadata"`
	Params     []Parameter `json:"params,omitempty"`
	Controls   []Control   `json:"controls,omitempty"`
	Groups     []Group     `json:"groups,omitempty"`
	BackMatter *BackMatter `json:"back-matter,omitempty"`
}

// Group is a grouping of controls (e.g. a SP 800-53 family) or of groups.
type Group struct {
	ID       string      `json:"id,omitempty"`
	Class    string      `json:"class,omitempty"`
	Title    string      `json:"title"`
	Params   []Parameter `json:"params,omitempty"`
	Props    []Property  `json:"props,omitempty"`
	Links    []Link      `json:"links,omitempty"`
	Parts    []Part      `json:"parts,omitempty"`
	Groups   []Group     `json:"groups,omitempty"`
	Controls []Control   `json:"controls,omitempty"`
}

// Control is a safeguard or countermeasure; child controls are typically
// control enhancements.
type Control struct {
	ID       string      `json:"id"`
	Class    string      `json:"class,omitempty"`
	Title    string      `json:"title"`
	Params   []Parameter `json:"params,omitempty"`
	Props    []Property  `json:"props,omitempty"`
	Links    []Link      `json:"links,omitempty"`
	Parts    []Part      `json:"parts,omitempty"`
	Controls []Control   `json:"controls,omitempty"`
}

// ControlRef locates a control within a catalog.
type ControlRef struct {
	Control *Control
	// ParentID is the id of the enclosing control, or "" if the control is
	// directly under a group or the catalog.
	ParentID string
	// GroupPath lists the ids of the enclosing groups, outermost first.
	GroupPath []string
	// Order is the control's position in a depth-first document-order walk.
	Order int
}

// WalkControls visits every control in document order (depth first, groups
// before their nested content, parents before children). Returning false
// from fn stops the walk.
func (c *Catalog) WalkControls(fn func(ref ControlRef) bool) {
	order := 0
	var walkControls func(cs []Control, parent string, groups []string) bool
	walkControls = func(cs []Control, parent string, groups []string) bool {
		for i := range cs {
			ref := ControlRef{Control: &cs[i], ParentID: parent, GroupPath: groups, Order: order}
			order++
			if !fn(ref) {
				return false
			}
			if !walkControls(cs[i].Controls, cs[i].ID, groups) {
				return false
			}
		}
		return true
	}
	var walkGroups func(gs []Group, path []string) bool
	walkGroups = func(gs []Group, path []string) bool {
		for i := range gs {
			p := append(slices.Clip(path), gs[i].ID)
			if !walkControls(gs[i].Controls, "", p) {
				return false
			}
			if !walkGroups(gs[i].Groups, p) {
				return false
			}
		}
		return true
	}
	if !walkControls(c.Controls, "", nil) {
		return
	}
	walkGroups(c.Groups, nil)
}

// ControlIndex maps control ids to their locations. When ids are duplicated
// the first occurrence in document order wins.
func (c *Catalog) ControlIndex() map[string]ControlRef {
	idx := map[string]ControlRef{}
	c.WalkControls(func(ref ControlRef) bool {
		if _, dup := idx[ref.Control.ID]; !dup {
			idx[ref.Control.ID] = ref
		}
		return true
	})
	return idx
}

// ControlIDs returns all control ids in document order.
func (c *Catalog) ControlIDs() []string {
	var ids []string
	c.WalkControls(func(ref ControlRef) bool {
		ids = append(ids, ref.Control.ID)
		return true
	})
	return ids
}

// Control returns the control with the given id.
func (c *Catalog) Control(id string) (*Control, bool) {
	var found *Control
	c.WalkControls(func(ref ControlRef) bool {
		if ref.Control.ID == id {
			found = ref.Control
			return false
		}
		return true
	})
	return found, found != nil
}

// AllParams returns every parameter in the catalog (loose params, group
// params and control params, recursively) keyed by id. First wins on clashes.
func (c *Catalog) AllParams() map[string]*Parameter {
	out := map[string]*Parameter{}
	add := func(ps []Parameter) {
		for i := range ps {
			if _, dup := out[ps[i].ID]; !dup {
				out[ps[i].ID] = &ps[i]
			}
		}
	}
	add(c.Params)
	var walkGroups func(gs []Group)
	walkGroups = func(gs []Group) {
		for i := range gs {
			add(gs[i].Params)
			walkGroups(gs[i].Groups)
		}
	}
	walkGroups(c.Groups)
	c.WalkControls(func(ref ControlRef) bool {
		add(ref.Control.Params)
		return true
	})
	return out
}

// Clone returns a deep copy of the control, including child controls.
func (c Control) Clone() Control {
	c.Params = cloneParams(c.Params)
	c.Props = slices.Clone(c.Props)
	c.Links = slices.Clone(c.Links)
	c.Parts = cloneParts(c.Parts)
	if c.Controls != nil {
		cs := make([]Control, len(c.Controls))
		for i := range c.Controls {
			cs[i] = c.Controls[i].Clone()
		}
		c.Controls = cs
	}
	return c
}

// CloneShallow returns a deep copy of the control without its child controls.
func (c Control) CloneShallow() Control {
	c.Controls = nil
	return c.Clone()
}

// Clone returns a deep copy of the group's own content (title, params,
// props, links, parts) without nested groups or controls.
func (g Group) Clone() Group {
	g.Params = cloneParams(g.Params)
	g.Props = slices.Clone(g.Props)
	g.Links = slices.Clone(g.Links)
	g.Parts = cloneParts(g.Parts)
	g.Groups = nil
	g.Controls = nil
	return g
}

// Clone returns a deep copy of the part.
func (p Part) Clone() Part {
	p.Props = slices.Clone(p.Props)
	p.Links = slices.Clone(p.Links)
	p.Parts = cloneParts(p.Parts)
	return p
}

// Clone returns a deep copy of the parameter.
func (p Parameter) Clone() Parameter {
	p.Props = slices.Clone(p.Props)
	p.Links = slices.Clone(p.Links)
	if p.Constraints != nil {
		cs := make([]ParameterConstraint, len(p.Constraints))
		for i, c := range p.Constraints {
			c.Tests = slices.Clone(c.Tests)
			cs[i] = c
		}
		p.Constraints = cs
	}
	p.Guidelines = slices.Clone(p.Guidelines)
	p.Values = slices.Clone(p.Values)
	if p.Select != nil {
		s := *p.Select
		s.Choice = slices.Clone(s.Choice)
		p.Select = &s
	}
	return p
}

func cloneParams(ps []Parameter) []Parameter {
	if ps == nil {
		return nil
	}
	out := make([]Parameter, len(ps))
	for i := range ps {
		out[i] = ps[i].Clone()
	}
	return out
}

func cloneParts(ps []Part) []Part {
	if ps == nil {
		return nil
	}
	out := make([]Part, len(ps))
	for i := range ps {
		out[i] = ps[i].Clone()
	}
	return out
}

// Prop returns the value of the first prop with the given name.
func Prop(props []Property, name string) (string, bool) {
	for _, p := range props {
		if p.Name == name {
			return p.Value, true
		}
	}
	return "", false
}

// Label returns a control's display label (the "label" prop without a
// class, falling back to any label, then to the id).
func (c *Control) Label() string {
	for _, p := range c.Props {
		if p.Name == "label" && p.Class == "" {
			return p.Value
		}
	}
	if v, ok := Prop(c.Props, "label"); ok {
		return v
	}
	return c.ID
}

// IsWithdrawn reports whether the control carries status=withdrawn.
func (c *Control) IsWithdrawn() bool {
	v, _ := Prop(c.Props, "status")
	return v == "withdrawn"
}

// StatementPart returns the control's top-level "statement" part, if any.
func (c *Control) StatementPart() (*Part, bool) {
	for i := range c.Parts {
		if c.Parts[i].Name == "statement" {
			return &c.Parts[i], true
		}
	}
	return nil, false
}
