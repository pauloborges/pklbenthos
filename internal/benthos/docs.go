package benthos

import (
	"regexp"
	"strings"
)

// The schema writes its documentation in AsciiDoc, because Antora builds the
// documentation site of the build from it. A Pkl doc comment holds Markdown,
// and a reader of a module has no Antora, so the two link forms of AsciiDoc
// need a translation.

// xrefPattern matches a cross reference, which points at another page of the
// documentation site:
//
//	xref:MODULE:PAGE.adoc[TEXT]
//	xref:MODULE:PAGE.adoc#ANCHOR[TEXT]
//	xref:PAGE.adoc[TEXT]
//
// The site holds the page of a module at BASE/MODULE/PAGE, with no extension.
//
// The last form names no module. Antora reads such a reference against the
// module of the page that holds it, which the schema does not say. See
// resolveXrefs for what this package does with one.
var xrefPattern = regexp.MustCompile(`xref:(?:([a-zA-Z0-9_-]+):)?([^\[\]#:\s]+?)\.adoc(#[^\[\]\s]*)?\[` + macroText + `\]`)

// macroText matches the text of a macro of AsciiDoc, which runs to the first
// closing bracket that carries no backslash.
//
// The schema writes a bracket of its own inside such a text, as in the DSN
// template of the `trino` driver:
//
//	https://example.com/dsn[`http[s\]://user[:pass\]@host`^]
//
// A pattern that stopped at the first bracket would cut the text in half, and
// put the URL in the middle of what is left. Markdown reads a backslash the
// same way, so the text goes through as it is.
const macroText = `((?:\\.|[^\]\\])*)`

// urlMacroPattern matches a link macro, which points at a page of another
// site:
//
//	https://example.com/page[TEXT]
//
// The URL runs up to the bracket, because AsciiDoc ends it there.
var urlMacroPattern = regexp.MustCompile(`(https?://[^\s\[\]]+)\[` + macroText + `\]`)

// resolveLinks turns the link markup of one line into Markdown.
//
// An empty base leaves a cross reference as it is, which suits a build whose
// documentation this package does not know. A link macro carries its own URL,
// so it needs no base and always resolves.
//
// The caller decides which lines to send here. [toMarkdown] keeps back a line
// that Markdown reads as code, because a link there renders as the characters
// that make it, and a rewrite would only change which characters a reader
// sees.
//
// This runs before a comment goes through the wrapper. The wrapper breaks a
// line at its spaces, and markup that carries a space in its text would then
// no longer match.
func resolveLinks(line, base string) string {
	line = resolveXrefs(line, base)
	line = resolveURLMacros(line)

	return line
}

// resolveXrefs turns each cross reference of one line into a Markdown link.
func resolveXrefs(line, base string) string {
	if base == "" || !strings.Contains(line, "xref:") {
		return line
	}

	base = strings.TrimSuffix(base, "/")

	return xrefPattern.ReplaceAllStringFunc(line, func(match string) string {
		fields := xrefPattern.FindStringSubmatch(match)
		if fields == nil {
			return match
		}

		module, page, anchor, label := fields[1], fields[2], fields[3], fields[4]

		// Antora gives an empty text the title of the page, which the schema
		// does not carry. The path of the page says as much, and says it
		// without a guess.
		if strings.TrimSpace(label) == "" {
			label = page
		}

		// A reference that names no module needs the module of the page that
		// holds it, and the schema does not say what that is. Keep the text
		// and drop the markup: a reader then reads a plain sentence, and
		// follows no link that leads nowhere.
		//
		// The one such reference in the schema of Redpanda Connect 4.103
		// points at a page that the documentation site does not hold, under
		// any module. A guess would give a dead link.
		if module == "" {
			return label
		}

		return "[" + label + "](" + base + "/" + module + "/" + page + anchor + ")"
	})
}

// resolveURLMacros turns each link macro of one line into a Markdown link.
func resolveURLMacros(line string) string {
	if !strings.Contains(line, "://") {
		return line
	}

	return urlMacroPattern.ReplaceAllStringFunc(line, func(match string) string {
		fields := urlMacroPattern.FindStringSubmatch(match)
		if fields == nil {
			return match
		}

		url, label := fields[1], cleanMacroLabel(fields[2])

		// AsciiDoc shows the URL itself when the macro carries no text.
		if label == "" {
			return url
		}

		return "[" + label + "](" + url + ")"
	})
}

// cleanMacroLabel takes the text of a link macro and drops the marks that
// belong to AsciiDoc.
//
// A caret asks AsciiDoc to open the link in a new window. It belongs at the
// end of the text, and the schema also writes it at the front, so both go.
func cleanMacroLabel(label string) string {
	label = strings.TrimSpace(label)
	label = strings.TrimSuffix(label, "^")
	label = strings.TrimPrefix(label, "^")

	return strings.TrimSpace(label)
}

// isIndented tells if a line carries the indent that starts a code block of
// Markdown. See resolveLinks for the blank line that has to come first.
func isIndented(line string) bool {
	return strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
}
