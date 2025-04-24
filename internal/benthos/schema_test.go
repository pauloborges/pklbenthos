package benthos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// standardSchemaFile holds the schema of a released build of Redpanda Connect,
// as `list --format json-full` wrote it. Refresh it with:
//
//	pklbenthos schema redpanda-connect VERSION > internal/benthos/testdata/...
//
// A file keeps this module free of an import of Redpanda Connect, and keeps
// the tests the same from one run to the next.
const standardSchemaFile = "redpanda-connect-4.103.1.json"

// loadStandardSchemaJSON returns the schema of a released build of Redpanda
// Connect in the V0 JSON encoding.
func loadStandardSchemaJSON(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", standardSchemaFile))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}

	return data
}

// loadStandardSchema parses the schema of a released build of Redpanda
// Connect. The tests compile that schema, because it holds every shape of
// property that the compiler must handle.
func loadStandardSchema(t *testing.T) *Schema {
	t.Helper()

	schema, err := ParseSchema(loadStandardSchemaJSON(t))
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}

	return schema
}

// TestLoadSchemaHasAllComponents guards the schema file. A file that holds
// only the few components that public/schema registers by itself still parses,
// and the generated Pkl library is then nearly empty.
func TestLoadSchemaHasAllComponents(t *testing.T) {
	schema := loadStandardSchema(t)

	// Lower bounds, well below the real counts, so that the test does not
	// fail when Redpanda Connect adds or removes a component.
	groups := []struct {
		name    string
		items   []*Component
		atLeast int
	}{
		{"buffers", schema.Buffers, 3},
		{"caches", schema.Caches, 10},
		{"inputs", schema.Inputs, 50},
		{"outputs", schema.Outputs, 50},
		{"metrics", schema.Metrics, 5},
		{"processors", schema.Processors, 50},
		{"rate-limits", schema.RateLimits, 1},
		{"scanners", schema.Scanners, 5},
		{"tracers", schema.Tracers, 3},
	}

	for _, group := range groups {
		if len(group.items) < group.atLeast {
			t.Errorf("%s: got %d components, want at least %d",
				group.name, len(group.items), group.atLeast)
		}

		for _, component := range group.items {
			if component.Config == nil {
				t.Errorf("%s: component %q has no config", group.name, component.Name)
			}
		}
	}

	if len(schema.Config) == 0 {
		t.Error("root config has no properties")
	}
}

// TestSchemaVocabulary fails when Redpanda Connect introduces a property type
// or kind that the Type and Kind constants do not cover. A new value would
// otherwise fall through to the default branch of pklTypeName and produce a
// wrong Pkl type.
func TestSchemaVocabulary(t *testing.T) {
	schema := loadStandardSchema(t)

	knownTypes := map[Type]bool{
		TypeBool: true, TypeInt: true, TypeFloat: true, TypeString: true,
		TypeObject: true, TypeUnknown: true, TypeBuffer: true, TypeCache: true,
		TypeInput: true, TypeOutput: true, TypeMetrics: true, TypeProcessor: true,
		TypeRateLimit: true, TypeScanner: true, TypeTracer: true,
	}
	knownKinds := map[Kind]bool{
		KindUnset: true, KindScalar: true, KindArray: true,
		Kind2DArray: true, KindMap: true,
	}

	seenType := make(map[Type]bool)
	seenKind := make(map[Kind]bool)

	var walk func(*Property)
	walk = func(prop *Property) {
		if prop == nil {
			return
		}

		if !knownTypes[prop.Type] && !seenType[prop.Type] {
			seenType[prop.Type] = true
			t.Errorf("property %q has unknown type %q", prop.Name, prop.Type)
		}
		if !knownKinds[prop.Kind] && !seenKind[prop.Kind] {
			seenKind[prop.Kind] = true
			t.Errorf("property %q has unknown kind %q", prop.Name, prop.Kind)
		}

		for _, child := range prop.Children {
			walk(child)
		}
	}

	for _, prop := range schema.Config {
		walk(prop)
	}

	groups := [][]*Component{
		schema.Buffers, schema.Caches, schema.Inputs, schema.Outputs,
		schema.Metrics, schema.Processors, schema.RateLimits,
		schema.Scanners, schema.Tracers,
	}
	for _, group := range groups {
		for _, component := range group {
			walk(component.Config)
		}
	}
}

