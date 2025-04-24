package benthos

import (
	"slices"
	"strings"
	"testing"

	"github.com/pauloborges/pklbenthos/internal/pkl"
)

const testModulePrefix = "RedpandaConnect"

func compileTestSchema(t *testing.T) pkl.Modules {
	t.Helper()

	schema := loadStandardSchema(t)

	modules, err := Compile(schema, &CompileOptions{ModulePrefix: testModulePrefix})
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}

	return modules
}

func TestCompileConfigurationModule(t *testing.T) {
	modules := compileTestSchema(t)

	module, ok := modules[testModulePrefix+".Configuration"]
	if !ok {
		t.Fatal("Configuration module not found")
	}

	if module.Path != "Configuration.pkl" {
		t.Errorf("path = %q, want %q", module.Path, "Configuration.pkl")
	}

	// Pkl keeps the name `output` for the property that controls rendering,
	// and the root configuration has a field with that name. The fields go in
	// a class, and the module renders an instance of it.
	if module.Output == nil {
		t.Fatal("the module has no output")
	}
	if module.Output.Value != "config" {
		t.Errorf("output value = %q, want %q", module.Output.Value, "config")
	}

	// A plugin keeps its fields at the top level. The converter wraps them in
	// the name of the plugin, which is the shape that Redpanda Connect reads.
	if len(module.Output.Converters) != 1 {
		t.Fatalf("got %d converters, want 1", len(module.Output.Converters))
	}
	if got := module.Output.Converters[0].Key; got != "Plugin.getClass()" {
		t.Errorf("converter key = %q, want %q", got, "Plugin.getClass()")
	}
	if !hasImport(module.Imports, "Plugin.pkl") {
		t.Error("Configuration module does not import Plugin.pkl")
	}

	if len(module.Properties) != 1 {
		t.Fatalf("got %d module properties, want 1", len(module.Properties))
	}
	if got := module.Properties[0].Name; got != "config" {
		t.Errorf("module property = %q, want %q", got, "config")
	}

	root := findClass(module.Classes, "Root")
	if root == nil {
		t.Fatal("Root class not found")
	}

	for _, name := range []pkl.Identifier{"input", "output", "pipeline", "tracer", "metrics"} {
		if findProperty(root.Properties, name) == nil {
			t.Errorf("Root class has no %q property", name)
		}
	}

	// A field that holds a component uses the abstract module of the family
	// as its type, so the module has to import it.
	tracer := findProperty(root.Properties, "tracer")
	if tracer != nil && tracer.Type.Name != "Tracer" {
		t.Errorf("tracer type = %q, want %q", tracer.Type.Name, "Tracer")
	}

	if !hasImport(module.Imports, "Tracer.pkl") {
		t.Error("Configuration module does not import Tracer.pkl")
	}
}

// TestCompilePluginBaseModule covers the module that holds the name of a
// plugin. Every component family extends it, so the converter of the
// Configuration module reaches every plugin through one class.
func TestCompilePluginBaseModule(t *testing.T) {
	modules := compileTestSchema(t)

	module, ok := modules[testModulePrefix+".Plugin"]
	if !ok {
		t.Fatal("Plugin module not found")
	}

	if got := module.Modifiers.String(); got != "abstract" {
		t.Errorf("modifiers = %q, want %q", got, "abstract")
	}

	prop := findProperty(module.Properties, "plugin")
	if prop == nil {
		t.Fatal("Plugin module has no plugin property")
	}

	// Hidden keeps the name out of the fields of the plugin, and fixed stops
	// a configuration from changing it.
	want := "fixed hidden plugin: String"
	if got := prop.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestCompileProductName covers the name that the generated documentation
// gives to the program which reads the configuration. A custom build takes the
// name of its own product, and a build that gives no name takes the default.
//
// An empty schema is enough, because the name goes in the modules that the
// compiler always writes.
func TestCompileProductName(t *testing.T) {
	tests := []struct {
		name string
		opts *CompileOptions
		want string
	}{
		{"default", &CompileOptions{}, defaultProductName},
		{"custom", &CompileOptions{ProductName: "Example Connect"}, "Example Connect"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modules, err := Compile(&Schema{}, test.opts)
			if err != nil {
				t.Fatalf("compile schema: %v", err)
			}

			for _, name := range []pkl.QualifiedIdentifier{"Configuration", "Plugin"} {
				module, ok := modules[name]
				if !ok {
					t.Fatalf("%s module not found", name)
				}

				if !strings.Contains(module.Documentation, test.want) {
					t.Errorf("documentation of %s has no %q:\n%s",
						name, test.want, module.Documentation)
				}
			}
		})
	}
}

