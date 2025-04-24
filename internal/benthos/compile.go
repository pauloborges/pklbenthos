package benthos

import (
	"fmt"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/pauloborges/pklbenthos/internal/pkl"
)

// CompileOptions controls the shape of the generated library. The zero value
// of a field selects the default.
type CompileOptions struct {
	ModulePrefix string

	// ProductName is the name of the program that reads the configuration,
	// such as "Redpanda Connect". The documentation of the generated modules
	// speaks of the product by this name. An empty name gives
	// defaultProductName.
	ProductName string

	// DocsBaseURL is the root of the documentation site of the build, such as
	// "https://docs.redpanda.com/redpanda-connect". The compiler turns each
	// cross reference of the schema into a link under this root. An empty URL
	// leaves a cross reference as the schema wrote it.
	DocsBaseURL string
	// TODO: PklProject options
}

// defaultProductName is the name that the generated documentation gives to the
// program that reads the configuration. Every build of Benthos reads the same
// shape of configuration, so the plain name of the project fits a build that
// gives no name of its own.
const defaultProductName = "Benthos"

// productName returns the name of the program that reads the configuration.
func (o *CompileOptions) productName() string {
	if o.ProductName == "" {
		return defaultProductName
	}

	return o.ProductName
}

// Compile generates a set of Pkl modules for the given configuration schema.
func Compile(schema *Schema, opts *CompileOptions) (pkl.Modules, error) {
	e := &env{
		opts:        opts,
		schema:      schema,
		modules:     make(pkl.Modules),
		rootModules: make(map[string]bool),
	}

	// Keep the names of the modules that the compiler always writes, so that
	// a field of the root configuration never takes one of them.
	e.rootModules["Configuration"] = true
	e.rootModules[pluginBaseModule] = true
	for _, name := range componentModules {
		e.rootModules[name] = true
	}

	e.createPluginBaseModule()

	for _, group := range e.groups() {
		if err := e.createComponentModule(group); err != nil {
			return nil, fmt.Errorf("create %s module: %w", group.Module, err)
		}

		for _, component := range group.Components {
			if err := e.createPluginModule(group, component); err != nil {
				return nil, fmt.Errorf("create %s plugin %q: %w",
					group.Module, component.Name, err)
			}
		}
	}

	if err := e.createConfigurationModule(); err != nil {
		return nil, fmt.Errorf("create Configuration module: %w", err)
	}

	return e.modules, nil
}

type env struct {
	opts    *CompileOptions
	schema  *Schema
	modules pkl.Modules

	// rootModules holds the short names of the modules at the root of the
	// library, so that two of them never take the same file.
	rootModules map[string]bool
}

// product returns the name that the generated documentation gives to the
// program that reads the configuration.
func (e *env) product() string {
	return e.opts.productName()
}

// reserveRootModule returns a short name that no other module at the root of
// the library uses.
func (e *env) reserveRootModule(base string) string {
	name := base
	for i := 2; e.rootModules[name]; i++ {
		name = base + strconv.Itoa(i)
	}

	e.rootModules[name] = true

	return name
}

// componentGroup is one family of components, such as every tracer.
type componentGroup struct {
	// Module is the name of the abstract module of the family, such as
	// "Tracer".
	Module string

	// Dir is the directory that holds the plugin modules of the family, such
	// as "tracers".
	Dir string

	Components []*Component
}

func (e *env) groups() []componentGroup {
	return []componentGroup{
		{Module: "Buffer", Dir: "buffers", Components: e.schema.Buffers},
		{Module: "Cache", Dir: "caches", Components: e.schema.Caches},
		{Module: "Input", Dir: "inputs", Components: e.schema.Inputs},
		{Module: "Output", Dir: "outputs", Components: e.schema.Outputs},
		{Module: "Metrics", Dir: "metrics", Components: e.schema.Metrics},
		{Module: "Processor", Dir: "processors", Components: e.schema.Processors},
		{Module: "RateLimit", Dir: "rate_limits", Components: e.schema.RateLimits},
		{Module: "Scanner", Dir: "scanners", Components: e.schema.Scanners},
		{Module: "Tracer", Dir: "tracers", Components: e.schema.Tracers},
	}
}

