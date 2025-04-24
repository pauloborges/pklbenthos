package pkl

import (
	"errors"
	"strings"
)

// Project is the PklProject of a generated library. It carries nothing yet,
// because the sync command writes the file itself.
type Project struct{}

// Modules holds every module of a library, by its name.
type Modules map[QualifiedIdentifier]*Module

// ModuleModifier is a word that stands in front of a module declaration,
// such as `abstract`.
type ModuleModifier string

// String returns the modifier as Pkl source.
func (m ModuleModifier) String() string {
	return string(m)
}

const (
	ModuleModifierAbstract ModuleModifier = "abstract"
	ModuleModifierOpen     ModuleModifier = "open"
)

// ModuleModifiers is a list of module modifiers.
// FIXME: validate that the modifiers are unique and do not conflict.
type ModuleModifiers []ModuleModifier

// String joins the modifiers with a space, in the order that they carry.
func (m ModuleModifiers) String() string {
	mm := make([]string, len(m))
	for i, modifier := range m {
		mm[i] = modifier.String()
	}
	return strings.Join(mm, " ")
}

// Module represents a Pkl module.
type Module struct {
	// Path is the relative path to the project root.
	Path string

	Documentation string
	Modifiers     ModuleModifiers
	Name          QualifiedIdentifier
	Extends       string
	Amends        string

	Imports     []*Import
	Classes     []*Class
	TypeAliases []*TypeAlias
	Properties  []*Property

	Output *ModuleOutput

	// TODO: annotations
}

// ModuleOutput controls how a module renders.
type ModuleOutput struct {
	// Value is a Pkl expression that the module renders in place of itself.
	// Use it when the configuration needs a top-level property that Pkl keeps
	// for itself, such as `output`. Hold those properties in a class, and
	// render an instance of that class.
	Value string

	// Converters are the entries of the converter mapping of the renderer.
	// The module amends the renderer instead of replacing it, so that the
	// caller still chooses the output format.
	Converters []*Converter
}

// Converter turns a value into another one while a module renders.
type Converter struct {
	// Key is a Pkl expression that gives the class to convert.
	Key string
	// Function is a Pkl lambda that takes the value and returns the
	// replacement.
	Function string
}

// Import brings another module into the one that holds it.
type Import struct {
	Path  string
	Alias string
}

// ClassModifier is a word that stands in front of a class declaration, such
// as `abstract`.
type ClassModifier string

const (
	ClassModifierAbstract ClassModifier = "abstract"
	ClassModifierLocal    ClassModifier = "local"
	ClassModifierOpen     ClassModifier = "open"
)

// ClassModifiers is the list of modifiers of one class.
type ClassModifiers []ClassModifier

// String joins the modifiers with a space, in the order that they carry.
func (m ClassModifiers) String() string {
	mm := make([]string, len(m))
	for i, modifier := range m {
		mm[i] = string(modifier)
	}
	return strings.Join(mm, " ")
}

// Class is a class of a module. A nested object of the schema becomes a class
// beside the module that holds it, and not a module of its own.
type Class struct {
	Documentation string
	Modifiers     ClassModifiers
	Name          Identifier
	Extends       QualifiedIdentifier

	Properties []*Property

	// TODO: annotations
}

// Declaration returns the class header, such as `class Tls extends Base`.
func (c *Class) Declaration() string {
	parts := []string{}

	if m := c.Modifiers.String(); m != "" {
		parts = append(parts, m)
	}
	parts = append(parts, "class", c.Name.String())

	if c.Extends != "" {
		parts = append(parts, "extends", string(c.Extends))
	}

	return strings.Join(parts, " ")
}

// TypeAliasModifier is a word that stands in front of a type alias, such as
// `local`.
type TypeAliasModifier string

const (
	TypeAliasModifierLocal TypeAliasModifier = "local"
)

// TypeAlias gives a name to a type, such as a union of the accepted values of
// a closed string property.
type TypeAlias struct {
	Documentation string
	Modifiers     []TypeAliasModifier
	Name          Identifier
	Type          Type

	// TODO: annotations
}

// PropertyModifier is a word that stands in front of a property, such as
// `hidden` or `fixed`.
type PropertyModifier string

const (
	PropertyModifierAbstract PropertyModifier = "abstract"
	PropertyModifierConst    PropertyModifier = "const"
	PropertyModifierFixed    PropertyModifier = "fixed"
	PropertyModifierHidden   PropertyModifier = "hidden"
	PropertyModifierLocal    PropertyModifier = "local"
)

// Property represents a class or module property.
type Property struct {
	Documentation string
	Modifiers     []PropertyModifier
	Name          Identifier
	Type          *Type

	// Default is a Pkl expression, already in source form. An empty string
	// means that the property has no default. A property whose type is
	// nullable and whose default is empty falls back to null, because null
	// is the default value of every nullable type.
	Default string

	// TODO: annotations
}

