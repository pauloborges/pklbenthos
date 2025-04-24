package benthos

import (
	"encoding/json"
	"fmt"
)

// ParseSchema decodes a configuration schema in the V0 JSON encoding.
//
// The V0 encoding is the full-fidelity export of the schema. It marshals the
// same internal types that Redpanda Connect itself builds the schema from, so
// the types in this file mirror those. The schema of a custom build uses the
// same encoding, and holds the components that the build registers.
//
// This module imports no part of Redpanda Connect. A released build writes
// its own schema with `list --format json-full`, so the schema of any version
// comes from the container image of that version. See internal/distribution.
func ParseSchema(data []byte) (*Schema, error) {
	var schema Schema

	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}

	return &schema, nil
}

// Schema is the configuration schema of a build, in the V0 JSON encoding. It
// holds the root configuration and every component that the build registers.
type Schema struct {
	Version string `json:"version"`
	Date    string `json:"date"`

	Config     []*Property  `json:"config"`
	Buffers    []*Component `json:"buffers"`
	Caches     []*Component `json:"caches"`
	Inputs     []*Component `json:"inputs"`
	Outputs    []*Component `json:"outputs"`
	Metrics    []*Component `json:"metrics"`
	Processors []*Component `json:"processors"`
	RateLimits []*Component `json:"rate-limits"`
	Scanners   []*Component `json:"scanners"`
	Tracers    []*Component `json:"tracers"`
}

// Type is the type of a property, such as a string, an object, or a component
// of a family.
type Type string

const (
	TypeBool    Type = "bool"
	TypeInt     Type = "int"
	TypeFloat   Type = "float"
	TypeString  Type = "string"
	TypeObject  Type = "object"
	TypeUnknown Type = "unknown"

	// A property with one of these types holds a component of that type.
	TypeBuffer    Type = "buffer"
	TypeCache     Type = "cache"
	TypeInput     Type = "input"
	TypeOutput    Type = "output"
	TypeMetrics   Type = "metrics"
	TypeProcessor Type = "processor"
	TypeRateLimit Type = "rate_limit"
	TypeScanner   Type = "scanner"
	TypeTracer    Type = "tracer"
)

// IsPrimitive tells if the type holds a single value that Pkl has a type for.
func (t Type) IsPrimitive() bool {
	switch t {
	case TypeBool, TypeInt, TypeFloat, TypeString:
		return true
	default:
		return false
	}
}

// IsComponent tells if the type refers to a component, such as an input or a
// processor.
func (t Type) IsComponent() bool {
	switch t {
	case TypeBuffer, TypeCache, TypeInput, TypeOutput, TypeMetrics,
		TypeProcessor, TypeRateLimit, TypeScanner, TypeTracer:
		return true
	default:
		return false
	}
}

// Kind tells how many values a property holds, and how they are keyed.
type Kind string

const (
	// The schema leaves the kind empty for some object properties. Read an
	// empty kind as a scalar.
	KindUnset   Kind = ""
	KindScalar  Kind = "scalar"
	KindArray   Kind = "array"
	Kind2DArray Kind = "2darray"
	KindMap     Kind = "map"
)

// Component is one plugin of a build, such as the `kafka` input.
type Component struct {
	Name        string   `json:"name"`
	Type        Type     `json:"type"`
	Status      string   `json:"status"`
	Plugin      bool     `json:"plugin"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	Version     string   `json:"version"`

	// SupportLevel is an abstract concept that a distribution of Redpanda
	// Connect sets. The community build leaves it empty.
	SupportLevel string `json:"support_level"`

	// Footnotes holds text that goes at the bottom of the documentation page
	// of the component.
	Footnotes string `json:"footnotes"`

	// Examples shows the component in use.
	Examples []*Example `json:"examples"`

	// Config is an unnamed object property. Its children are the
	// configuration fields of the component.
	Config *Property `json:"config"`
}

// Example is one annotated configuration snippet of a component.
type Example struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Config  string `json:"config"`
}

// Property is one configuration field, of a component or of the root.
type Property struct {
	Name        string `json:"name"`
	Type        Type   `json:"type"`
	Kind        Kind   `json:"kind"`
	Description string `json:"description"`

	// ShortDescription is a one sentence summary in plain text. The schema
	// leaves it empty for most properties, and a reader then falls back to
	// Description.
	ShortDescription string `json:"short_description"`

	// Default holds the default value of the property, and is nil when the
	// schema gives the property no default at all.
	//
	// The pointer keeps those two cases apart, because the schema also states
	// a default of null for a few properties. Such a property has a non-nil
	// Default that points to a nil value, and a configuration may leave it
	// out. See IsRequired.
	//
	// Only (possibly) set when Type is NOT TypeObject.
	Default *any `json:"default"`

	// Only set when Type is TypeObject.
	Children []*Property `json:"children"`

	IsOptional   bool `json:"is_optional"`
	IsDeprecated bool `json:"is_deprecated"`
	IsSecret     bool `json:"is_secret"`

	// IsAdvanced marks a property that most configurations leave out.
	IsAdvanced bool `json:"is_advanced"`

	// Interpolated tells if the property accepts interpolation functions,
	// such as `${! json("id") }`.
	Interpolated bool `json:"interpolated"`

	// Bloblang tells if a string property holds a Bloblang mapping.
	Bloblang bool `json:"bloblang"`

	// Examples holds example values of the property.
	Examples []any `json:"examples"`

	// Version is the release of Redpanda Connect that added the property.
	Version string `json:"version"`

	// Linter is a Bloblang mapping that Redpanda Connect runs to check the
	// value of the property.
	Linter string `json:"linter"`

	// Scrubber is a Bloblang mapping that Redpanda Connect runs to hide the
	// value of the property when it echoes a configuration.
	Scrubber string `json:"scrubber"`

	// Options holds the accepted values of a closed string property.
	Options []string `json:"options"`

	// AnnotatedOptions holds the accepted values of a closed string
	// property, together with a description of each one. Each entry is a
	// pair of value and description.
	AnnotatedOptions [][]string `json:"annotated_options"`
}

// HasDefault tells if the schema gives the property a default. A default of
// null counts, because a configuration may still leave the property out.
func (p *Property) HasDefault() bool {
	return p.Default != nil
}

// DefaultValue returns the default value of the property, and false when the
// schema gives it no default.
func (p *Property) DefaultValue() (any, bool) {
	if p.Default == nil {
		return nil, false
	}
	return *p.Default, true
}

// IsRequired tells if a configuration must set the property.
func (p *Property) IsRequired() bool {
	return !p.HasDefault() && !p.IsOptional
}
