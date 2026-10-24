package oscal

// Profile is an OSCAL profile: a selection and tailoring of controls from
// one or more catalogs or other profiles.
type Profile struct {
	UUID       string      `json:"uuid"`
	Metadata   Metadata    `json:"metadata"`
	Imports    []Import    `json:"imports"`
	Merge      *Merge      `json:"merge,omitempty"`
	Modify     *Modify     `json:"modify,omitempty"`
	BackMatter *BackMatter `json:"back-matter,omitempty"`
}

// IncludeAll is the empty include-all directive.
type IncludeAll struct{}

// Matching selects controls whose id matches a glob pattern.
type Matching struct {
	Pattern string `json:"pattern,omitempty"`
	Remarks string `json:"remarks,omitempty"`
}

// SelectControlByID is an include-controls / exclude-controls directive.
type SelectControlByID struct {
	// WithChildControls is "yes" or "no" (default "no").
	WithChildControls string     `json:"with-child-controls,omitempty"`
	WithIDs           []string   `json:"with-ids,omitempty"`
	Matching          []Matching `json:"matching,omitempty"`
}

// Import names a catalog or profile and the controls to take from it.
type Import struct {
	Href            string              `json:"href"`
	IncludeAll      *IncludeAll         `json:"include-all,omitempty"`
	IncludeControls []SelectControlByID `json:"include-controls,omitempty"`
	ExcludeControls []SelectControlByID `json:"exclude-controls,omitempty"`
}

// Combine sets the rule for controls with clashing ids.
type Combine struct {
	// Method is "use-first", "keep" (default) or the deprecated "merge".
	Method string `json:"method,omitempty"`
}

// Flat is the empty "flat" structuring directive.
type Flat struct{}

// InsertControls places selected controls into a custom structure.
type InsertControls struct {
	// Order is "keep" (default), "ascending" or "descending".
	Order           string              `json:"order,omitempty"`
	IncludeAll      *IncludeAll         `json:"include-all,omitempty"`
	IncludeControls []SelectControlByID `json:"include-controls,omitempty"`
	ExcludeControls []SelectControlByID `json:"exclude-controls,omitempty"`
}

// CustomGroup is a group declared in a custom merge structure.
type CustomGroup struct {
	ID             string           `json:"id,omitempty"`
	Class          string           `json:"class,omitempty"`
	Title          string           `json:"title"`
	Params         []Parameter      `json:"params,omitempty"`
	Props          []Property       `json:"props,omitempty"`
	Links          []Link           `json:"links,omitempty"`
	Parts          []Part           `json:"parts,omitempty"`
	Groups         []CustomGroup    `json:"groups,omitempty"`
	InsertControls []InsertControls `json:"insert-controls,omitempty"`
}

// Custom is the "custom" structuring directive.
type Custom struct {
	Groups         []CustomGroup    `json:"groups,omitempty"`
	InsertControls []InsertControls `json:"insert-controls,omitempty"`
}

// Merge controls how imported controls are combined and structured.
type Merge struct {
	Combine *Combine `json:"combine,omitempty"`
	Flat    *Flat    `json:"flat,omitempty"`
	AsIs    *bool    `json:"as-is,omitempty"`
	Custom  *Custom  `json:"custom,omitempty"`
}

// ProfileSetParameter changes a parameter in the resolved catalog.
type ProfileSetParameter struct {
	ParamID     string                `json:"param-id"`
	Class       string                `json:"class,omitempty"`
	DependsOn   string                `json:"depends-on,omitempty"`
	Props       []Property            `json:"props,omitempty"`
	Links       []Link                `json:"links,omitempty"`
	Label       string                `json:"label,omitempty"`
	Usage       string                `json:"usage,omitempty"`
	Constraints []ParameterConstraint `json:"constraints,omitempty"`
	Guidelines  []ParameterGuideline  `json:"guidelines,omitempty"`
	Values      []string              `json:"values,omitempty"`
	Select      *ParameterSelection   `json:"select,omitempty"`
}

// Remove deletes matching objects from inside a control.
type Remove struct {
	ByName     string `json:"by-name,omitempty"`
	ByClass    string `json:"by-class,omitempty"`
	ByID       string `json:"by-id,omitempty"`
	ByItemName string `json:"by-item-name,omitempty"`
	ByNS       string `json:"by-ns,omitempty"`
	Remarks    string `json:"remarks,omitempty"`
}

// Add inserts content into a control, or relative to an object inside it.
type Add struct {
	// Position is "before", "after", "starting" or "ending" (default).
	Position string      `json:"position,omitempty"`
	ByID     string      `json:"by-id,omitempty"`
	Title    string      `json:"title,omitempty"`
	Params   []Parameter `json:"params,omitempty"`
	Props    []Property  `json:"props,omitempty"`
	Links    []Link      `json:"links,omitempty"`
	Parts    []Part      `json:"parts,omitempty"`
}

// Alter changes the content of one control.
type Alter struct {
	ControlID string   `json:"control-id"`
	Removes   []Remove `json:"removes,omitempty"`
	Adds      []Add    `json:"adds,omitempty"`
}

// Modify holds the parameter settings and control alterations of a profile.
type Modify struct {
	SetParameters []ProfileSetParameter `json:"set-parameters,omitempty"`
	Alters        []Alter               `json:"alters,omitempty"`
}
