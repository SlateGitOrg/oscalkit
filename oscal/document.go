package oscal

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Model names an OSCAL document model by its root key.
type Model string

// Supported models.
const (
	ModelCatalog             Model = "catalog"
	ModelProfile             Model = "profile"
	ModelComponentDefinition Model = "component-definition"
	ModelSSP                 Model = "system-security-plan"
)

// MaxDocumentBytes bounds the decoded size of a document (after gzip
// decompression) to protect against decompression bombs.
const MaxDocumentBytes = 256 << 20

// ErrUnknownModel is returned when a document's root key is not a supported
// OSCAL model.
var ErrUnknownModel = errors.New("oscal: unknown or unsupported document model")

// Document is a decoded OSCAL document. Exactly one model pointer is set.
type Document struct {
	Catalog             *Catalog             `json:"catalog,omitempty"`
	Profile             *Profile             `json:"profile,omitempty"`
	ComponentDefinition *ComponentDefinition `json:"component-definition,omitempty"`
	SSP                 *SSP                 `json:"system-security-plan,omitempty"`

	// JSON holds the document as JSON bytes exactly as read (YAML input is
	// converted to JSON). Schema validation runs against these bytes.
	JSON []byte `json:"-"`
	// Source is where the document was read from (path or URI), if known.
	Source string `json:"-"`
}

// Model reports which model the document holds.
func (d *Document) Model() Model {
	switch {
	case d.Catalog != nil:
		return ModelCatalog
	case d.Profile != nil:
		return ModelProfile
	case d.ComponentDefinition != nil:
		return ModelComponentDefinition
	case d.SSP != nil:
		return ModelSSP
	}
	return ""
}

// ReadFile loads an OSCAL document from disk. Gzip-compressed files are
// detected by their magic bytes; YAML is detected by extension or content.
// When path does not exist but path+".gz" does, the compressed copy is read.
func ReadFile(path string) (*Document, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		if gz, gzErr := os.Open(path + ".gz"); gzErr == nil {
			f, err = gz, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("oscal: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("oscal: read %s: %w", path, err)
	}
	doc, err := Decode(data, path)
	if err != nil {
		return nil, err
	}
	doc.Source = path
	return doc, nil
}

// Decode parses document bytes. name is used for error messages and to
// recognise YAML by extension; it may be empty.
func Decode(data []byte, name string) (*Document, error) {
	raw, err := Normalize(data, name)
	if err != nil {
		return nil, err
	}
	model, err := DetectModel(raw)
	if err != nil {
		return nil, fmt.Errorf("oscal: %s: %w", displayName(name), err)
	}
	doc := &Document{JSON: raw}
	if err := json.Unmarshal(raw, doc); err != nil {
		return nil, fmt.Errorf("oscal: decode %s %s: %w", model, displayName(name), err)
	}
	if doc.Model() != model {
		return nil, fmt.Errorf("oscal: %s: %w", displayName(name), ErrUnknownModel)
	}
	return doc, nil
}

// Normalize turns raw input bytes (JSON, YAML, optionally gzipped) into JSON.
func Normalize(data []byte, name string) ([]byte, error) {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("oscal: gunzip %s: %w", displayName(name), err)
		}
		defer func() { _ = zr.Close() }()
		data, err = io.ReadAll(io.LimitReader(zr, MaxDocumentBytes+1))
		if err != nil {
			return nil, fmt.Errorf("oscal: gunzip %s: %w", displayName(name), err)
		}
		name = strings.TrimSuffix(name, ".gz")
	}
	if len(data) > MaxDocumentBytes {
		return nil, fmt.Errorf("oscal: %s exceeds %d bytes", displayName(name), MaxDocumentBytes)
	}
	if isYAML(data, name) {
		js, err := yamlToJSON(data)
		if err != nil {
			return nil, fmt.Errorf("oscal: yaml %s: %w", displayName(name), err)
		}
		return js, nil
	}
	return data, nil
}

func displayName(name string) string {
	if name == "" {
		return "<input>"
	}
	return name
}

func isYAML(data []byte, name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml":
		return true
	case ".json":
		return false
	}
	trimmed := bytes.TrimLeft(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), " \t\r\n")
	return len(trimmed) > 0 && trimmed[0] != '{'
}

// DetectModel inspects the root object of a JSON document and returns the
// OSCAL model it holds. A "$schema" key alongside the model is permitted.
func DetectModel(raw []byte) (Model, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return "", fmt.Errorf("not a JSON object: %w", err)
	}
	var found []Model
	for k := range root {
		switch Model(k) {
		case ModelCatalog, ModelProfile, ModelComponentDefinition, ModelSSP:
			found = append(found, Model(k))
		}
	}
	if len(found) != 1 {
		return "", ErrUnknownModel
	}
	return found[0], nil
}

// Marshal encodes v as indented JSON without HTML escaping, terminated by a
// newline.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := Encode(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Encode writes v as indented JSON without HTML escaping.
func Encode(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("oscal: encode: %w", err)
	}
	return nil
}

// WriteFile encodes v as JSON to path ("-" writes to stdout).
func WriteFile(path string, v any) error {
	if path == "-" || path == "" {
		bw := bufio.NewWriter(os.Stdout)
		if err := Encode(bw, v); err != nil {
			return err
		}
		return bw.Flush()
	}
	data, err := Marshal(v)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("oscal: write %s: %w", path, err)
	}
	return nil
}

// Wrap places a model value under its root key, ready for encoding.
func Wrap(v any) (any, error) {
	switch m := v.(type) {
	case *Catalog:
		return Document{Catalog: m}, nil
	case *Profile:
		return Document{Profile: m}, nil
	case *ComponentDefinition:
		return Document{ComponentDefinition: m}, nil
	case *SSP:
		return Document{SSP: m}, nil
	}
	return nil, fmt.Errorf("oscal: cannot wrap %T", v)
}

// yamlToJSON converts YAML to JSON while keeping every scalar's textual form
// (YAML timestamps stay strings, as OSCAL requires).
func yamlToJSON(data []byte) ([]byte, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	if node.Kind == 0 {
		return nil, errors.New("empty document")
	}
	budget := maxYAMLNodes
	v, err := yamlValue(&node, &budget)
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

// maxYAMLNodes bounds the number of nodes produced when expanding YAML,
// defeating alias-expansion ("billion laughs") attacks.
const maxYAMLNodes = 20_000_000

var errYAMLTooLarge = errors.New("yaml document expands to too many nodes (alias bomb?)")

func yamlValue(n *yaml.Node, budget *int) (any, error) {
	*budget--
	if *budget < 0 {
		return nil, errYAMLTooLarge
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return yamlValue(n.Content[0], budget)
	case yaml.AliasNode:
		return yamlValue(n.Alias, budget)
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i].Value
			v, err := yamlValue(n.Content[i+1], budget)
			if err != nil {
				return nil, err
			}
			m[k] = v
		}
		return m, nil
	case yaml.SequenceNode:
		s := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := yamlValue(c, budget)
			if err != nil {
				return nil, err
			}
			s = append(s, v)
		}
		return s, nil
	case yaml.ScalarNode:
		switch n.ShortTag() {
		case "!!null":
			return nil, nil
		case "!!bool":
			b, err := strconv.ParseBool(strings.ToLower(n.Value))
			if err != nil {
				return n.Value, nil
			}
			return b, nil
		case "!!int", "!!float":
			return json.Number(n.Value), nil
		}
		return n.Value, nil
	}
	return nil, fmt.Errorf("unsupported YAML node kind %d at line %d", n.Kind, n.Line)
}
