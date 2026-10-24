package oscal

import "testing"

// FuzzDecode checks that arbitrary input never panics the decoder and that
// anything accepted re-encodes.
func FuzzDecode(f *testing.F) {
	f.Add([]byte(`{"catalog":{"uuid":"u","metadata":{"title":"t"}}}`), "c.json")
	f.Add([]byte(yamlProfile), "p.yaml")
	f.Add([]byte("profile:\n  imports: [&a {href: x}, *a]\n"), "")
	f.Fuzz(func(t *testing.T, data []byte, name string) {
		doc, err := Decode(data, name)
		if err != nil {
			return
		}
		if _, err := Marshal(doc); err != nil {
			t.Fatalf("accepted document does not re-encode: %v", err)
		}
	})
}