// componentModules maps a property type to the abstract module that holds the
// components of that type.
var componentModules = map[Type]string{
	TypeBuffer:    "Buffer",
	TypeCache:     "Cache",
	TypeInput:     "Input",
	TypeOutput:    "Output",
	TypeMetrics:   "Metrics",
	TypeProcessor: "Processor",
	TypeRateLimit: "RateLimit",
	TypeScanner:   "Scanner",
	TypeTracer:    "Tracer",
}

// createConfigurationModule creates the root module of the library.
//
// The root fields live in a class rather than on the module itself, because
// one of them is named `output`, and Pkl keeps that name for the property that
// controls how a module renders. The module renders the class instead of
// itself, so the generated YAML still holds the root fields at the top level.
func (e *env) createConfigurationModule() error {
	shortName := "Configuration"

	module := &pkl.Module{
		Path:          e.modulePath(shortName, nil),
		Name:          e.moduleName(shortName),
		Documentation: "Root configuration of a " + e.product() + " pipeline.",
	}
	e.modules[module.Name] = module

	b := e.newBuilder(module, nil)

	root := &pkl.Class{
		Name:          b.className([]string{configurationRootClass}),
		Documentation: "The fields of a " + e.product() + " configuration file.",
	}
	module.Classes = append(module.Classes, root)

	for _, prop := range e.schema.Config {
		root.Properties = append(root.Properties, e.rootProperty(b, prop))
	}

	// The configuration itself keeps a value, because it is what a
	// configuration file amends.
	config := &pkl.Property{
		Name:          configurationProperty,
		Type:          &pkl.Type{Name: pkl.QualifiedIdentifier(root.Name)},
		Documentation: "The configuration to render.",
		Default:       "new {}",
	}
	module.Properties = append(module.Properties, config)

	// Redpanda Connect selects a component with a single-key object. A plugin
	// keeps its fields at the top level, so the renderer wraps each one in the
	// name of its plugin.
	b.importRoot(pluginBaseModule)

	module.Output = &pkl.ModuleOutput{
		Value: string(configurationProperty),
		Converters: []*pkl.Converter{{
			Key: pluginBaseModule + ".getClass()",
			Function: "(it) -> Map(it." + string(pluginProperty) +
				", it." + string(pluginValueProperty) + ")",
		}},
	}

	return nil
}

// rootProperty converts one field of the root configuration.
//
// A field that holds an object goes in a module of its own, such as Http.pkl,
// to keep Configuration.pkl small. The whole subtree of the field goes in that
// one module: a nested object becomes a class beside it, not another module.
// Every other field stays where it is.
func (e *env) rootProperty(b *builder, prop *Property) *pkl.Property {
	if !e.extractsToModule(prop) {
		return b.property(prop, nil)
	}

	name := e.createFieldModule(prop)
	b.importRoot(name)

	out := &pkl.Property{
		Documentation: e.propertyDocumentation(prop),
		Name:          pkl.Identifier(prop.Name),
		Type:          wrapKind(&pkl.Type{Name: pkl.QualifiedIdentifier(name)}, prop.Kind),
	}

	// A nullable object still takes an amend, at any depth, so
	// `http { cors { enabled = true } }` reads the same as before. A block
	// that no one touches stays out of the rendered file.
	out.Type.Nullable = true

	return out
}

// extractsToModule tells if a field of the root configuration goes in a module
// of its own.
func (e *env) extractsToModule(prop *Property) bool {
	if prop.Type != TypeObject || len(prop.Children) == 0 {
		return false
	}

	// The fields of the module sit at the top level, so a field with a name
	// that Pkl keeps for itself has to stay in a class instead.
	for _, child := range prop.Children {
		if pkl.IsReservedModuleProperty(child.Name) {
			return false
		}
	}

	return true
}