// TestPropertyIsRequired covers the four cases that decide if a configuration
// must set a property. The schema states a default of null for a few
// properties, and Default holds a pointer so that such a property stays apart
// from one that has no default at all.
func TestPropertyIsRequired(t *testing.T) {
	nullDefault := any(nil)
	valueDefault := any("const")

	tests := []struct {
		name string
		prop *Property
		want bool
	}{
		{
			name: "no default",
			prop: &Property{},
			want: true,
		},
		{
			name: "no default but optional",
			prop: &Property{IsOptional: true},
			want: false,
		},
		{
			name: "default of null",
			prop: &Property{Default: &nullDefault},
			want: false,
		},
		{
			name: "default with a value",
			prop: &Property{Default: &valueDefault},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.prop.IsRequired(); got != test.want {
				t.Errorf("IsRequired() = %v, want %v", got, test.want)
			}
		})
	}
}

// bloblangKeys holds the two top-level keys that this compiler reads nothing
// from. Bloblang functions and methods describe the mapping language, not the
// configuration of a component, so the Schema type declares no field for them
// and TestSchemaKeyCoverage lets them pass.
var bloblangKeys = []string{"bloblang-functions", "bloblang-methods"}

// TestSchemaKeyCoverage fails when the Redpanda Connect schema holds a key that
// the types in schema.go do not declare.
//
// Those types mirror the internal types of Redpanda Connect, which this module
// can not import. A key that no field claims decodes into nothing, and the
// generated Pkl library silently loses whatever the key carries. This test
// turns such an addition into a test failure.
func TestSchemaKeyCoverage(t *testing.T) {
	data := loadStandardSchemaJSON(t)

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("decode schema: %v", err)
	}

	// report names each key once, however many properties hold it. The schema
	// has thousands of properties, and one missing field would otherwise fill
	// the output.
	reported := make(map[string]bool)
	report := func(where, key string) {
		if reported[where+"."+key] {
			return
		}
		reported[where+"."+key] = true
		t.Errorf("%s has key %q that no field of the Go type declares", where, key)
	}

	schemaKeys := jsonKeys(reflect.TypeFor[Schema]())
	for key := range root {
		if slices.Contains(bloblangKeys, key) || schemaKeys[key] {
			continue
		}
		report("the schema", key)
	}

	componentKeys := jsonKeys(reflect.TypeFor[Component]())
	propertyKeys := jsonKeys(reflect.TypeFor[Property]())

	var walkProperty func(where string, raw json.RawMessage)
	walkProperty = func(where string, raw json.RawMessage) {
		var prop map[string]json.RawMessage
		if err := json.Unmarshal(raw, &prop); err != nil {
			t.Errorf("%s: decode property: %v", where, err)
			return
		}

		for key := range prop {
			if !propertyKeys[key] {
				report(where, key)
			}
		}

		var children []json.RawMessage
		if raw, ok := prop["children"]; ok {
			if err := json.Unmarshal(raw, &children); err != nil {
				t.Errorf("%s: decode children: %v", where, err)
				return
			}
		}
		for _, child := range children {
			walkProperty(where, child)
		}
	}

	for group, raw := range root {
		if group == "config" {
			var props []json.RawMessage
			if err := json.Unmarshal(raw, &props); err != nil {
				t.Fatalf("decode root config: %v", err)
			}
			for _, prop := range props {
				walkProperty("a root config property", prop)
			}
			continue
		}

		if slices.Contains(bloblangKeys, group) || !strings.HasPrefix(string(raw), "[") {
			continue
		}

		var components []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &components); err != nil {
			t.Errorf("decode %s: %v", group, err)
			continue
		}

		for _, component := range components {
			for key := range component {
				if !componentKeys[key] {
					report("a "+group+" component", key)
				}
			}

			if config, ok := component["config"]; ok {
				walkProperty("a "+group+" config property", config)
			}
		}
	}
}

// jsonKeys returns the JSON keys that the fields of a struct type claim.
func jsonKeys(typ reflect.Type) map[string]bool {
	keys := make(map[string]bool, typ.NumField())

	for field := range typ.Fields() {
		field := field

		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" {
			name = field.Name
		}
		if name == "-" {
			continue
		}

		keys[name] = true
	}

	return keys
}
