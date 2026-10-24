// Package param implements OSCAL parameter substitution: finding
// "{{ insert: param, <id> }}" references in markup and rendering control
// prose with parameter values, selections or assignment placeholders.
package param

import (
	"regexp"
	"strings"
)

// insertPattern matches OSCAL markup insert directives. Only "param"
// inserts are substituted; other insert types are left as-is.
var insertPattern = regexp.MustCompile(`\{\{\s*insert:\s*([A-Za-z_][\w.-]*)\s*,\s*([^\s},]+)\s*\}\}`)

// Insert is one insert directive found in markup.
type Insert struct {
	Type  string // e.g. "param"
	IDRef string
	Start int // byte offset of the directive in the text
	End   int
}

// FindInserts returns every insert directive in text, in order.
func FindInserts(text string) []Insert {
	if !strings.Contains(text, "{{") {
		return nil
	}
	var out []Insert
	for _, m := range insertPattern.FindAllStringSubmatchIndex(text, -1) {
		out = append(out, Insert{
			Type:  text[m[2]:m[3]],
			IDRef: text[m[4]:m[5]],
			Start: m[0],
			End:   m[1],
		})
	}
	return out
}

// Refs returns the ids of all parameters referenced by insert
// directives in text, in order of appearance (duplicates retained).
func Refs(text string) []string {
	var ids []string
	for _, in := range FindInserts(text) {
		if in.Type == "param" {
			ids = append(ids, in.IDRef)
		}
	}
	return ids
}
