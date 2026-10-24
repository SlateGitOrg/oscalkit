// Package oscal provides typed Go models for the OSCAL catalog, profile,
// component-definition and system-security-plan (SSP) JSON formats, plus
// loading/encoding helpers.
//
// Struct fields are declared in OSCAL canonical order so that encoded JSON
// follows the order defined by the OSCAL metaschema. Rarely used, deeply
// nested structures that oscalkit never inspects are kept as json.RawMessage
// so that they survive a decode/encode round trip unchanged.
package oscal

import "encoding/json"

// Property is an OSCAL prop: a name/value pair with optional namespace.
type Property struct {
	Name    string `json:"name"`
	UUID    string `json:"uuid,omitempty"`
	NS      string `json:"ns,omitempty"`
	Value   string `json:"value"`
	Class   string `json:"class,omitempty"`
	Group   string `json:"group,omitempty"`
	Remarks string `json:"remarks,omitempty"`
}

// Link is an OSCAL link to a resource, either external or "#uuid" internal.
type Link struct {
	Href             string `json:"href"`
	Rel              string `json:"rel,omitempty"`
	MediaType        string `json:"media-type,omitempty"`
	ResourceFragment string `json:"resource-fragment,omitempty"`
	Text             string `json:"text,omitempty"`
}

// Part is a partition of a control, group or other part (statement,
// guidance, objective, ...).
type Part struct {
	ID    string     `json:"id,omitempty"`
	Name  string     `json:"name"`
	NS    string     `json:"ns,omitempty"`
	Class string     `json:"class,omitempty"`
	Title string     `json:"title,omitempty"`
	Props []Property `json:"props,omitempty"`
	Prose string     `json:"prose,omitempty"`
	Parts []Part     `json:"parts,omitempty"`
	Links []Link     `json:"links,omitempty"`
}

// ConstraintTest is a machine-checkable expression constraining a parameter.
type ConstraintTest struct {
	Expression string `json:"expression"`
	Remarks    string `json:"remarks,omitempty"`
}

// ParameterConstraint describes a constraint on a parameter's value.
type ParameterConstraint struct {
	Description string           `json:"description,omitempty"`
	Tests       []ConstraintTest `json:"tests,omitempty"`
}

// ParameterGuideline is prose guidance on how to set a parameter.
type ParameterGuideline struct {
	Prose string `json:"prose"`
}

// ParameterSelection restricts a parameter to a set of choices.
type ParameterSelection struct {
	HowMany string   `json:"how-many,omitempty"`
	Choice  []string `json:"choice,omitempty"`
}

// Parameter is an OSCAL param: a placeholder whose value may be assigned
// by a profile, a component definition or an SSP.
type Parameter struct {
	ID          string                `json:"id"`
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
	Remarks     string                `json:"remarks,omitempty"`
}

// DocumentID is an identifier for a document under some scheme (e.g. a DOI).
type DocumentID struct {
	Scheme     string `json:"scheme,omitempty"`
	Identifier string `json:"identifier"`
}

// Role is a function assumed by a party.
type Role struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	ShortName   string     `json:"short-name,omitempty"`
	Description string     `json:"description,omitempty"`
	Props       []Property `json:"props,omitempty"`
	Links       []Link     `json:"links,omitempty"`
	Remarks     string     `json:"remarks,omitempty"`
}

// Address is a postal address.
type Address struct {
	Type       string   `json:"type,omitempty"`
	AddrLines  []string `json:"addr-lines,omitempty"`
	City       string   `json:"city,omitempty"`
	State      string   `json:"state,omitempty"`
	PostalCode string   `json:"postal-code,omitempty"`
	Country    string   `json:"country,omitempty"`
}

// TelephoneNumber is a contact phone number.
type TelephoneNumber struct {
	Type   string `json:"type,omitempty"`
	Number string `json:"number"`
}

// Party is a person or organization.
type Party struct {
	UUID                  string            `json:"uuid"`
	Type                  string            `json:"type"`
	Name                  string            `json:"name,omitempty"`
	ShortName             string            `json:"short-name,omitempty"`
	ExternalIDs           json.RawMessage   `json:"external-ids,omitempty"`
	Props                 []Property        `json:"props,omitempty"`
	Links                 []Link            `json:"links,omitempty"`
	EmailAddresses        []string          `json:"email-addresses,omitempty"`
	TelephoneNumbers      []TelephoneNumber `json:"telephone-numbers,omitempty"`
	Addresses             []Address         `json:"addresses,omitempty"`
	LocationUUIDs         []string          `json:"location-uuids,omitempty"`
	MemberOfOrganizations []string          `json:"member-of-organizations,omitempty"`
	Remarks               string            `json:"remarks,omitempty"`
}

