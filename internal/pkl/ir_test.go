package pkl

import "testing"

func TestTypeString(t *testing.T) {
	tests := []struct {
		name string
		typ  *Type
		want string
	}{
		{
			name: "builtin",
			typ:  &Type{Name: "String", Builtin: true},
			want: "String",
		},
		{
			name: "nullable",
			typ:  &Type{Name: "String", Builtin: true, Nullable: true},
			want: "String?",
		},
		{
			name: "listing",
			typ: &Type{Name: "Listing", Arguments: []*Type{
				{Name: "String"},
			}},
			want: "Listing<String>",
		},
		{
			name: "mapping",
			typ: &Type{Name: "Mapping", Arguments: []*Type{
				{Name: "String"}, {Name: "Any"},
			}},
			want: "Mapping<String, Any>",
		},
		{
			name: "nested listing",
			typ: &Type{Name: "Listing", Arguments: []*Type{
				{Name: "Listing", Arguments: []*Type{{Name: "Int"}}},
			}, Nullable: true},
			want: "Listing<Listing<Int>>?",
		},
		{
			name: "union",
			typ: &Type{Members: []*Type{
				{Name: `"none"`}, {Name: `"plain"`},
			}},
			want: `"none"|"plain"`,
		},
		{
			name: "nullable union needs parentheses",
			typ: &Type{Members: []*Type{
				{Name: `"none"`}, {Name: `"plain"`},
			}, Nullable: true},
			want: `("none"|"plain")?`,
		},
		{
			// A union of one takes no parentheses.
			name: "nullable union of one",
			typ:  &Type{Members: []*Type{{Name: `"const"`}}, Nullable: true},
			want: `"const"?`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.typ.String(); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestIdentifierString(t *testing.T) {
	tests := []struct {
		in   Identifier
		want string
	}{
		{"agent_address", "agent_address"},
		{"url", "url"},
		// Pkl keywords need backticks.
		{"module", "`module`"},
		{"function", "`function`"},
		{"when", "`when`"},
		{"local", "`local`"},
		{"switch", "`switch`"},
	}

	for _, test := range tests {
		t.Run(string(test.in), func(t *testing.T) {
			if got := test.in.String(); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestPropertyString(t *testing.T) {
	tests := []struct {
		name string
		prop *Property
		want string
	}{
		{
			name: "no default",
			prop: &Property{Name: "url", Type: &Type{Name: "String"}},
			want: "url: String",
		},
		{
			name: "with default",
			prop: &Property{
				Name:    "port",
				Type:    &Type{Name: "Int"},
				Default: "4195",
			},
			want: "port: Int = 4195",
		},
		{
			name: "with modifier",
			prop: &Property{
				Name:      "version",
				Modifiers: []PropertyModifier{PropertyModifierFixed},
				Type:      &Type{Name: "String"},
				Default:   `"1"`,
			},
			want: `fixed version: String = "1"`,
		},
		{
			name: "keyword name",
			prop: &Property{Name: "module", Type: &Type{Name: "String"}},
			want: "`module`: String",
		},
		{
			// A property that overrides an inherited one keeps its type.
			name: "no type",
			prop: &Property{
				Modifiers: []PropertyModifier{PropertyModifierFixed},
				Name:      "plugin",
				Default:   `"jaeger"`,
			},
			want: `fixed plugin = "jaeger"`,
		},
		{
			name: "fixed and hidden",
			prop: &Property{
				Modifiers: []PropertyModifier{PropertyModifierFixed, PropertyModifierHidden},
				Name:      "plugin",
				Type:      &Type{Name: "String"},
			},
			want: "fixed hidden plugin: String",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.prop.String(); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestClassDeclaration(t *testing.T) {
	tests := []struct {
		name  string
		class *Class
		want  string
	}{
		{
			name:  "plain",
			class: &Class{Name: "Tls"},
			want:  "class Tls",
		},
		{
			name: "modifiers and parent",
			class: &Class{
				Name:      "Base",
				Modifiers: ClassModifiers{ClassModifierAbstract},
				Extends:   "Other",
			},
			want: "abstract class Base extends Other",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.class.Declaration(); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}