// TestCompileRootFieldModules covers the fields of the root configuration
// that hold an object. Each one goes in a module of its own, so that
// Configuration.pkl stays small.
func TestCompileRootFieldModules(t *testing.T) {
	modules := compileTestSchema(t)

	configuration := modules[testModulePrefix+".Configuration"]
	if configuration == nil {
		t.Fatal("Configuration module not found")
	}

	root := findClass(configuration.Classes, "Root")
	if root == nil {
		t.Fatal("Root class not found")
	}

	tests := []struct {
		field  pkl.Identifier
		module string
		// typ is the type of the field in the Root class.
		typ string
		// classes are the nested objects of the field. They stay in the same
		// module as the field, rather than getting a module each.
		classes []pkl.Identifier
	}{
		{field: "http", module: "Http", typ: "Http?", classes: []pkl.Identifier{"Cors", "BasicAuth"}},
		{field: "logger", module: "Logger", typ: "Logger?", classes: []pkl.Identifier{"File"}},
		{field: "pipeline", module: "Pipeline", typ: "Pipeline?"},
		// A field that holds a list of objects uses the module as the type of
		// the element.
		{field: "tests", module: "Tests", typ: "Listing<Tests>?"},
	}

	for _, test := range tests {
		t.Run(string(test.field), func(t *testing.T) {
			module, ok := modules[pkl.QualifiedIdentifier(testModulePrefix+"."+test.module)]
			if !ok {
				t.Fatalf("module %s not found", test.module)
			}
			if module.Path != test.module+".pkl" {
				t.Errorf("path = %q, want %q", module.Path, test.module+".pkl")
			}

			for _, name := range test.classes {
				if findClass(module.Classes, name) == nil {
					t.Errorf("module %s has no %q class", test.module, name)
				}
			}

			field := findProperty(root.Properties, test.field)
			if field == nil {
				t.Fatalf("Root class has no %q field", test.field)
			}
			if got := field.Type.String(); got != test.typ {
				t.Errorf("type = %q, want %q", got, test.typ)
			}

			if !hasImport(configuration.Imports, test.module+".pkl") {
				t.Errorf("Configuration module does not import %s.pkl", test.module)
			}

			// The nested objects of the field left Configuration.pkl.
			for _, name := range test.classes {
				if findClass(configuration.Classes, name) != nil {
					t.Errorf("Configuration module still holds the %q class", name)
				}
			}
		})
	}

}

func TestCompileComponentModules(t *testing.T) {
	modules := compileTestSchema(t)

	components := []string{
		"Buffer", "Cache", "Input", "Output", "Metrics",
		"Processor", "RateLimit", "Scanner", "Tracer",
	}

	for _, component := range components {
		name := pkl.QualifiedIdentifier(testModulePrefix + "." + component)

		module, ok := modules[name]
		if !ok {
			t.Errorf("module %s not found", name)
			continue
		}

		if got := module.Modifiers.String(); got != "abstract" {
			t.Errorf("module %s modifiers = %q, want %q", name, got, "abstract")
		}
		if module.Extends != "Plugin.pkl" {
			t.Errorf("module %s extends %q, want %q", name, module.Extends, "Plugin.pkl")
		}
	}
}