// createFieldModule writes one field of the root configuration to its own
// module, and returns the short name of that module.
func (e *env) createFieldModule(prop *Property) string {
	shortName := e.reserveRootModule(pascal(prop.Name))

	module := &pkl.Module{
		Path:          e.modulePath(shortName, nil),
		Name:          e.moduleName(shortName),
		Documentation: e.propertyDocumentation(prop),
	}
	e.modules[module.Name] = module

	b := e.newBuilder(module, nil)
	for _, child := range prop.Children {
		module.Properties = append(module.Properties, b.property(child, nil))
	}

	return shortName
}

const (
	configurationRootClass = "Root"
	configurationProperty  = pkl.Identifier("config")

	// pluginBaseModule is the module that every component plugin extends.
	pluginBaseModule = "Plugin"
	pluginProperty   = pkl.Identifier("plugin")

	// pluginValueProperty holds what the plugin renders under its name.
	pluginValueProperty = pkl.Identifier("pluginValue")

	// valueProperty holds the configuration of a plugin that takes a single
	// value instead of a set of fields.
	valueProperty = pkl.Identifier("value")
)

// createPluginBaseModule creates the module that every component plugin
// extends, through the abstract module of its family. It holds the name that
// Redpanda Connect uses to select the plugin, and the value that goes under
// that name.
//
// Both properties are hidden, so they stay out of the fields of the plugin.
// The name is also fixed, so a configuration can not change it. The
// Configuration module converts a plugin into a single-key object.
func (e *env) createPluginBaseModule() {
	module := &pkl.Module{
		Path:      e.modulePath(pluginBaseModule, nil),
		Name:      e.moduleName(pluginBaseModule),
		Modifiers: pkl.ModuleModifiers{pkl.ModuleModifierAbstract},
		Documentation: "Base module of every component plugin.\n\n" +
			e.product() + " selects a component with a single-key object, " +
			"such as `tracer: {jaeger: {...}}`. A plugin declares its fields " +
			"at the top level, and the Configuration module wraps them in the " +
			"name of the plugin when it renders.",
		Properties: []*pkl.Property{{
			Documentation: "The name that " + e.product() +
				" uses to select this plugin.",
			Modifiers: []pkl.PropertyModifier{
				pkl.PropertyModifierFixed,
				pkl.PropertyModifierHidden,
			},
			Name: pluginProperty,
			Type: stringType(),
		}, {
			Documentation: "What this plugin renders under its name.\n\n" +
				"Most plugins take a set of fields, and those fields are the " +
				"properties of the module, which is what `toMap()` gives. A " +
				"plugin that takes a single value, such as the `mapping` " +
				"processor, overrides this property instead.",
			Modifiers: []pkl.PropertyModifier{pkl.PropertyModifierHidden},
			Name:      pluginValueProperty,
			Type:      anyType(),
			Default:   "toMap()",
		}},
	}

	e.modules[module.Name] = module
}

// createComponentModule creates the abstract module that every plugin of a
// family extends. A property that holds a component of the family uses this
// module as its type.
func (e *env) createComponentModule(group componentGroup) error {
	module := &pkl.Module{
		Path:      e.modulePath(group.Module, nil),
		Name:      e.moduleName(group.Module),
		Modifiers: pkl.ModuleModifiers{pkl.ModuleModifierAbstract},
		Extends:   pluginBaseModule + ".pkl",
		Documentation: fmt.Sprintf(
			"Base module of every %s component.\n\nEach plugin of the family extends this module.",
			group.Module),
	}
	e.modules[module.Name] = module

	return nil
}

