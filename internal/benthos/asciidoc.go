package benthos

import (
	"strings"
)

// The schema writes its documentation in AsciiDoc, because Antora builds the
// documentation site of the build from it. A Pkl doc comment holds Markdown.
// This file turns the one into the other.
//
// It covers the constructs that the schema of Redpanda Connect uses, and no
// more. TestMarkdownLeavesNoAsciiDoc walks the committed schema and fails when
// a construct that this file does not know reaches a doc comment, so a new one
// shows up as a failing test and not as markup in a module.

// admonitions are the labels that AsciiDoc gives to a call-out. Each one
// stands on its own line as "NOTE: text", or opens a block under "[NOTE]".
var admonitions = map[string]string{
	"NOTE":      "Note",
	"TIP":       "Tip",
	"WARNING":   "Warning",
	"CAUTION":   "Caution",
	"IMPORTANT": "Important",
}

// toMarkdown converts the documentation of a schema from AsciiDoc to Markdown.
//
// The base is the root of the documentation site, and an empty base leaves a
// cross reference as it is. See resolveLinks.
func toMarkdown(text, base string) string {
	c := &converter{base: base}
	c.run(strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n"))

	return strings.Join(c.out, "\n")
}

type converter struct {
	base string
	out  []string

	// attribute holds a block attribute, such as "[NOTE]" or
	// "[%header,format=dsv]", until the block that it describes arrives.
	attribute string

	// wasBlank tells if the line before was empty. Markdown starts a code
	// block at an indented line only after a blank line.
	wasBlank bool
}

func (c *converter) emit(lines ...string) {
	c.out = append(c.out, lines...)
}

func (c *converter) run(lines []string) {
	c.wasBlank = true

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		switch {
		// A fenced block keeps every line, because its text is code.
		case strings.HasPrefix(trimmed, "```"):
			i = c.copyFence(lines, i)

		// An indented line after a blank line is code, and keeps its text.
		case c.wasBlank && trimmed != "" && isIndented(line):
			i = c.copyIndented(lines, i)

		case trimmed == "|===":
			i = c.table(lines, i)

		case isBlockAttribute(trimmed):
			// The attribute describes the block that comes next, and says
			// nothing of its own. An admonition starts right below it.
			c.attribute = trimmed

			if end, ok := c.admonitionBlock(lines, i); ok {
				i = end
				break
			}

			c.wasBlank = false

			continue

		case isBlockFence(trimmed):
			// A row of equals signs opens or closes a block. Only an
			// admonition uses one here, and admonitionBlock takes both rows of
			// it, so a row that reaches this point has lost its attribute.
			// Drop it: the text of the block stays, and the rows say nothing.
			c.wasBlank = false

			continue

		case strings.HasPrefix(trimmed, "="):
			if heading, ok := asHeading(trimmed); ok {
				c.emit(heading)
			} else {
				c.emit(c.inline(line))
			}

		case isAttributeDefinition(trimmed):
			// An attribute definition tells Antora a value for its own page
			// template. It says nothing to a reader, and no text of the schema
			// reads one back.
			c.wasBlank = false

			continue

		case isBlockTitle(trimmed):
			c.emit("**" + c.inline(strings.TrimPrefix(trimmed, ".")) + "**")

		default:
			if label, body, ok := inlineAdmonition(trimmed); ok {
				c.emit("> **" + label + ":** " + c.inline(body))
			} else {
				c.emit(c.inline(line))
			}
		}

		c.wasBlank = strings.TrimSpace(c.last()) == ""
	}
}

func (c *converter) last() string {
	if len(c.out) == 0 {
		return ""
	}

	return c.out[len(c.out)-1]
}

// inline converts what sits inside a line, and leaves the rest of it alone.
func (c *converter) inline(line string) string {
	return unescapeBraces(resolveLinks(line, c.base))
}

