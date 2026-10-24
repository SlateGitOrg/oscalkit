// Package schema validates OSCAL JSON documents against the official NIST
// OSCAL JSON Schemas (draft-07), which are embedded in the binary so that
// validation works offline.
package schema

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// OSCALVersion is the OSCAL release whose JSON schemas are embedded.
const OSCALVersion = "1.2.3"

//go:embed oscal-v1.2.3/*.json
var schemaFS embed.FS

// Model identifies one of the OSCAL document models this package can validate.
type Model string

// The OSCAL models supported by the embedded schemas.
const (
	Catalog             Model = "catalog"
	Profile             Model = "profile"
	ComponentDefinition Model = "component-definition"
	SSP                 Model = "system-security-plan"
)

// Models lists every supported model in a stable order.
var Models = []Model{Catalog, Profile, ComponentDefinition, SSP}

var schemaFiles = map[Model]string{
	Catalog:             "oscal_catalog_schema.json",
	Profile:             "oscal_profile_schema.json",
	ComponentDefinition: "oscal_component_schema.json",
	SSP:                 "oscal_ssp_schema.json",
}

// Violation is one schema failure, located by a JSON Pointer into the
// instance document.
type Violation struct {
	// Pointer is an RFC 6901 JSON Pointer to the offending value ("" is the root).
	Pointer string `json:"pointer"`
	// Message is a human-readable description of the failure.
	Message string `json:"message"`
}

func (v Violation) String() string {
	p := v.Pointer
	if p == "" {
		p = "/"
	}
	return p + ": " + v.Message
}

// Validator validates documents against the embedded schemas. The zero value
// is not usable; construct one with New. A Validator is safe for concurrent use.
type Validator struct {
	mu       sync.Mutex
	compiled map[Model]*jsonschema.Schema
	printer  *message.Printer
}

// New returns a Validator. Schemas are compiled lazily on first use.
func New() *Validator {
	return &Validator{
		compiled: map[Model]*jsonschema.Schema{},
		printer:  message.NewPrinter(language.English),
	}
}

var defaultValidator = New()

// Validate validates raw JSON against the schema of the given model using a
// shared Validator.
func Validate(model Model, raw []byte) ([]Violation, error) {
	return defaultValidator.Validate(model, raw)
}

func (v *Validator) schemaFor(model Model) (*jsonschema.Schema, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if s, ok := v.compiled[model]; ok {
		return s, nil
	}
	file, ok := schemaFiles[model]
	if !ok {
		return nil, fmt.Errorf("schema: unsupported model %q", model)
	}
	data, err := schemaFS.ReadFile("oscal-v1.2.3/" + file)
	if err != nil {
		return nil, fmt.Errorf("schema: read embedded %s: %w", file, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("schema: parse embedded %s: %w", file, err)
	}
	if model == Profile {
		patchCombineMethod(doc)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft7)
	url := "embedded:///" + file
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("schema: add %s: %w", file, err)
	}
	s, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("schema: compile %s: %w", file, err)
	}
	v.compiled[model] = s
	return s, nil
}

// Validate checks raw JSON against the schema for model. A nil error with an
// empty slice means the document is schema-valid. An error is returned only
// when validation could not run (bad JSON, unknown model).
func (v *Validator) Validate(model Model, raw []byte) ([]Violation, error) {
	s, err := v.schemaFor(model)
	if err != nil {
		return nil, err
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("schema: parse instance: %w", err)
	}
	err = s.Validate(inst)
	if err == nil {
		return nil, nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return nil, fmt.Errorf("schema: validate: %w", err)
	}
	return v.flatten(ve), nil
}

// flatten reduces the validation error tree to its leaf failures. For
// anyOf/oneOf failures only the most plausible branch is reported: the one
// whose failures reach deepest into the instance (the branch that "almost"
// matched), breaking ties by fewest failures.
func (v *Validator) flatten(root *jsonschema.ValidationError) []Violation {
	var out []Violation
	seen := map[Violation]bool{}
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		switch e.ErrorKind.(type) {
		case *kind.AnyOf, *kind.OneOf:
			if len(e.Causes) > 0 {
				walk(bestBranch(e.Causes))
				return
			}
		}
		if len(e.Causes) == 0 {
			vi := Violation{
				Pointer: pointer(e.InstanceLocation),
				Message: e.ErrorKind.LocalizedString(v.printer),
			}
			if !seen[vi] {
				seen[vi] = true
				out = append(out, vi)
			}
			return
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(root)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pointer < out[j].Pointer })
	return out
}

func bestBranch(branches []*jsonschema.ValidationError) *jsonschema.ValidationError {
	best := branches[0]
	bestDepth, bestCount := branchScore(best)
	for _, b := range branches[1:] {
		d, c := branchScore(b)
		if d > bestDepth || (d == bestDepth && c < bestCount) {
			best, bestDepth, bestCount = b, d, c
		}
	}
	return best
}

// branchScore returns the maximum instance depth reached by any leaf failure
// and the number of leaf failures.
func branchScore(e *jsonschema.ValidationError) (depth, count int) {
	if len(e.Causes) == 0 {
		return len(e.InstanceLocation), 1
	}
	for _, c := range e.Causes {
		d, n := branchScore(c)
		if d > depth {
			depth = d
		}
		count += n
	}
	return depth, count
}

func pointer(loc []string) string {
	if len(loc) == 0 {
		return ""
	}
	var b strings.Builder
	for _, tok := range loc {
		b.WriteByte('/')
		tok = strings.ReplaceAll(tok, "~", "~0")
		tok = strings.ReplaceAll(tok, "/", "~1")
		b.WriteString(tok)
	}
	return b.String()
}

// patchCombineMethod repairs a defect in the published OSCAL 1.2.3 profile
// JSON schema: the merge/combine assembly is emitted with
// "additionalProperties": false but without its "method" flag, so every
// profile using combine.method (which the metaschema defines) would be
// rejected. The embedded file stays byte-identical to the upstream release
// (its checksum is verified); the fix is applied in memory before compiling.
func patchCombineMethod(doc any) {
	root, ok := doc.(map[string]any)
	if !ok {
		return
	}
	defs, _ := root["definitions"].(map[string]any)
	merge, _ := defs["oscal-profile-oscal-profile:merge"].(map[string]any)
	branches, _ := merge["anyOf"].([]any)
	for _, b := range branches {
		branch, _ := b.(map[string]any)
		props, _ := branch["properties"].(map[string]any)
		combine, _ := props["combine"].(map[string]any)
		if combine == nil {
			continue
		}
		if _, has := combine["properties"]; has {
			continue
		}
		combine["properties"] = map[string]any{
			"method": map[string]any{
				"type": "string",
				"enum": []any{"use-first", "merge", "keep"},
			},
		}
	}
}
