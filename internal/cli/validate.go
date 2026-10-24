package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/archit2501/oscalkit/check"
	"github.com/archit2501/oscalkit/oscal"
	"github.com/archit2501/oscalkit/schema"
)

// fileReport is the validation outcome for one file.
type fileReport struct {
	File     string             `json:"file"`
	Model    string             `json:"model,omitempty"`
	Error    string             `json:"error,omitempty"`
	Schema   []schema.Violation `json:"schema-violations"`
	Semantic []check.Finding    `json:"semantic-findings"`
	Valid    bool               `json:"valid"`
}

func runValidate(_ context.Context, e *env, args []string) int {
	fs := newFlags(e, "validate", "FILE...")
	format := fs.String("format", "text", "output format: text or json")
	schemaOnly := fs.Bool("schema-only", false, "skip semantic (referential integrity) checks")
	maxShown := fs.Int("max", 20, "maximum findings shown per file in text output (0 = all)")
	pos, code, ok := interleave(fs, args)
	if !ok {
		return code
	}
	if len(pos) == 0 {
		fs.Usage()
		return ExitUsage
	}
	if *format != "text" && *format != "json" {
		return e.errorf("validate: unknown format %q", *format)
	}
	var reports []fileReport
	failed := false
	for _, path := range pos {
		r := validateFile(path, *schemaOnly)
		if !r.Valid {
			failed = true
		}
		reports = append(reports, r)
	}
	if *format == "json" {
		enc := json.NewEncoder(e.stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(reports); err != nil {
			return e.errorf("%v", err)
		}
	} else {
		for _, r := range reports {
			printFileReport(e, r, *maxShown)
		}
	}
	if failed {
		return ExitFindings
	}
	return ExitOK
}

func validateFile(path string, schemaOnly bool) fileReport {
	r := fileReport{File: path, Schema: []schema.Violation{}, Semantic: []check.Finding{}}
	data, err := os.ReadFile(path)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	raw, err := oscal.Normalize(data, path)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	model, err := oscal.DetectModel(raw)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.Model = string(model)
	vs, err := schema.Validate(schema.Model(model), raw)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.Schema = append(r.Schema, vs...)
	if !schemaOnly {
		doc, err := oscal.Decode(raw, path)
		if err != nil {
			r.Error = err.Error()
			return r
		}
		r.Semantic = append(r.Semantic, check.Document(doc)...)
	}
	r.Valid = len(r.Schema) == 0 && check.Errors(r.Semantic) == 0
	return r
}

func printFileReport(e *env, r fileReport, maxShown int) {
	if r.Error != "" {
		fmt.Fprintf(e.stdout, "%s: ERROR %s\n", r.File, r.Error)
		return
	}
	verdict := "valid"
	if !r.Valid {
		verdict = "INVALID"
	}
	warnings := len(r.Semantic) - check.Errors(r.Semantic)
	fmt.Fprintf(e.stdout, "%s: %s %s (%d schema violations, %d semantic errors, %d warnings)\n",
		r.File, r.Model, verdict, len(r.Schema), check.Errors(r.Semantic), warnings)
	shown := 0
	limit := func() bool { return maxShown > 0 && shown >= maxShown }
	for _, v := range r.Schema {
		if limit() {
			break
		}
		fmt.Fprintf(e.stdout, "  schema   %s\n", v)
		shown++
	}
	for _, f := range r.Semantic {
		if limit() {
			break
		}
		fmt.Fprintf(e.stdout, "  %-8s %s %s: %s\n", f.Severity, f.Code, f.Path, f.Message)
		shown++
	}
	if total := len(r.Schema) + len(r.Semantic); shown < total {
		fmt.Fprintf(e.stdout, "  ... %d more (use --max 0 to show all)\n", total-shown)
	}
}