func TestCompilePluginModules(t *testing.T) {
	modules := compileTestSchema(t)

	tests := []struct {
		module  pkl.QualifiedIdentifier
		path    string
		extends string
		plugin  string
		fields  []pkl.Identifier
	}{
		{
			module:  testModulePrefix + ".tracers.Jaeger",
			path:    "tracers/Jaeger.pkl",
			extends: "../Tracer.pkl",
			plugin:  `"jaeger"`,
			fields:  []pkl.Identifier{"agent_address", "collector_url", "sampler_type"},
		},
		{
			module:  testModulePrefix + ".tracers.OpenTelemetryCollector",
			path:    "tracers/OpenTelemetryCollector.pkl",
			extends: "../Tracer.pkl",
			plugin:  `"open_telemetry_collector"`,
			fields:  []pkl.Identifier{"service", "http", "grpc", "sampling"},
		},
		{
			module:  testModulePrefix + ".metrics.Prometheus",
			path:    "metrics/Prometheus.pkl",
			extends: "../Metrics.pkl",
			plugin:  `"prometheus"`,
			fields:  []pkl.Identifier{"use_histogram_timing"},
		},
	}

	for _, test := range tests {
		t.Run(string(test.module), func(t *testing.T) {
			module, ok := modules[test.module]
			if !ok {
				t.Fatal("module not found")
			}

			if module.Path != test.path {
				t.Errorf("path = %q, want %q", module.Path, test.path)
			}
			if module.Extends != test.extends {
				t.Errorf("extends = %q, want %q", module.Extends, test.extends)
			}

			// The first property fixes the name that selects the plugin. It
			// takes no type, because it overrides the one that the Plugin
			// module declares.
			if len(module.Properties) == 0 {
				t.Fatal("module has no properties")
			}

			plugin := module.Properties[0]
			if plugin.Name != "plugin" {
				t.Fatalf("first property = %q, want %q", plugin.Name, "plugin")
			}
			if plugin.Default != test.plugin {
				t.Errorf("plugin = %s, want %s", plugin.Default, test.plugin)
			}
			if plugin.Type != nil {
				t.Errorf("plugin has type %q, want none", plugin.Type)
			}

			// The fields of the plugin sit next to it, at the top level.
			for _, field := range test.fields {
				if findProperty(module.Properties, field) == nil {
					t.Errorf("module has no %q property", field)
				}
			}
		})
	}
}

// TestCompilePluginModulesForEveryFamily checks that each family of components
// gets a module for each of its plugins.
func TestCompilePluginModulesForEveryFamily(t *testing.T) {
	modules := compileTestSchema(t)

	counts := map[string]int{}
	for name := range modules {
		for _, dir := range componentDirs {
			if strings.HasPrefix(string(name), testModulePrefix+"."+dir+".") {
				counts[dir]++
			}
		}
	}

	for _, dir := range componentDirs {
		if counts[dir] == 0 {
			t.Errorf("no plugin modules under %s", dir)
		}
	}
}

var componentDirs = []string{
	"buffers", "caches", "inputs", "outputs", "metrics",
	"processors", "rate_limits", "scanners", "tracers",
}