// createPluginModule creates the module of one plugin, such as the Jaeger
// tracer. A fixed property holds the name that selects the plugin.
//
// The fields of the plugin sit at the top level of the module. A plugin with
// no fields, such as the `none` tracer, keeps the name alone, and the
// converter turns it into an empty object. A plugin that takes a single value
// instead of a set of fields holds that value in one property. See
// takesFieldSet.
func (e *env) createPluginModule(group componentGroup, component *Component) error {
	if component.Config == nil {
		return fmt.Errorf("component has no config")
	}

	dir := []string{group.Dir}
	shortName := pascal(component.Name)

	module := &pkl.Module{
		Path:          e.modulePath(shortName, dir),
		Name:          e.moduleName(group.Dir, shortName),
		Documentation: e.componentDocumentation(component),
		Extends:       relativeToRoot(dir, group.Module+".pkl"),
		Properties: []*pkl.Property{{
			Modifiers: []pkl.PropertyModifier{pkl.PropertyModifierFixed},
			Name:      pluginProperty,
			Default:   pklString(component.Name),
		}},
	}
	e.modules[module.Name] = module

	b := e.newBuilder(module, dir)

	if !takesFieldSet(component.Config) {
		module.Properties = append(module.Properties,
			valuePluginProperties(b, component)...)
		return nil
	}

	// renamed holds the fields that the module declares under another name.
	// See renamedFieldName.
	var renamed [][2]string

	for _, field := range component.Config.Children {
		prop := b.property(field, nil)

		if pkl.IsReservedModuleProperty(field.Name) {
			prop.Name = renamedFieldName(field.Name)
			prop.Modifiers = append(prop.Modifiers, pkl.PropertyModifierHidden)
			prop.Documentation = noteRenamedField(prop.Documentation, field.Name)
			renamed = append(renamed, [2]string{field.Name, string(prop.Name)})
		}

		module.Properties = append(module.Properties, prop)
	}

	if len(renamed) > 0 {
		module.Properties = append(module.Properties, &pkl.Property{
			Documentation: "Puts each renamed field back under the name that " +
				e.product() + " reads.",
			Name:    pluginValueProperty,
			Default: putRenamedFields(renamed),
		})
	}

	return nil
}

// takesFieldSet tells if the configuration of a plugin is a set of named
// fields, which the module holds as the properties at its top level.
//
// Every other plugin takes a single value under its name, such as the
// `mapping` processor, which takes a Bloblang mapping as a string, or the
// `fallback` output, which takes a list of outputs. An object with a kind
// other than scalar counts as a value as well, because the plugin then takes a
// list or a map of such objects, not one of them.
func takesFieldSet(config *Property) bool {
	if config.Type != TypeObject {
		return false
	}

	return config.Kind == KindScalar || config.Kind == KindUnset
}

// valuePluginProperties builds the properties of a plugin that takes a single
// value. The value goes in one property, and pluginValue points at it, so the
// plugin renders the bare value under its name.
func valuePluginProperties(b *builder, component *Component) []*pkl.Property {
	config := component.Config

	documentation := "The value that this plugin takes.\n\n" +
		b.env.product() + " reads a single value under the name of this " +
		"plugin, not a set of fields."
	if text := b.env.propertyDocumentation(config); text != "" {
		documentation = text + "\n\n" + documentation
	}

	value := &pkl.Property{
		Documentation: documentation,
		Name:          valueProperty,

		// The name of the component gives the generated classes a name, such
		// as the `GroupBy` of the `group_by` processor.
		Type: wrapKind(b.baseType(config, []string{component.Name}), config.Kind),
	}

	if !config.IsRequired() || isBlock(config) {
		value.Type.Nullable = true
		value.Documentation = documentDefault(value.Documentation, config, value.Type)
	}

	return []*pkl.Property{value, {
		Name:    pluginValueProperty,
		Default: string(valueProperty),
	}}
}

// renamedFieldName returns the name that a module gives to a field whose own
// name Pkl keeps for itself, such as the `output` field of the `drop_on`
// output. React does the same with `htmlFor`.
func renamedFieldName(name string) pkl.Identifier {
	return pkl.Identifier("yaml" + pascal(name))
}

func noteRenamedField(documentation, name string) string {
	note := fmt.Sprintf(
		"Pkl keeps the name `%s` for itself on every module, so this property "+
			"takes another name. It still renders as `%s`.", name, name)

	if documentation == "" {
		return note
	}

	return documentation + "\n\n" + note
}

// putRenamedFields writes the expression that a plugin with a renamed field
// gives to pluginValue. A renamed property is hidden, so `toMap()` leaves it
// out, and the expression puts it back under the name that the YAML needs.
func putRenamedFields(renamed [][2]string) string {
	var b strings.Builder
	b.WriteString("toMap()")

	for _, pair := range renamed {
		b.WriteString(".put(")
		b.WriteString(pklString(pair[0]))
		b.WriteString(", ")
		b.WriteString(pkl.Identifier(pair[1]).String())
		b.WriteString(")")
	}

	return b.String()
}

