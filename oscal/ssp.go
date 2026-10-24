package oscal

import "encoding/json"

// SSP is an OSCAL System Security Plan.
type SSP struct {
	UUID                  string                `json:"uuid"`
	Metadata              Metadata              `json:"metadata"`
	ImportProfile         ImportProfile         `json:"import-profile"`
	SystemCharacteristics SystemCharacteristics `json:"system-characteristics"`
	SystemImplementation  SystemImplementation  `json:"system-implementation"`
	ControlImplementation SSPControlImpl        `json:"control-implementation"`
	BackMatter            *BackMatter           `json:"back-matter,omitempty"`
}

// ImportProfile references the baseline profile the SSP implements.
type ImportProfile struct {
	Href    string `json:"href"`
	Remarks string `json:"remarks,omitempty"`
}

// SystemID is an identifier for the system.
type SystemID struct {
	IdentifierType string `json:"identifier-type,omitempty"`
	ID             string `json:"id"`
}

// Categorization ties an information type to a categorization system.
type Categorization struct {
	System             string   `json:"system"`
	InformationTypeIDs []string `json:"information-type-ids,omitempty"`
}

// Impact is a FIPS-199 style impact level for one security objective.
type Impact struct {
	Props                   []Property `json:"props,omitempty"`
	Links                   []Link     `json:"links,omitempty"`
	Base                    string     `json:"base"`
	Selected                string     `json:"selected,omitempty"`
	AdjustmentJustification string     `json:"adjustment-justification,omitempty"`
}

// InformationType is a type of information processed by the system.
type InformationType struct {
	UUID                  string           `json:"uuid,omitempty"`
	Title                 string           `json:"title"`
	Description           string           `json:"description"`
	Categorizations       []Categorization `json:"categorizations,omitempty"`
	Props                 []Property       `json:"props,omitempty"`
	Links                 []Link           `json:"links,omitempty"`
	ConfidentialityImpact *Impact          `json:"confidentiality-impact,omitempty"`
	IntegrityImpact       *Impact          `json:"integrity-impact,omitempty"`
	AvailabilityImpact    *Impact          `json:"availability-impact,omitempty"`
}

// SystemInformation lists the information types handled by the system.
type SystemInformation struct {
	Props            []Property        `json:"props,omitempty"`
	Links            []Link            `json:"links,omitempty"`
	InformationTypes []InformationType `json:"information-types"`
}

// SecurityImpactLevel is the overall FIPS-199 categorization.
type SecurityImpactLevel struct {
	Confidentiality string `json:"security-objective-confidentiality"`
	Integrity       string `json:"security-objective-integrity"`
	Availability    string `json:"security-objective-availability"`
}

// Status is a lifecycle status.
type Status struct {
	State   string `json:"state"`
	Remarks string `json:"remarks,omitempty"`
}

// AuthorizationBoundary describes the system's authorization boundary.
type AuthorizationBoundary struct {
	Description string          `json:"description"`
	Props       []Property      `json:"props,omitempty"`
	Links       []Link          `json:"links,omitempty"`
	Diagrams    json.RawMessage `json:"diagrams,omitempty"`
	Remarks     string          `json:"remarks,omitempty"`
}

// SystemCharacteristics describes the system under authorization.
type SystemCharacteristics struct {
	SystemIDs                []SystemID            `json:"system-ids"`
	SystemName               string                `json:"system-name"`
	SystemNameShort          string                `json:"system-name-short,omitempty"`
	Description              string                `json:"description"`
	Props                    []Property            `json:"props,omitempty"`
	Links                    []Link                `json:"links,omitempty"`
	DateAuthorized           string                `json:"date-authorized,omitempty"`
	SecuritySensitivityLevel string                `json:"security-sensitivity-level,omitempty"`
	SystemInformation        SystemInformation     `json:"system-information"`
	SecurityImpactLevel      *SecurityImpactLevel  `json:"security-impact-level,omitempty"`
	Status                   Status                `json:"status"`
	AuthorizationBoundary    AuthorizationBoundary `json:"authorization-boundary"`
	NetworkArchitecture      json.RawMessage       `json:"network-architecture,omitempty"`
	DataFlow                 json.RawMessage       `json:"data-flow,omitempty"`
	ResponsibleParties       []ResponsibleParty    `json:"responsible-parties,omitempty"`
	Remarks                  string                `json:"remarks,omitempty"`
}

