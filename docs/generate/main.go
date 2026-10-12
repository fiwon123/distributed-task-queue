// Command generate writes docs/openapi.json from docs/openapi.yaml.
//
// The YAML is the single source of truth; the JSON is a generated artifact that
// is committed so the API can serve /openapi.json without a YAML runtime
// dependency. Regenerate it with `make openapi-json` (or `go run ./docs/generate`);
// TestOpenAPIJSONMatchesYAML in backend/internal/httpapi fails if the two drift.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	src = "docs/openapi.yaml"
	dst = "docs/openapi.json"
)

func main() {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(src)
	if err != nil {
		fail(err)
	}
	// Validate so a malformed spec never gets baked into the committed JSON.
	if err := doc.Validate(loader.Context); err != nil {
		fail(err)
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fail(err)
	}
	out = append(out, '\n')
	if err := os.WriteFile(dst, out, 0o644); err != nil {
		fail(err)
	}
	fmt.Println("wrote", dst)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "generate openapi.json:", err)
	os.Exit(1)
}