// String returns the property as Pkl source, with its modifiers, its type and
// its default.
func (p *Property) String() string {
	var b strings.Builder

	for _, modifier := range p.Modifiers {
		b.WriteString(string(modifier))
		b.WriteString(" ")
	}

	b.WriteString(p.Name.String())

	// A property that only assigns a value, such as `fixed plugin = "kafka"`,
	// takes the type of the property that it overrides.
	if p.Type != nil {
		b.WriteString(": ")
		b.WriteString(p.Type.String())
	}

	if p.Default != "" {
		b.WriteString(" = ")
		b.WriteString(p.Default)
	}

	return b.String()
}

// Identifier is the name of a class, a property or a type argument.
type Identifier string

// String returns the identifier as Pkl source. Pkl needs backticks around an
// identifier that is one of its keywords.
func (i Identifier) String() string {
	if keywords[string(i)] {
		return "`" + string(i) + "`"
	}
	return string(i)
}

// keywords holds every Pkl keyword. A property whose name is a keyword needs
// backticks around it.
var keywords = map[string]bool{
	"abstract": true, "amends": true, "as": true, "class": true,
	"const": true, "else": true, "extends": true, "external": true,
	"false": true, "fixed": true, "for": true, "function": true,
	"hidden": true, "if": true, "import": true, "import*": true,
	"in": true, "is": true, "let": true, "local": true, "module": true,
	"new": true, "nothing": true, "null": true, "open": true, "out": true,
	"outer": true, "read": true, "read*": true, "read?": true, "super": true,
	"this": true, "throw": true, "trace": true, "true": true,
	"typealias": true, "unknown": true, "when": true,
	// Reserved for future use by Pkl.
	"case": true, "delete": true, "override": true, "protected": true,
	"record": true, "switch": true, "vararg": true,
}

// IsReservedModuleProperty tells if Pkl keeps the name for itself on every
// module. A module can not declare a property with such a name, but a class
// can.
func IsReservedModuleProperty(name string) bool {
	return moduleProperties[name]
}

// moduleProperties holds the properties that pkl.base#Module declares. The
// `output` property controls how a module renders.
var moduleProperties = map[string]bool{
	"output": true,
}

// QualifiedIdentifier is a name that carries the names above it, joined by a
// dot, such as `com.redpanda.connect.Configuration`.
type QualifiedIdentifier string

// NewQualifiedIdentifier joins the parts of a name with a dot.
func NewQualifiedIdentifier(name ...string) QualifiedIdentifier {
	return QualifiedIdentifier(strings.Join(name, "."))
}

// Type is the type of a property. It covers a plain name, a name with type
// arguments, a union of members, and a nullable form of each.
type Type struct {
	Name      QualifiedIdentifier
	Arguments []*Type
	Builtin   bool
	Nullable  bool

	Members []*Type
	Default *Type
}

// String returns the type as Pkl source, such as `Listing<String>?`.
func (t *Type) String() string {
	if t == nil {
		return "unknown"
	}

	if len(t.Members) > 0 {
		members := make([]string, len(t.Members))
		for i, member := range t.Members {
			members[i] = member.String()
		}

		union := strings.Join(members, "|")

		switch {
		case !t.Nullable:
			return union
		case len(members) == 1:
			// A union of one is just that member, and it takes no
			// parentheses.
			return union + "?"
		default:
			// A question mark after a union binds to the last member only, so
			// the union needs parentheses.
			return "(" + union + ")?"
		}
	}

	var b strings.Builder
	b.WriteString(string(t.Name))

	if len(t.Arguments) > 0 {
		args := make([]string, len(t.Arguments))
		for i, arg := range t.Arguments {
			args[i] = arg.String()
		}
		b.WriteString("<")
		b.WriteString(strings.Join(args, ", "))
		b.WriteString(">")
	}

	if t.Nullable {
		b.WriteString("?")
	}

	return b.String()
}

// Validate reports what a type says that Pkl does not accept, such as a union
// that also carries a name.
func (t *Type) Validate() error {
	var errs error

	if len(t.Members) > 0 {
		if t.Name != "" {
			errs = errors.Join(errs, errors.New("union type can not have name"))
		}
		if len(t.Arguments) > 0 {
			errs = errors.Join(errs, errors.New("union type can not have type arguments"))
		}
		if t.Builtin {
			errs = errors.Join(errs, errors.New("union type can not be builtin"))
		}

		return errs
	}

	if t.Name == "" {
		errs = errors.Join(errs, errors.New("type name can not be empty"))
	}

	if t.Builtin && len(t.Arguments) > 0 {
		errs = errors.Join(errs, errors.New("builtin type can not have type arguments"))
	}

	return errs
}
