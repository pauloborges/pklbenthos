package benthos

import (
	"regexp"
	"strings"
	"testing"
)

func TestToMarkdownBlocks(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			// Antora gives the title of a page one equals sign, and the schema
			// holds the body of a page, so its highest title carries two.
			name: "section titles take the level below their own",
			text: "== Write modes\n\ntext\n\n=== Auto-create\n\n==== Deep",
			want: "## Write modes\n\ntext\n\n### Auto-create\n\n#### Deep",
		},
		{
			name: "an inline call-out becomes a quote",
			text: "IMPORTANT: This input tracks its position in memory only.",
			want: "> **Important:** This input tracks its position in memory only.",
		},
		{
			name: "a call-out block becomes a quote",
			text: "[CAUTION]\n====\n`insert` is unconditional.\n====",
			want: "> **Caution**\n>\n> `insert` is unconditional.\n",
		},
		{
			name: "a block title becomes bold text",
			text: ".Endpoint caveats\nSome prose.",
			want: "**Endpoint caveats**\nSome prose.",
		},
		{
			// The definition tells Antora a value for its own page template.
			// No text of the schema reads one back.
			name: "an attribute definition goes",
			text: ":driver-support: mysql=certified, postgres=certified\nProse stays.",
			want: "Prose stays.",
		},
		{
			name: "a table of bars becomes a table of Markdown",
			text: "Intro.\n\n|===\n| Driver | DSN Format\n\n| `mysql` \n| `user@host` \n\n| `postgres` \n| `postgres://host` \n|===",
			want: "Intro.\n\n| Driver | DSN Format |\n| --- | --- |\n| `mysql` | `user@host` |\n| `postgres` | `postgres://host` |\n",
		},
		{
			name: "a table of separated values becomes a table of Markdown",
			text: "[%header,format=dsv]\n|===\nBloblang type:Iceberg type\nstring:string\nbytes:binary\n|===",
			want: "| Bloblang type | Iceberg type |\n| --- | --- |\n| string | string |\n| bytes | binary |\n",
		},
		{
			// A cell of a later row may hold the separator itself, so a row
			// splits into no more cells than the header has.
			name: "a separator inside a cell stays in the cell",
			text: "[%header,format=dsv]\n|===\nType:Meaning\nBOOLEAN:bool, or a string that reads: true\n|===",
			want: "| Type | Meaning |\n| --- | --- |\n| BOOLEAN | bool, or a string that reads: true |\n",
		},
		{
			// A cell of Markdown ends at a bar. A table of separated values
			// splits on its own separator, so a bar reaches a cell whole.
			name: "a bar inside a cell takes a backslash",
			text: "[%header,format=dsv]\n|===\nName:Pattern\nalt:`a|b`\n|===",
			want: "| Name | Pattern |\n| --- | --- |\n| alt | `a\\|b` |\n",
		},
		{
			name: "a fenced block keeps every line",
			text: "Prose.\n\n```yaml\n== not a title\nNOTE: not a call-out\n```",
			want: "Prose.\n\n```yaml\n== not a title\nNOTE: not a call-out\n```",
		},
		{
			name: "an indented block after a blank line keeps every line",
			text: "Prose.\n\n    == not a title\n    NOTE: not a call-out",
			want: "Prose.\n\n    == not a title\n    NOTE: not a call-out",
		},
		{
			name: "a link inside a table cell resolves",
			text: "|===\n| Driver | DSN\n\n| `trino` \n| https://example.com/dsn[the format^] \n|===",
			want: "| Driver | DSN |\n| --- | --- |\n| `trino` | [the format](https://example.com/dsn) |\n",
		},
		{
			// AsciiDoc reads "{name}" as an attribute, so an author who wants
			// the braces writes "\{name}". Markdown has no such attribute and
			// would show the backslash.
			name: "an escaped brace loses its backslash",
			text: "POSTs to the `/channels/\\{channel_id}/messages` endpoint.",
			want: "POSTs to the `/channels/{channel_id}/messages` endpoint.",
		},
		{
			name: "prose with no markup stays as it is",
			text: "A plain sentence.\n\nAnother one.",
			want: "A plain sentence.\n\nAnother one.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := toMarkdown(test.text, testDocs)

			if got != test.want {
				t.Errorf("toMarkdown()\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

// leftovers holds the markup that must not reach a doc comment. Each entry
// names a construct of AsciiDoc that Markdown reads as something else, or as
// nothing.
var leftovers = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"a cross reference", regexp.MustCompile(`xref:`)},
	{"a link macro", urlMacroPattern},
	{"a section title", regexp.MustCompile(`(?m)^={1,6}\s+\S`)},
	{"a table delimiter", regexp.MustCompile(`(?m)^\|===\s*$`)},
	{"a block fence", regexp.MustCompile(`(?m)^={4,}\s*$`)},
	{"an attribute definition", regexp.MustCompile(`(?m)^:[a-zA-Z0-9_-]+:\s`)},
	{"a block attribute", regexp.MustCompile(`(?m)^\[(NOTE|TIP|WARNING|CAUTION|IMPORTANT|%[^\]]*)\]\s*$`)},
	{"a block title", regexp.MustCompile(`(?m)^\.[A-Z]\S`)},
	{"an inline call-out", regexp.MustCompile(`(?m)^(NOTE|TIP|WARNING|CAUTION|IMPORTANT):\s`)},
	{"an include", regexp.MustCompile(`(?m)^include::`)},
	{"an anchor", regexp.MustCompile(`(?m)^\[\[`)},
	{"an image", regexp.MustCompile(`image::?[^\[\s]*\[`)},
	{"a passthrough", regexp.MustCompile(`\+\+\+|pass:\[`)},
	{"a role span", regexp.MustCompile(`\[\.[^\]]+\]#`)},
	{"a footnote", regexp.MustCompile(`footnote:\[`)},
	{"a keyboard or menu macro", regexp.MustCompile(`\b(kbd|btn|menu):\[`)},
	{"an escaped brace", regexp.MustCompile(`\\\{`)},
}

// TestMarkdownLeavesNoAsciiDoc walks the committed schema and fails when the
// markup of AsciiDoc survives the conversion.
//
// It guards the converter against a construct that the schema starts to use
// later. Such a construct then shows up as a failing test, and not as markup
// in a generated module.
func TestMarkdownLeavesNoAsciiDoc(t *testing.T) {
	schema := loadStandardSchema(t)

	e := &env{opts: &CompileOptions{DocsBaseURL: testDocs}}

	var checked int

	check := func(where, text string) {
		if text == "" {
			return
		}

		checked++

		got := e.docs(text)

		// A fenced block and an indented block keep their text on purpose, so
		// the markup inside one is not a leftover.
		got = withoutCode(got)

		for _, leftover := range leftovers {
			if leftover.pattern.MatchString(got) {
				t.Errorf("%s still holds %s:\n%s", where, leftover.name, got)
				return
			}
		}
	}

	var walk func(*Property)
	walk = func(prop *Property) {
		if prop == nil {
			return
		}

		check("a property", prop.Description)

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
			check("a component summary", component.Summary)
			check("a component description", component.Description)
			walk(component.Config)
		}
	}

	if checked == 0 {
		t.Fatal("the schema gave no text to check")
	}
}

// withoutCode drops the lines that Markdown reads as code, so that a check
// reads the prose alone.
func withoutCode(text string) string {
	var (
		out      []string
		fenced   bool
		inCode   bool
		wasBlank = true
	)

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			fenced = !fenced
			wasBlank = false

			continue
		}

		blank := trimmed == ""

		if !fenced && !blank {
			inCode = isIndented(line) && (wasBlank || inCode)
		}

		wasBlank = blank

		if !fenced && !inCode {
			out = append(out, line)
		}
	}

	return strings.Join(out, "\n")
}
