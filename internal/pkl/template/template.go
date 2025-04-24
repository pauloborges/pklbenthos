package template

import (
	"embed"
	"regexp"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

// FS holds the templates that this package renders from.
//
//go:embed *.tmpl
var FS embed.FS

// Module is the name of the template that renders one Pkl module.
const (
	Module = "module.pkl.tmpl"
)

// wrapWidth is the column that a comment wraps at. It counts the indent and
// the prefix of the comment, not only the text.
const wrapWidth = 90

// Load reads the template that renders one module, with the helpers that wrap
// a comment.
func Load() (*template.Template, error) {
	funcs := sprig.FuncMap()
	funcs["pklDoc"] = pklDoc
	funcs["pklComment"] = pklComment

	return template.New("template").Funcs(funcs).ParseFS(FS, "*.tmpl")
}

// pklDoc turns a description into a Pkl doc comment. Each line gets the `///`
// prefix. The result has no trailing newline, so that the template controls
// the spacing.
func pklDoc(indent, doc string) string {
	return wrapComment(indent, "/// ", doc)
}

// pklComment turns a text into a Pkl comment. It follows the same rules as
// pklDoc.
func pklComment(indent, text string) string {
	return wrapComment(indent, "// ", text)
}

// wrapComment gives each line of the text the indent and the prefix, and
// breaks a line that goes past wrapWidth.
//
// A line inside a fenced code block, and a line that an indent marks as code,
// stays as it is. A break in such a line changes what it says.
func wrapComment(indent, prefix, text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	head := indent + prefix
	limit := wrapWidth - len(head)

	var out []string
	var fenced bool

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, " \t")

		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		}

		switch {
		case line == "":
			// An empty line keeps the prefix, but no trailing space.
			out = append(out, strings.TrimRight(head, " "))

		case fenced || isIndentedCode(line) || isTableRow(line) || len(line) <= limit:
			out = append(out, head+line)

		default:
			out = append(out, wrapLine(head, line, limit)...)
		}
	}

	return strings.Join(out, "\n")
}

// wrapLine breaks one line at its spaces. A word that is longer than the limit
// goes on a line of its own, and that line is longer than the limit.
//
// The lines after the first keep the indent of the line, and a line that
// carries a list marker aligns them under its first word.
func wrapLine(head, line string, limit int) []string {
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	body := line[len(indent):]

	hanging := indent + strings.Repeat(" ", listMarkerWidth(body))

	// A quote holds only the lines that carry its mark, so every line after
	// the first takes the mark again. A hanging indent of spaces would drop
	// the rest of the text out of the quote. The first line keeps its own
	// mark, because the mark is the first word of the body.
	if marker := quoteMarker(body); marker != "" {
		hanging = indent + marker
	}

	var out []string
	current := indent

	for _, word := range strings.Fields(body) {
		switch {
		case strings.TrimSpace(current) == "":
			current += word
		case len(current)+1+len(word) <= limit:
			current += " " + word
		default:
			out = append(out, head+current)
			current = hanging + word
		}
	}

	if strings.TrimSpace(current) != "" {
		out = append(out, head+current)
	}

	return out
}

// listMarker matches the symbol that starts a list item, and the space after
// it. It covers the bullets of Markdown and of AsciiDoc, such as `-`, `*`,
// `**` and `.`, and an ordered item, such as `1.`.
var listMarker = regexp.MustCompile(`^([-*+.]+|[0-9]+[.)])[ \t]+`)

// listMarkerWidth returns how far the text of a list item sits from the start
// of the line, and 0 when the line carries no list marker.
func listMarkerWidth(body string) int {
	return len(listMarker.FindString(body))
}

// isIndentedCode tells if the indent of a line marks it as a code block.
func isIndentedCode(line string) bool {
	return strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
}

// isTableRow tells if a line is one row of a table of Markdown. A break in
// such a line would put half of the row outside the table.
func isTableRow(line string) bool {
	return strings.HasPrefix(strings.TrimLeft(line, " \t"), "|")
}

// quoteMarker returns the mark that opens a quote, with the space after it,
// and an empty string when the line opens none.
func quoteMarker(body string) string {
	if !strings.HasPrefix(body, ">") {
		return ""
	}

	marker := ">"
	for len(marker) < len(body) && body[len(marker)] == ' ' {
		marker += " "
	}

	return marker
}
