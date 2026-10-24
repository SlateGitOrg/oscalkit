package param

import (
	"slices"
	"strings"

	"github.com/archit2501/oscalkit/oscal"
)

// maxDepth bounds recursive substitution (selection choices and aggregated
// params may themselves contain inserts).
const maxDepth = 8

// Renderer substitutes parameter inserts using the parameters of a
// (resolved) catalog, optionally overridden by values set elsewhere, such
// as an SSP's set-parameters.
type Renderer struct {
	params    map[string]*oscal.Parameter
	aliases   map[string]string // alt-identifier -> param id
	overrides map[string][]string
}

// NewRenderer indexes every parameter in cat.
func NewRenderer(cat *oscal.Catalog) *Renderer {
	r := &Renderer{
		params:    cat.AllParams(),
		aliases:   map[string]string{},
		overrides: map[string][]string{},
	}
	for id, p := range r.params {
		for _, pr := range p.Props {
			if pr.Name == "alt-identifier" && pr.Value != id {
				if _, taken := r.params[pr.Value]; !taken {
					r.aliases[pr.Value] = id
				}
			}
		}
	}
	return r
}

// Canonical maps a parameter id (or a legacy alt-identifier, as used by
// SSPs written against older SP 800-53 OSCAL content) to the catalog's id.
// ok is false when the id is unknown.
func (r *Renderer) Canonical(id string) (string, bool) {
	if _, ok := r.params[id]; ok {
		return id, true
	}
	if c, ok := r.aliases[id]; ok {
		return c, true
	}
	return id, false
}

// Param returns the catalog parameter with the given id or alias.
func (r *Renderer) Param(id string) (*oscal.Parameter, bool) {
	c, ok := r.Canonical(id)
	if !ok {
		return nil, false
	}
	return r.params[c], true
}

// SetValues overrides the value of a parameter (by id or alias). Later calls
// for the same parameter win.
func (r *Renderer) SetValues(id string, values []string) {
	c, _ := r.Canonical(id)
	r.overrides[c] = slices.Clone(values)
}

// Values returns the effective values for a parameter: an override if set,
// otherwise the catalog's values.
func (r *Renderer) Values(id string) []string {
	c, _ := r.Canonical(id)
	if v, ok := r.overrides[c]; ok {
		return v
	}
	if p, ok := r.params[c]; ok {
		return p.Values
	}
	return nil
}

// IsSet reports whether a parameter has an effective value. A parameter
// that aggregates others (prop name "aggregates") is also set when every
// aggregated parameter is set.
func (r *Renderer) IsSet(id string) bool {
	return r.isSet(id, 0)
}

func (r *Renderer) isSet(id string, depth int) bool {
	if len(r.Values(id)) > 0 {
		return true
	}
	if depth >= maxDepth {
		return false
	}
	agg := r.aggregated(id)
	if len(agg) == 0 {
		return false
	}
	for _, a := range agg {
		if !r.isSet(a, depth+1) {
			return false
		}
	}
	return true
}

func (r *Renderer) aggregated(id string) []string {
	p, ok := r.Param(id)
	if !ok {
		return nil
	}
	var out []string
	for _, pr := range p.Props {
		if pr.Name == "aggregates" {
			out = append(out, pr.Value)
		}
	}
	return out
}

// Text returns the substitution text for one parameter:
//   - its effective values (with any inserts inside them substituted),
//     comma separated;
//   - otherwise, for an aggregating parameter whose parts are all set, the
//     rendered parts joined by "; ";
//   - otherwise a selection "[Selection (one or more): a; b]";
//   - otherwise an assignment placeholder "[Assignment: <label>]".
func (r *Renderer) Text(id string) string {
	return r.text(id, 0)
}