func (e *env) moduleName(parts ...string) pkl.QualifiedIdentifier {
	if e.opts.ModulePrefix != "" {
		parts = append([]string{e.opts.ModulePrefix}, parts...)
	}
	return pkl.NewQualifiedIdentifier(parts...)
}

func (e *env) modulePath(shortName string, dir []string) string {
	parts := make([]string, 0, len(dir)+1)
	parts = append(parts, dir...)
	parts = append(parts, shortName+".pkl")

	return path.Join(parts...)
}

// relativeToRoot returns the path of a file that sits at the root of the
// generated library, seen from a module inside dir.
func relativeToRoot(dir []string, file string) string {
	if len(dir) == 0 {
		return file
	}
	return strings.Repeat("../", len(dir)) + file
}

// builder collects the classes, the imports and the properties of one module.
type builder struct {
	env    *env
	module *pkl.Module
	dir    []string

	imported map[string]bool
	classes  map[pkl.Identifier]bool
}

func (e *env) newBuilder(module *pkl.Module, dir []string) *builder {
	b := &builder{
		env:      e,
		module:   module,
		dir:      dir,
		imported: make(map[string]bool),
		classes:  make(map[pkl.Identifier]bool),
	}

	// An import binds the name of the module it points to. Keep those names
	// out of the generated class names, so that a field named `input` does
	// not shadow the Input module.
	b.classes[pkl.Identifier(pluginBaseModule)] = true
	for _, name := range componentModules {
		b.classes[pkl.Identifier(name)] = true
	}

	return b
}

// importRoot imports a module that sits at the root of the generated library.
func (b *builder) importRoot(shortName string) {
	target := relativeToRoot(b.dir, shortName+".pkl")
	if b.imported[target] {
		return
	}

	b.imported[target] = true
	b.module.Imports = append(b.module.Imports, &pkl.Import{Path: target})
}

// property converts a schema property to a Pkl property. The path holds the
// names of the ancestors of the property, and gives the generated classes
// names that do not collide.
//
// A property that a configuration may leave out is nullable and carries no
// value. See documentDefault for why.
func (b *builder) property(prop *Property, ancestors []string) *pkl.Property {
	out := &pkl.Property{
		Documentation: b.env.propertyDocumentation(prop),
		Name:          pkl.Identifier(prop.Name),
	}

	// Copy the path, so that a sibling property does not overwrite the tail
	// of the slice that a previous sibling still holds.
	path := make([]string, 0, len(ancestors)+1)
	path = append(path, ancestors...)
	path = append(path, prop.Name)

	out.Type = wrapKind(b.baseType(prop, path), prop.Kind)

	if prop.IsRequired() && !isBlock(prop) {
		return out
	}

	out.Type.Nullable = true
	out.Documentation = documentDefault(out.Documentation, prop, out.Type)

	return out
}

// documentDefault adds the default of a property to its documentation.
//
// The generated modules hold no default values. A configuration that repeats a
// default keeps it even after Redpanda Connect changes that default, and the
// rendered file says nothing about what the author chose. An unset property
// stays out of the rendered file, and Redpanda Connect applies its own
// default, so the documentation is the place for it.
func documentDefault(documentation string, prop *Property, typ *pkl.Type) string {
	value, ok := prop.DefaultValue()
	if !ok || isEmptyDefault(value) {
		return documentation
	}

	literal, ok := pklLiteral(value, typ)
	if !ok {
		// The default selects a component, or holds a shape that this
		// compiler can not write yet.
		//
		// TODO: describe the remaining defaults.
		return documentation
	}

	note := "Default: `" + literal + "`"
	if documentation == "" {
		return note
	}

	return documentation + "\n\n" + note
}