// TestCompilePluginWithoutFields covers a plugin that takes no configuration,
// such as the `none` tracer. Only the name of the plugin stays, and the
// converter turns it into an empty object.
func TestCompilePluginWithoutFields(t *testing.T) {
	modules := compileTestSchema(t)

	module, ok := modules[testModulePrefix+".tracers.None"]
	if !ok {
		t.Fatal("None tracer module not found")
	}

	if len(module.Properties) != 1 {
		t.Fatalf("got %d properties, want 1", len(module.Properties))
	}

	want := `fixed plugin = "none"`
	if got := module.Properties[0].String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestCompileDefaultsGoInDocumentation covers the rule that a generated module
// holds no default value. A configuration that repeats a default keeps it even
// after Redpanda Connect changes that default, so the default belongs in the
// documentation and the property stays unset.
func TestCompileDefaultsGoInDocumentation(t *testing.T) {
	modules := compileTestSchema(t)

	module, ok := modules[testModulePrefix+".tracers.Jaeger"]
	if !ok {
		t.Fatal("Jaeger tracer module not found")
	}

	tests := []struct {
		name pkl.Identifier
		// note is the line that the documentation gains, if any.
		note string
	}{
		// A default that carries a value goes in the documentation.
		{name: "sampler_param", note: "Default: `1.0`"},
		{name: "sampler_type", note: "Default: `\"const\"`"},
		// An empty default says no more than leaving the field out.
		{name: "agent_address"},
		{name: "collector_url"},
		{name: "tags"},
		// A field that the schema gives no default at all.
		{name: "flush_interval"},
	}

	for _, test := range tests {
		t.Run(string(test.name), func(t *testing.T) {
			prop := findProperty(module.Properties, test.name)
			if prop == nil {
				t.Fatal("property not found")
			}

			if !prop.Type.Nullable {
				t.Errorf("type = %q, want a nullable type", prop.Type)
			}
			if prop.Default != "" {
				t.Errorf("default = %q, want none", prop.Default)
			}

			if test.note == "" {
				if strings.Contains(prop.Documentation, "Default:") {
					t.Errorf("documentation notes a default:\n%s", prop.Documentation)
				}
				return
			}

			if !strings.Contains(prop.Documentation, test.note) {
				t.Errorf("documentation has no %q:\n%s", test.note, prop.Documentation)
			}
		})
	}
}

// TestCompileBlocksAreOptional covers the fields that hold an object. The
// schema marks none of them optional, but a configuration may leave out a
// whole block, so each one is nullable.
func TestCompileBlocksAreOptional(t *testing.T) {
	modules := compileTestSchema(t)

	root := findClass(modules[testModulePrefix+".Configuration"].Classes, "Root")
	if root == nil {
		t.Fatal("Root class not found")
	}

	for _, name := range []pkl.Identifier{"http", "pipeline", "logger", "error_handling", "redpanda"} {
		prop := findProperty(root.Properties, name)
		if prop == nil {
			t.Errorf("Root class has no %q field", name)
			continue
		}

		if !prop.Type.Nullable {
			t.Errorf("%s: type = %q, want a nullable type", name, prop.Type)
		}
		if prop.Default != "" {
			t.Errorf("%s: default = %q, want none", name, prop.Default)
		}
	}

	// A field that a block itself requires stays required inside the block.
	redpanda, ok := modules[testModulePrefix+".Redpanda"]
	if !ok {
		t.Fatal("Redpanda module not found")
	}

	brokers := findProperty(redpanda.Properties, "seed_brokers")
	if brokers == nil {
		t.Fatal("Redpanda module has no seed_brokers property")
	}
	if brokers.Type.Nullable {
		t.Errorf("seed_brokers type = %q, want a required type", brokers.Type)
	}
}

func TestIsEmptyDefault(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{"empty string", "", true},
		{"empty list", []any{}, true},
		{"empty map", map[string]any{}, true},
		{"string", "x", false},
		{"no default", nil, false},
		{"zero", float64(0), false},
		{"false", false, false},
		{"list", []any{"a"}, false},
		{"map", map[string]any{"a": 1}, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isEmptyDefault(test.value); got != test.want {
				t.Errorf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestPascal(t *testing.T) {
	tests := []struct {
		parts []string
		want  string
	}{
		{[]string{"kafka"}, "Kafka"},
		{[]string{"kafka_franz"}, "KafkaFranz"},
		{[]string{"redpanda_migrator"}, "RedpandaMigrator"},
		{[]string{"redpanda", "schema_registry", "tls"}, "RedpandaSchemaRegistryTls"},
		{[]string{""}, ""},
		{nil, ""},
	}

	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			if got := pascal(test.parts...); got != test.want {
				t.Errorf("pascal(%q) = %q, want %q", test.parts, got, test.want)
			}
		})
	}
}

func TestPklString(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"plain", `"plain"`},
		{`with "quotes"`, `"with \"quotes\""`},
		{`back\slash`, `"back\\slash"`},
		// A backslash before a parenthesis starts an interpolation in Pkl.
		// Escaping the backslash stops that.
		{`\(1 + 1)`, `"\\(1 + 1)"`},
		{"two\nlines", `"two\nlines"`},
	}

	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			if got := pklString(test.in); got != test.want {
				t.Errorf("got %s, want %s", got, test.want)
			}
		})
	}
}

