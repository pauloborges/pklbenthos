package benthos

import "testing"

const testDocs = "https://docs.redpanda.com/redpanda-connect"

func TestResolveLinks(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "a plain reference",
			text: "See the xref:components:processors/branch.adoc[`branch` processor].",
			want: "See the [`branch` processor](" + testDocs + "/components/processors/branch).",
		},
		{
			name: "a reference with an anchor",
			text: "Read xref:configuration:batching.adoc#batch-policy[batching].",
			want: "Read [batching](" + testDocs + "/configuration/batching#batch-policy).",
		},
		{
			// Antora gives an empty text the title of the page, which the
			// schema does not carry. The path of the page stands in for it.
			name: "an empty text takes the path of the page",
			text: "See xref:guides:cloud/aws.adoc[].",
			want: "See [cloud/aws](" + testDocs + "/guides/cloud/aws).",
		},
		{
			name: "two references in one line",
			text: "xref:guides:bloblang/about.adoc[Bloblang] and xref:configuration:interpolation.adoc[interpolation]",
			want: "[Bloblang](" + testDocs + "/guides/bloblang/about) and " +
				"[interpolation](" + testDocs + "/configuration/interpolation)",
		},
		{
			name: "a text that carries a space wraps into the link",
			text: "A xref:guides:bloblang/about.adoc[Bloblang mapping] runs here.",
			want: "A [Bloblang mapping](" + testDocs + "/guides/bloblang/about) runs here.",
		},
		{
			name: "a base with a trailing slash gives one slash",
			text: "xref:guides:sync_responses.adoc[responses]",
			want: "[responses](" + testDocs + "/guides/sync_responses)",
		},
		{
			// The schema does not say which module holds such a page, and the
			// one that Redpanda Connect writes points at a page that the
			// documentation site does not hold. Keep the text, drop the
			// markup.
			name: "a reference with no module keeps its text alone",
			text: "See the xref:outputs/bigquery_cdc_migration.adoc[CDC migration guide].",
			want: "See the CDC migration guide.",
		},
		{
			name: "a link macro becomes a Markdown link",
			text: "Uses a http://jmespath.org/[JMESPath query] here.",
			want: "Uses a [JMESPath query](http://jmespath.org/) here.",
		},
		{
			// A caret asks AsciiDoc to open the link in a new window.
			name: "a caret at the end of the text goes",
			text: "See https://docs.snowflake.com/en/guide.html[supported^] stage types.",
			want: "See [supported](https://docs.snowflake.com/en/guide.html) stage types.",
		},
		{
			// The schema also writes the caret at the front of the text.
			name: "a caret at the front of the text goes",
			text: "Read https://api.slack.com/apis/socket-mode[^Socket Mode].",
			want: "Read [Socket Mode](https://api.slack.com/apis/socket-mode).",
		},
		{
			name: "a link macro with no text keeps the URL alone",
			text: "See https://example.com/page[] for more.",
			want: "See https://example.com/page for more.",
		},
		{
			name: "a bare URL stays as it is",
			text: "Read https://example.com/page for more.",
			want: "Read https://example.com/page for more.",
		},
		{
			name: "a cross reference and a link macro on one line",
			text: "xref:guides:bloblang/about.adoc[Bloblang] and http://jmespath.org/[JMESPath]",
			want: "[Bloblang](" + testDocs + "/guides/bloblang/about) and " +
				"[JMESPath](http://jmespath.org/)",
		},
		{
			name: "text with no reference stays as it is",
			text: "This description holds no cross reference.",
			want: "This description holds no cross reference.",
		},
		{
			// A Markdown link of the schema of Benthos must survive, because
			// it is not a cross reference.
			name: "a Markdown link stays as it is",
			text: "See [the docs](/docs/guides/bloblang/about).",
			want: "See [the docs](/docs/guides/bloblang/about).",
		},
		{
			// A Markdown link that already holds an absolute URL must not
			// take a second pass, because its URL ends at a bracket of its
			// own.
			name: "a Markdown link with an absolute URL stays as it is",
			text: "See [the docs](https://example.com/page).",
			want: "See [the docs](https://example.com/page).",
		},
		{
			// Markdown reads an indented line as code when a blank line comes
			// before it, and shows a link there as the characters that make
			// it. A rewrite would only change which characters a reader sees.
			name: "an indented line after a blank line keeps its markup",
			text: "Prose here.\n\n\thttps://example.com/page[text^] stays.",
			want: "Prose here.\n\n\thttps://example.com/page[text^] stays.",
		},
		{
			// An indented line that carries on the paragraph above is prose,
			// and Markdown reads it as such. The schema of Redpanda Connect
			// writes one, for the stage types of Snowflake.
			name: "an indented line that carries on a paragraph is prose",
			text: "Use one of the\n\t\thttps://example.com/stage[supported^] stage types.",
			want: "Use one of the\n\t\t[supported](https://example.com/stage) stage types.",
		},
		{
			// A bracket that carries a backslash does not end the text of a
			// macro. A pattern that stopped there would put the URL in the
			// middle of the text that is left.
			name: "an escaped bracket does not end the text",
			text: "| https://example.com/dsn[`http[s\\]://user[:pass\\]@host`^]",
			want: "| [`http[s\\]://user[:pass\\]@host`](https://example.com/dsn)",
		},
		{
			name: "a fenced block keeps its markup",
			text: "Prose.\n\n```\nhttps://example.com/page[text^]\nxref:guides:a.adoc[b]\n```",
			want: "Prose.\n\n```\nhttps://example.com/page[text^]\nxref:guides:a.adoc[b]\n```",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := testDocs
			if test.name == "a base with a trailing slash gives one slash" {
				base = testDocs + "/"
			}

			if got := toMarkdown(test.text, base); got != test.want {
				t.Errorf("toMarkdown()\n got: %s\nwant: %s", got, test.want)
			}
		})
	}
}

// TestResolveXrefsWithoutBase covers a build whose documentation this package
// does not know. The text then has to survive untouched, because a link to
// nowhere is worse than a reference that a reader can look up.
func TestResolveXrefsWithoutBase(t *testing.T) {
	text := "See xref:guides:bloblang/about.adoc[Bloblang]."

	if got := toMarkdown(text, ""); got != text {
		t.Errorf("toMarkdown() with no base changed the text:\n%s", got)
	}
}

// TestResolveLinksLeavesNoneBehind walks the committed schema and fails when
// the markup of AsciiDoc survives. It guards the patterns against a form that
// the schema starts to use later.
func TestResolveLinksLeavesNoneBehind(t *testing.T) {
	schema := loadStandardSchema(t)

	e := &env{opts: &CompileOptions{DocsBaseURL: testDocs}}

	var checked int

	check := func(where, text string) {
		if text == "" {
			return
		}

		checked++

		got := e.docs(text)

		if containsXref(got) {
			t.Errorf("%s still holds a cross reference:\n%s", where, got)
		}

		if urlMacroPattern.MatchString(got) {
			t.Errorf("%s still holds a link macro:\n%s", where, got)
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

func containsXref(text string) bool {
	return xrefPattern.MatchString(text) || contains(text, "xref:")
}

func contains(text, sub string) bool {
	for i := 0; i+len(sub) <= len(text); i++ {
		if text[i:i+len(sub)] == sub {
			return true
		}
	}

	return false
}