// unescapeBraces drops the backslash that AsciiDoc needs in front of an
// opening brace.
//
// AsciiDoc reads "{name}" as an attribute, so an author who wants the braces
// themselves writes "\{name}". Markdown has no such attribute, and shows the
// backslash instead, as in "/channels/\{channel_id}/messages". Every brace in
// the schema of Redpanda Connect stands for a value that a reader fills in,
// such as a path parameter, a JSON example, or a pattern of grok. None is an
// attribute, so the backslash has no work left to do.
func unescapeBraces(line string) string {
	return strings.ReplaceAll(line, `\{`, "{")
}

// takeAttribute returns the block attribute that waits for a block, and clears
// it.
func (c *converter) takeAttribute() string {
	attribute := c.attribute
	c.attribute = ""

	return attribute
}

// copyFence copies a fenced block, and returns the index of its last line.
func (c *converter) copyFence(lines []string, start int) int {
	c.emit(lines[start])

	for i := start + 1; i < len(lines); i++ {
		c.emit(lines[i])

		if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
			return i
		}
	}

	return len(lines) - 1
}

// copyIndented copies a block of indented code, and returns the index of its
// last line.
func (c *converter) copyIndented(lines []string, start int) int {
	last := start

	for i := start; i < len(lines); i++ {
		line := lines[i]

		if strings.TrimSpace(line) == "" {
			// A blank line ends the block unless an indented line follows.
			if i+1 < len(lines) && isIndented(lines[i+1]) {
				c.emit(line)
				continue
			}

			return last
		}

		if !isIndented(line) {
			return last
		}

		c.emit(line)

		last = i
	}

	return last
}

// isBlockAttribute tells if a line is a block attribute, such as "[NOTE]" or
// "[%header,format=dsv]".
func isBlockAttribute(line string) bool {
	if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
		return false
	}

	body := line[1 : len(line)-1]
	if body == "" || strings.ContainsAny(body, "[]") {
		return false
	}

	// A Markdown link starts with a bracket as well, and carries a URL after
	// it. A block attribute stands alone on its line, so the check above
	// already keeps the two apart. Keep out anything with a space, which reads
	// as prose.
	return !strings.Contains(body, " ")
}

// isAttributeDefinition tells if a line gives a value to an attribute of
// AsciiDoc, as in ":driver-support: mysql=certified".
func isAttributeDefinition(line string) bool {
	if !strings.HasPrefix(line, ":") {
		return false
	}

	name, _, found := strings.Cut(line[1:], ":")

	return found && name != "" && !strings.ContainsAny(name, " \t")
}

// isBlockTitle tells if a line gives a title to the block below it, as in
// ".Endpoint caveats".
func isBlockTitle(line string) bool {
	if !strings.HasPrefix(line, ".") || len(line) < 2 {
		return false
	}

	// A title starts with a letter or a digit. An ellipsis, and a line of
	// dots, do not.
	head := rune(line[1])

	return head >= 'A' && head <= 'Z'
}

// asHeading turns a section title of AsciiDoc into one of Markdown.
//
// Antora gives the title of a page one equals sign, and the schema holds the
// body of a page, so its highest title carries two. Two therefore becomes the
// second level of Markdown, and the levels below it follow.
func asHeading(line string) (string, bool) {
	depth := 0
	for depth < len(line) && line[depth] == '=' {
		depth++
	}

	if depth == 0 || depth > 6 || depth >= len(line) || line[depth] != ' ' {
		return "", false
	}

	title := strings.TrimSpace(line[depth:])
	if title == "" {
		return "", false
	}

	return strings.Repeat("#", depth) + " " + title, true
}

// inlineAdmonition reads a call-out that stands on one line, as in
// "NOTE: the text".
func inlineAdmonition(line string) (label, body string, ok bool) {
	name, rest, found := strings.Cut(line, ": ")
	if !found {
		return "", "", false
	}

	label, ok = admonitions[name]
	if !ok {
		return "", "", false
	}

	return label, strings.TrimSpace(rest), true
}