// ResponsibleParty binds a role to one or more parties.
type ResponsibleParty struct {
	RoleID     string     `json:"role-id"`
	PartyUUIDs []string   `json:"party-uuids"`
	Props      []Property `json:"props,omitempty"`
	Links      []Link     `json:"links,omitempty"`
	Remarks    string     `json:"remarks,omitempty"`
}

// ResponsibleRole names a role responsible for something, optionally with
// the parties that hold it.
type ResponsibleRole struct {
	RoleID     string     `json:"role-id"`
	Props      []Property `json:"props,omitempty"`
	Links      []Link     `json:"links,omitempty"`
	PartyUUIDs []string   `json:"party-uuids,omitempty"`
	Remarks    string     `json:"remarks,omitempty"`
}

// Metadata is the document-level metadata shared by all OSCAL models.
type Metadata struct {
	Title              string             `json:"title"`
	Published          string             `json:"published,omitempty"`
	LastModified       string             `json:"last-modified"`
	Version            string             `json:"version"`
	OSCALVersion       string             `json:"oscal-version"`
	Revisions          json.RawMessage    `json:"revisions,omitempty"`
	DocumentIDs        []DocumentID       `json:"document-ids,omitempty"`
	Props              []Property         `json:"props,omitempty"`
	Links              []Link             `json:"links,omitempty"`
	Roles              []Role             `json:"roles,omitempty"`
	Locations          json.RawMessage    `json:"locations,omitempty"`
	Parties            []Party            `json:"parties,omitempty"`
	ResponsibleParties []ResponsibleParty `json:"responsible-parties,omitempty"`
	Actions            json.RawMessage    `json:"actions,omitempty"`
	Remarks            string             `json:"remarks,omitempty"`
}

// Hash is a cryptographic digest of a linked resource.
type Hash struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

// RLink is a resource link: a URL to a (possibly alternate format) copy of
// a back-matter resource.
type RLink struct {
	Href      string `json:"href"`
	MediaType string `json:"media-type,omitempty"`
	Hashes    []Hash `json:"hashes,omitempty"`
}

// Base64 is a resource embedded inline as base64.
type Base64 struct {
	Filename  string `json:"filename,omitempty"`
	MediaType string `json:"media-type,omitempty"`
	Value     string `json:"value"`
}

// Citation is a bibliographic citation for a resource.
type Citation struct {
	Text  string     `json:"text"`
	Props []Property `json:"props,omitempty"`
	Links []Link     `json:"links,omitempty"`
}

// Resource is a back-matter resource, addressable as "#uuid".
type Resource struct {
	UUID        string       `json:"uuid"`
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Props       []Property   `json:"props,omitempty"`
	DocumentIDs []DocumentID `json:"document-ids,omitempty"`
	Citation    *Citation    `json:"citation,omitempty"`
	RLinks      []RLink      `json:"rlinks,omitempty"`
	Base64      *Base64      `json:"base64,omitempty"`
	Remarks     string       `json:"remarks,omitempty"`
}

// BackMatter holds a document's resources.
type BackMatter struct {
	Resources []Resource `json:"resources,omitempty"`
}

// Resource returns the back-matter resource with the given UUID.
func (b *BackMatter) Resource(uuid string) (*Resource, bool) {
	if b == nil {
		return nil, false
	}
	for i := range b.Resources {
		if b.Resources[i].UUID == uuid {
			return &b.Resources[i], true
		}
	}
	return nil, false
}

// ImplementationStatus records how far an implementation has progressed.
type ImplementationStatus struct {
	State   string `json:"state"`
	Remarks string `json:"remarks,omitempty"`
}

// SetParameter assigns values to a parameter in a component definition or SSP.
type SetParameter struct {
	ParamID string   `json:"param-id"`
	Values  []string `json:"values"`
	Remarks string   `json:"remarks,omitempty"`
}
