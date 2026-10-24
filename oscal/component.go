package oscal

import "encoding/json"

// ComponentDefinition describes reusable components (software, services,
// policies, ...) and the controls they help satisfy.
type ComponentDefinition struct {
	UUID                       string             `json:"uuid"`
	Metadata                   Metadata           `json:"metadata"`
	ImportComponentDefinitions json.RawMessage    `json:"import-component-definitions,omitempty"`
	Components                 []DefinedComponent `json:"components,omitempty"`
	Capabilities               []Capability       `json:"capabilities,omitempty"`
	BackMatter                 *BackMatter        `json:"back-matter,omitempty"`
}

// DefinedComponent is a component and its control implementations.
type DefinedComponent struct {
	UUID                   string                  `json:"uuid"`
	Type                   string                  `json:"type"`
	Title                  string                  `json:"title"`
	Description            string                  `json:"description"`
	Purpose                string                  `json:"purpose,omitempty"`
	Props                  []Property              `json:"props,omitempty"`
	Links                  []Link                  `json:"links,omitempty"`
	ResponsibleRoles       []ResponsibleRole       `json:"responsible-roles,omitempty"`
	Protocols              json.RawMessage         `json:"protocols,omitempty"`
	ControlImplementations []ControlImplementation `json:"control-implementations,omitempty"`
	Remarks                string                  `json:"remarks,omitempty"`
}

// Capability is a grouping of components that together provide a function.
type Capability struct {
	UUID                   string                  `json:"uuid"`
	Name                   string                  `json:"name"`
	Description            string                  `json:"description"`
	Props                  []Property              `json:"props,omitempty"`
	Links                  []Link                  `json:"links,omitempty"`
	IncorporatesComponents json.RawMessage         `json:"incorporates-components,omitempty"`
	ControlImplementations []ControlImplementation `json:"control-implementations,omitempty"`
	Remarks                string                  `json:"remarks,omitempty"`
}

// ControlImplementation is a set of implemented requirements against one
// catalog or profile (the source).
type ControlImplementation struct {
	UUID                    string                            `json:"uuid"`
	Source                  string                            `json:"source"`
	Description             string                            `json:"description"`
	Props                   []Property                        `json:"props,omitempty"`
	Links                   []Link                            `json:"links,omitempty"`
	SetParameters           []SetParameter                    `json:"set-parameters,omitempty"`
	ImplementedRequirements []ComponentImplementedRequirement `json:"implemented-requirements"`
}

// ComponentImplementedRequirement describes how a component addresses one
// control.
type ComponentImplementedRequirement struct {
	UUID             string               `json:"uuid"`
	ControlID        string               `json:"control-id"`
	Description      string               `json:"description"`
	Props            []Property           `json:"props,omitempty"`
	Links            []Link               `json:"links,omitempty"`
	SetParameters    []SetParameter       `json:"set-parameters,omitempty"`
	ResponsibleRoles []ResponsibleRole    `json:"responsible-roles,omitempty"`
	Statements       []ComponentStatement `json:"statements,omitempty"`
	Remarks          string               `json:"remarks,omitempty"`
}

// ComponentStatement describes how a component addresses one statement
// (sub-requirement) of a control.
type ComponentStatement struct {
	StatementID      string            `json:"statement-id"`
	UUID             string            `json:"uuid"`
	Description      string            `json:"description"`
	Props            []Property        `json:"props,omitempty"`
	Links            []Link            `json:"links,omitempty"`
	ResponsibleRoles []ResponsibleRole `json:"responsible-roles,omitempty"`
	Remarks          string            `json:"remarks,omitempty"`
}