// baseType returns the Pkl type of a property, before the kind of the property
// wraps it in a Listing or a Mapping.
func (b *builder) baseType(prop *Property, path []string) *pkl.Type {
	switch {
	case prop.Type == TypeObject && len(prop.Children) > 0:
		return &pkl.Type{Name: pkl.QualifiedIdentifier(b.class(prop, path))}

	case prop.Type == TypeObject:
		// An object with no children accepts any key.
		return mappingType(anyType())

	case prop.Type.IsComponent():
		module := componentModules[prop.Type]
		b.importRoot(module)
		return &pkl.Type{Name: pkl.QualifiedIdentifier(module)}

	case prop.Type == TypeString:
		if members := optionMembers(prop); len(members) > 0 {
			return &pkl.Type{Members: members}
		}
	}

	return &pkl.Type{Name: builtinTypes[prop.Type], Builtin: true}
}

// class adds a class for an object property, and returns its name.
func (b *builder) class(prop *Property, path []string) pkl.Identifier {
	class := &pkl.Class{Name: b.className(path)}
	b.module.Classes = append(b.module.Classes, class)

	for _, child := range prop.Children {
		class.Properties = append(class.Properties, b.property(child, path))
	}

	return class.Name
}

// className builds a unique class name from the path of a property. The path
// keeps the name unique when two objects in the same module share a leaf name,
// such as the `tls` of a broker and the `tls` of a schema registry.
func (b *builder) className(path []string) pkl.Identifier {
	base := pascal(path...)
	if base == "" {
		base = "Config"
	}

	name := pkl.Identifier(base)
	for i := 2; b.classes[name]; i++ {
		name = pkl.Identifier(base + strconv.Itoa(i))
	}

	b.classes[name] = true

	return name
}

var builtinTypes = map[Type]pkl.QualifiedIdentifier{
	TypeBool:    "Boolean",
	TypeInt:     "Int",
	TypeFloat:   "Float",
	TypeString:  "String",
	TypeUnknown: "Any",
}

func anyType() *pkl.Type    { return &pkl.Type{Name: "Any", Builtin: true} }
func stringType() *pkl.Type { return &pkl.Type{Name: "String", Builtin: true} }

func listingType(element *pkl.Type) *pkl.Type {
	return &pkl.Type{Name: "Listing", Arguments: []*pkl.Type{element}}
}

func mappingType(value *pkl.Type) *pkl.Type {
	return &pkl.Type{Name: "Mapping", Arguments: []*pkl.Type{stringType(), value}}
}

// wrapKind applies the kind of a property to its type. A Redpanda Connect
// property holds its element type and its shape apart: a list of strings has
// the type `string` and the kind `array`.
func wrapKind(base *pkl.Type, kind Kind) *pkl.Type {
	switch kind {
	case KindArray:
		return listingType(base)
	case Kind2DArray:
		return listingType(listingType(base))
	case KindMap:
		return mappingType(base)
	default:
		return base
	}
}

// isBlock tells if a property holds an object that has fields of its own.
//
// A configuration may leave out a whole block, and Redpanda Connect then
// applies the defaults of every field in it. The schema marks no block as
// optional, so a block is nullable whatever the schema says. A field that the
// block itself requires stays required inside it, which is what a
// configuration needs: leave out `redpanda` and nothing else matters, but use
// it and it needs its `seed_brokers`.
func isBlock(prop *Property) bool {
	return prop.Type == TypeObject && len(prop.Children) > 0
}

// isEmptyDefault tells if a default from the schema holds nothing. Leaving
// such a field out of a configuration says the same as writing it, so the
// documentation gains nothing from it.
func isEmptyDefault(value any) bool {
	switch v := value.(type) {
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	default:
		return false
	}
}

// optionMembers turns the closed set of values of a string property into the
// members of a Pkl union, such as `"none"|"PLAIN"`.
func optionMembers(prop *Property) []*pkl.Type {
	values := prop.Options

	if len(values) == 0 {
		for _, option := range prop.AnnotatedOptions {
			if len(option) > 0 {
				values = append(values, option[0])
			}
		}
	}

	members := make([]*pkl.Type, 0, len(values))
	for _, value := range values {
		members = append(members, &pkl.Type{Name: pkl.QualifiedIdentifier(pklString(value))})
	}

	return members
}