// SystemUser is a type of user of the system.
type SystemUser struct {
	UUID                 string          `json:"uuid"`
	Title                string          `json:"title,omitempty"`
	ShortName            string          `json:"short-name,omitempty"`
	Description          string          `json:"description,omitempty"`
	Props                []Property      `json:"props,omitempty"`
	Links                []Link          `json:"links,omitempty"`
	RoleIDs              []string        `json:"role-ids,omitempty"`
	AuthorizedPrivileges json.RawMessage `json:"authorized-privileges,omitempty"`
	Remarks              string          `json:"remarks,omitempty"`
}

// SystemComponent is a component within the system boundary.
type SystemComponent struct {
	UUID             string            `json:"uuid"`
	Type             string            `json:"type"`
	Title            string            `json:"title"`
	Description      string            `json:"description"`
	Purpose          string            `json:"purpose,omitempty"`
	Props            []Property        `json:"props,omitempty"`
	Links            []Link            `json:"links,omitempty"`
	Status           Status            `json:"status"`
	ResponsibleRoles []ResponsibleRole `json:"responsible-roles,omitempty"`
	Protocols        json.RawMessage   `json:"protocols,omitempty"`
	Remarks          string            `json:"remarks,omitempty"`
}

// SystemImplementation lists the system's users and components.
type SystemImplementation struct {
	Props                   []Property        `json:"props,omitempty"`
	Links                   []Link            `json:"links,omitempty"`
	LeveragedAuthorizations json.RawMessage   `json:"leveraged-authorizations,omitempty"`
	Users                   []SystemUser      `json:"users,omitempty"`
	Components              []SystemComponent `json:"components"`
	InventoryItems          json.RawMessage   `json:"inventory-items,omitempty"`
	Remarks                 string            `json:"remarks,omitempty"`
}

// SSPControlImpl is the SSP's control-implementation section.
type SSPControlImpl struct {
	Description             string                      `json:"description"`
	SetParameters           []SetParameter              `json:"set-parameters,omitempty"`
	ImplementedRequirements []SSPImplementedRequirement `json:"implemented-requirements"`
}

// SSPImplementedRequirement describes how the system implements one control.
type SSPImplementedRequirement struct {
	UUID             string            `json:"uuid"`
	ControlID        string            `json:"control-id"`
	Props            []Property        `json:"props,omitempty"`
	Links            []Link            `json:"links,omitempty"`
	SetParameters    []SetParameter    `json:"set-parameters,omitempty"`
	ResponsibleRoles []ResponsibleRole `json:"responsible-roles,omitempty"`
	Statements       []SSPStatement    `json:"statements,omitempty"`
	ByComponents     []ByComponent     `json:"by-components,omitempty"`
	Remarks          string            `json:"remarks,omitempty"`
}

// SSPStatement describes how the system implements one control statement.
type SSPStatement struct {
	StatementID      string            `json:"statement-id"`
	UUID             string            `json:"uuid"`
	Props            []Property        `json:"props,omitempty"`
	Links            []Link            `json:"links,omitempty"`
	ResponsibleRoles []ResponsibleRole `json:"responsible-roles,omitempty"`
	ByComponents     []ByComponent     `json:"by-components,omitempty"`
	Remarks          string            `json:"remarks,omitempty"`
}

// ByComponent describes one component's contribution to a requirement.
type ByComponent struct {
	ComponentUUID        string                `json:"component-uuid"`
	UUID                 string                `json:"uuid"`
	Description          string                `json:"description"`
	Props                []Property            `json:"props,omitempty"`
	Links                []Link                `json:"links,omitempty"`
	SetParameters        []SetParameter        `json:"set-parameters,omitempty"`
	ImplementationStatus *ImplementationStatus `json:"implementation-status,omitempty"`
	Export               json.RawMessage       `json:"export,omitempty"`
	Inherited            json.RawMessage       `json:"inherited,omitempty"`
	Satisfied            json.RawMessage       `json:"satisfied,omitempty"`
	ResponsibleRoles     []ResponsibleRole     `json:"responsible-roles,omitempty"`
	Remarks              string                `json:"remarks,omitempty"`
}
