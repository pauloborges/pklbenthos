package benthos

import (
	"strings"
)

// table converts a table of AsciiDoc into one of Markdown, and returns the
// index of the closing delimiter.
//
// The schema writes a table in two forms. Both open and close with "|===".
//
// The first gives every cell of the header on one line, and then each cell of
// the body on a line of its own:
//
//	|===
//	| Driver | Data Source Name Format
//
//	| `mysql`
//	| `[username[:password]@]/dbname`
//	|===
//
// The second names a separator in its block attribute, and gives a whole row
// on each line:
//
//	[%header,format=dsv]
//	|===
//	Bloblang type:Iceberg type
//	string:string
//	|===
func (c *converter) table(lines []string, start int) int {
	attribute := c.takeAttribute()

	end := start + 1
	for end < len(lines) && strings.TrimSpace(lines[end]) != "|===" {
		end++
	}

	body := lines[start+1 : min(end, len(lines))]

	var rows [][]string

	if separator, ok := dsvSeparator(attribute); ok {
		rows = dsvRows(body, separator)
	} else {
		rows = pipeRows(body)
	}

	if len(rows) == 0 {
		// A table that this code cannot read keeps its lines, so that no text
		// of the schema goes missing.
		for _, line := range lines[start:min(end+1, len(lines))] {
			c.emit(c.inline(line))
		}

		return end
	}

	c.emitTable(rows)

	return end
}

// emitTable writes the rows as a table of Markdown. The first row is the
// header, because both forms of the schema carry one.
func (c *converter) emitTable(rows [][]string) {
	width := 0
	for _, row := range rows {
		width = max(width, len(row))
	}

	line := func(cells []string) string {
		out := make([]string, width)

		for i := range out {
			if i < len(cells) {
				// A cell of Markdown ends at a bar, so a bar inside one needs
				// a backslash.
				out[i] = strings.ReplaceAll(c.inline(strings.TrimSpace(cells[i])), "|", `\|`)
			}
		}

		return "| " + strings.Join(out, " | ") + " |"
	}

	// A table of Markdown needs a blank line above it, or the paragraph before
	// swallows it.
	if strings.TrimSpace(c.last()) != "" {
		c.emit("")
	}

	c.emit(line(rows[0]))

	separator := make([]string, width)
	for i := range separator {
		separator[i] = "---"
	}

	c.emit("| " + strings.Join(separator, " | ") + " |")

	for _, row := range rows[1:] {
		c.emit(line(row))
	}

	c.emit("")
}

// dsvSeparator reads the separator of a table whose block attribute names one,
// as in "[%header,format=dsv]".
func dsvSeparator(attribute string) (string, bool) {
	if !strings.Contains(attribute, "format=dsv") {
		return "", false
	}

	return ":", true
}

// dsvRows reads a table that holds a whole row on each line.
func dsvRows(body []string, separator string) [][]string {
	var (
		rows  [][]string
		width int
	)

	for _, line := range body {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// The header sets the width. A cell of a later row may hold the
		// separator itself, so a row splits into no more cells than the header
		// has.
		var cells []string

		if width == 0 {
			cells = strings.Split(line, separator)
			width = len(cells)
		} else {
			cells = strings.SplitN(line, separator, width)
		}

		rows = append(rows, cells)
	}

	return rows
}

// pipeRows reads a table that gives the header on one line and then one cell
// on each line.
func pipeRows(body []string) [][]string {
	var (
		header []string
		cells  []string
	)

	for _, line := range body {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if !strings.HasPrefix(trimmed, "|") {
			// A line that carries on the cell above it belongs to that cell.
			if len(cells) > 0 {
				cells[len(cells)-1] += " " + trimmed
				continue
			}

			return nil
		}

		parts := strings.Split(trimmed, "|")[1:]

		if header == nil {
			// Every cell of the header sits on the first line.
			for _, part := range parts {
				header = append(header, strings.TrimSpace(part))
			}

			continue
		}

		for _, part := range parts {
			cells = append(cells, strings.TrimSpace(part))
		}
	}

	if len(header) == 0 {
		return nil
	}

	rows := [][]string{header}

	for i := 0; i < len(cells); i += len(header) {
		rows = append(rows, cells[i:min(i+len(header), len(cells))])
	}

	return rows
}

// admonitionBlock converts a call-out that spans several lines:
//
//	[CAUTION]
//	====
//	the text
//	====
//
// It returns the index of the closing row of equals signs, and false when the
// attribute opens no such block.
func (c *converter) admonitionBlock(lines []string, start int) (int, bool) {
	label, ok := admonitions[strings.Trim(c.attribute, "[]")]
	if !ok {
		return start, false
	}

	if start+1 >= len(lines) || !isBlockFence(lines[start+1]) {
		return start, false
	}

	c.takeAttribute()

	end := start + 2
	for end < len(lines) && !isBlockFence(lines[end]) {
		end++
	}

	if strings.TrimSpace(c.last()) != "" {
		c.emit("")
	}

	c.emit("> **"+label+"**", ">")

	for _, line := range lines[start+2 : min(end, len(lines))] {
		if strings.TrimSpace(line) == "" {
			c.emit(">")
			continue
		}

		c.emit("> " + c.inline(line))
	}

	c.emit("")

	return end, true
}

// isBlockFence tells if a line is the row of equals signs that opens or closes
// a block of AsciiDoc.
func isBlockFence(line string) bool {
	trimmed := strings.TrimSpace(line)

	return len(trimmed) >= 4 && trimmed == strings.Repeat("=", len(trimmed))
}