// pklLiteral writes a default value from the schema as Pkl source. It reports
// false when the value has a shape that it can not write, such as the
// single-key object that selects a component.
func pklLiteral(value any, typ *pkl.Type) (string, bool) {
	switch v := value.(type) {
	case bool:
		return strconv.FormatBool(v), true

	case string:
		return pklString(v), true

	case float64:
		return pklNumber(v, typ), true

	case []any:
		return pklListing(v, typ)

	case map[string]any:
		return pklMapping(v, typ)

	default:
		// A nil default means that the schema states the default is null.
		return "", false
	}
}

func pklListing(items []any, typ *pkl.Type) (string, bool) {
	if len(items) == 0 {
		return "new {}", true
	}

	element := typeArgument(typ, 0)

	parts := make([]string, 0, len(items))
	for _, item := range items {
		literal, ok := pklLiteral(item, element)
		if !ok {
			return "", false
		}
		parts = append(parts, literal)
	}

	return "new { " + strings.Join(parts, " ") + " }", true
}

func pklMapping(entries map[string]any, typ *pkl.Type) (string, bool) {
	if len(entries) == 0 {
		return "new {}", true
	}

	// Only a Mapping takes entries. An object default belongs to a class, and
	// a class needs its own syntax.
	if typ == nil || typ.Name != "Mapping" {
		return "", false
	}

	value := typeArgument(typ, 1)

	parts := make([]string, 0, len(entries))
	for _, key := range sortedKeys(entries) {
		literal, ok := pklLiteral(entries[key], value)
		if !ok {
			return "", false
		}
		parts = append(parts, "["+pklString(key)+"] = "+literal)
	}

	return "new { " + strings.Join(parts, "; ") + " }", true
}

func typeArgument(typ *pkl.Type, index int) *pkl.Type {
	if typ == nil || index >= len(typ.Arguments) {
		return nil
	}
	return typ.Arguments[index]
}

func pklNumber(value float64, typ *pkl.Type) string {
	if typ != nil && typ.Name == "Float" {
		text := strconv.FormatFloat(value, 'f', -1, 64)
		if !strings.ContainsAny(text, ".eE") {
			text += ".0"
		}
		return text
	}

	if value == math.Trunc(value) && !math.IsInf(value, 0) {
		return strconv.FormatInt(int64(value), 10)
	}

	return strconv.FormatFloat(value, 'f', -1, 64)
}

// pklString quotes a string as Pkl source. It escapes the backslash first,
// which also stops a `\(` sequence from starting an interpolation.
func pklString(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	)

	return `"` + replacer.Replace(value) + `"`
}

func sortedKeys(entries map[string]any) []string {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}

	// A stable order keeps the generated library reproducible.
	slices.Sort(keys)

	return keys
}

// pascal turns the snake_case names of a schema into a single PascalCase
// identifier, such as `redpanda_migrator` and `tls` into `RedpandaMigratorTls`.
func pascal(parts ...string) string {
	var b strings.Builder

	for _, part := range parts {
		for word := range strings.SplitSeq(part, "_") {
			if word == "" {
				continue
			}
			b.WriteString(strings.ToUpper(word[:1]))
			b.WriteString(word[1:])
		}
	}

	return b.String()
}

// docs prepares text that the schema wrote for the documentation site of the
// build. The schema writes AsciiDoc, and a Pkl doc comment holds Markdown.
func (e *env) docs(text string) string {
	return toMarkdown(text, e.opts.DocsBaseURL)
}

func (e *env) componentDocumentation(component *Component) string {
	var parts []string

	if component.Summary != "" {
		parts = append(parts, e.docs(strings.TrimSpace(component.Summary)))
	}
	if component.Description != "" {
		parts = append(parts, e.docs(strings.TrimSpace(component.Description)))
	}
	if component.Status != "" {
		parts = append(parts, "Status: "+component.Status)
	}

	return strings.Join(parts, "\n\n")
}

func (e *env) propertyDocumentation(prop *Property) string {
	var parts []string

	if prop.Description != "" {
		parts = append(parts, e.docs(strings.TrimSpace(prop.Description)))
	}
	if prop.IsDeprecated {
		parts = append(parts, "Deprecated.")
	}
	if prop.IsSecret {
		parts = append(parts, "This field holds a secret.")
	}

	return strings.Join(parts, "\n\n")
}