func (r *Renderer) text(id string, depth int) string {
	if depth > maxDepth {
		return "[...]"
	}
	if v := r.Values(id); len(v) > 0 {
		// A value may be a selection choice that itself contains inserts.
		out := make([]string, len(v))
		for i, s := range v {
			out[i] = r.render(s, depth+1)
		}
		return strings.Join(out, ", ")
	}
	p, ok := r.Param(id)
	if !ok {
		return "[Unknown parameter: " + id + "]"
	}
	if agg := r.aggregated(id); len(agg) > 0 && r.isSet(id, depth) {
		parts := make([]string, 0, len(agg))
		for _, a := range agg {
			parts = append(parts, r.text(a, depth+1))
		}
		return strings.Join(parts, "; ")
	}
	if p.Select != nil && len(p.Select.Choice) > 0 {
		choices := make([]string, 0, len(p.Select.Choice))
		for _, c := range p.Select.Choice {
			choices = append(choices, r.render(c, depth+1))
		}
		head := "[Selection: "
		if p.Select.HowMany == "one-or-more" {
			head = "[Selection (one or more): "
		}
		return head + strings.Join(choices, "; ") + "]"
	}
	label := p.Label
	if label == "" {
		label = p.ID
	}
	return "[Assignment: " + r.render(label, depth+1) + "]"
}

// Render replaces every param insert in text with its substitution text.
// Inserts of other types are left untouched.
func (r *Renderer) Render(text string) string {
	return r.render(text, 0)
}

func (r *Renderer) render(text string, depth int) string {
	ins := FindInserts(text)
	if len(ins) == 0 {
		return text
	}
	var b strings.Builder
	last := 0
	for _, in := range ins {
		if in.Type != "param" {
			continue
		}
		b.WriteString(text[last:in.Start])
		b.WriteString(r.text(in.IDRef, depth))
		last = in.End
	}
	b.WriteString(text[last:])
	return b.String()
}

// Line is one rendered line of a control statement.
type Line struct {
	Depth int    // nesting depth, 0 for the statement's own prose
	Label string // e.g. "a." or "1."; may be empty
	Text  string // prose with parameters substituted
	// PartID is the id of the part the line came from.
	PartID string
}

// Statement renders a control's statement part as lines, depth first.
func (r *Renderer) Statement(c *oscal.Control) []Line {
	st, ok := c.StatementPart()
	if !ok {
		return nil
	}
	var lines []Line
	var walk func(p *oscal.Part, depth int)
	walk = func(p *oscal.Part, depth int) {
		if p.Prose != "" {
			label, _ := oscal.Prop(p.Props, "label")
			lines = append(lines, Line{Depth: depth, Label: label, Text: strings.TrimSpace(r.Render(p.Prose)), PartID: p.ID})
		}
		for i := range p.Parts {
			if p.Parts[i].Name == "item" || p.Parts[i].Name == "statement" {
				walk(&p.Parts[i], depth+1)
			}
		}
	}
	walk(st, 0)
	return lines
}

// StatementRefs returns the distinct parameter ids referenced by inserts in
// a control's statement, in order of first appearance.
func StatementRefs(c *oscal.Control) []string {
	st, ok := c.StatementPart()
	if !ok {
		return nil
	}
	var ids []string
	var walk func(p *oscal.Part)
	walk = func(p *oscal.Part) {
		for _, id := range Refs(p.Prose) {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		for i := range p.Parts {
			walk(&p.Parts[i])
		}
	}
	walk(st)
	return ids
}

// FormatMarkdown renders statement lines as an indented Markdown list.
func FormatMarkdown(lines []Line) string {
	var b strings.Builder
	for _, l := range lines {
		if l.Depth == 0 {
			b.WriteString(l.Text)
			b.WriteString("\n")
			continue
		}
		b.WriteString(strings.Repeat("  ", l.Depth-1))
		b.WriteString("- ")
		if l.Label != "" {
			b.WriteString(l.Label)
			b.WriteString(" ")
		}
		b.WriteString(l.Text)
		b.WriteString("\n")
	}
	return b.String()
}