func TestPklNumber(t *testing.T) {
	float := &pkl.Type{Name: "Float"}
	integer := &pkl.Type{Name: "Int"}

	tests := []struct {
		name  string
		value float64
		typ   *pkl.Type
		want  string
	}{
		// Pkl rejects an Int literal where it expects a Float.
		{"whole float keeps a decimal point", 1, float, "1.0"},
		{"fractional float", 0.5, float, "0.5"},
		{"integer", 42, integer, "42"},
		{"negative integer", -1, integer, "-1"},
		{"large integer", 524288000, integer, "524288000"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := pklNumber(test.value, test.typ); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestPklLiteral(t *testing.T) {
	stringMap := mappingType(stringType())
	list := listingType(stringType())

	tests := []struct {
		name  string
		value any
		typ   *pkl.Type
		want  string
		ok    bool
	}{
		{"bool", true, nil, "true", true},
		{"string", "x", nil, `"x"`, true},
		{"empty listing", []any{}, list, "new {}", true},
		{"listing", []any{"a", "b"}, list, `new { "a" "b" }`, true},
		{"empty mapping", map[string]any{}, stringMap, "new {}", true},
		{
			name:  "mapping",
			value: map[string]any{"b": "2", "a": "1"},
			typ:   stringMap,
			// The keys come out sorted, so the output stays reproducible.
			want: `new { ["a"] = "1"; ["b"] = "2" }`,
			ok:   true,
		},
		{
			name:  "component selection is not expressible",
			value: map[string]any{"stdin": map[string]any{}},
			typ:   &pkl.Type{Name: "Input"},
			ok:    false,
		},
		{"null", nil, nil, "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := pklLiteral(test.value, test.typ)
			if ok != test.ok {
				t.Fatalf("ok = %v, want %v (got %q)", ok, test.ok, got)
			}
			if ok && got != test.want {
				t.Errorf("got %s, want %s", got, test.want)
			}
		})
	}
}

func TestWrapKind(t *testing.T) {
	tests := []struct {
		kind Kind
		want string
	}{
		{KindScalar, "String"},
		{KindUnset, "String"},
		{KindArray, "Listing<String>"},
		{Kind2DArray, "Listing<Listing<String>>"},
		{KindMap, "Mapping<String, String>"},
	}

	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			if got := wrapKind(stringType(), test.kind).String(); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func findClass(classes []*pkl.Class, name pkl.Identifier) *pkl.Class {
	index := slices.IndexFunc(classes, func(c *pkl.Class) bool { return c.Name == name })
	if index < 0 {
		return nil
	}
	return classes[index]
}

func findProperty(properties []*pkl.Property, name pkl.Identifier) *pkl.Property {
	index := slices.IndexFunc(properties, func(p *pkl.Property) bool { return p.Name == name })
	if index < 0 {
		return nil
	}
	return properties[index]
}

func hasImport(imports []*pkl.Import, path string) bool {
	return slices.ContainsFunc(imports, func(i *pkl.Import) bool { return i.Path == path })
}
