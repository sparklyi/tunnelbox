package docs_test

import (
	"os"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestOpenAPIContract(t *testing.T) {
	data, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI document: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("parse OpenAPI document: %v", err)
	}
	validateReferences(t, document, document, "$")

	if !sessionCookieSecurity(document["security"]) {
		t.Fatal("global security must require sessionCookie")
	}
	public := map[string]bool{
		"GET /healthz":            true,
		"GET /readyz":             true,
		"GET /api/v1/auth/status": true,
		"POST /api/v1/auth/setup": true,
		"POST /api/v1/auth/login": true,
	}
	methods := map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true}
	for path, rawPath := range objectAt(t, document, "paths") {
		pathItem, ok := rawPath.(map[string]any)
		if !ok {
			t.Fatalf("path %s is not an object", path)
		}
		for method, rawOperation := range pathItem {
			if !methods[method] {
				continue
			}
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				t.Fatalf("operation %s %s is not an object", method, path)
			}
			security, overrides := operation["security"]
			anonymous := overrides && emptyList(security)
			key := strings.ToUpper(method) + " " + path
			if anonymous != public[key] {
				t.Errorf("%s anonymous = %v, want %v", key, anonymous, public[key])
			}
		}
	}

	schemas := objectAt(t, document, "components", "schemas")
	for schemaName, propertyName := range map[string]string{
		"AuthRequest": "password", "CloudflareConfigureRequest": "token",
	} {
		schema := objectAt(t, schemas, schemaName)
		property := objectAt(t, schema, "properties", propertyName)
		if property["writeOnly"] != true {
			t.Errorf("%s.%s must be writeOnly", schemaName, propertyName)
		}
	}
}

func validateReferences(t *testing.T, root, value any, location string) {
	t.Helper()
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if key == "$ref" {
				ref, ok := child.(string)
				if !ok || !resolveReference(root, ref) {
					t.Errorf("unresolved reference at %s: %v", location, child)
				}
			}
			validateReferences(t, root, child, location+"."+key)
		}
	case []any:
		for _, child := range current {
			validateReferences(t, root, child, location)
		}
	}
}

func resolveReference(root any, ref string) bool {
	if !strings.HasPrefix(ref, "#/") {
		return false
	}
	current := root
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return false
		}
		current, ok = object[part]
		if !ok {
			return false
		}
	}
	return true
}

func objectAt(t *testing.T, root any, keys ...string) map[string]any {
	t.Helper()
	current := root
	for _, key := range keys {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("%s is not an object", strings.Join(keys, "."))
		}
		current, ok = object[key]
		if !ok {
			t.Fatalf("%s is missing", strings.Join(keys, "."))
		}
	}
	object, ok := current.(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object", strings.Join(keys, "."))
	}
	return object
}

func sessionCookieSecurity(value any) bool {
	entries, ok := value.([]any)
	if !ok || len(entries) != 1 {
		return false
	}
	requirement, ok := entries[0].(map[string]any)
	if !ok || len(requirement) != 1 {
		return false
	}
	return emptyList(requirement["sessionCookie"])
}

func emptyList(value any) bool {
	values, ok := value.([]any)
	return ok && len(values) == 0
}
