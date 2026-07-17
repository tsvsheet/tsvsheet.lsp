package app

import (
	tsvsheet "github.com/tsvsheet/go-tsvsheet"
	"go.lsp.dev/protocol"
)

// hover produces the hover for the cell under an LSP position: the cell's
// computed value, its formula (if any), and its resolved inputs, as Markdown.
// It returns nil — no hover — when the position is not over a grid cell, the
// document does not parse, the cell is out of the grid, or the cell is empty.
func hover(text documentText, pos protocol.Position) *protocol.Hover {
	grid := buildGridMap(text)
	addr, ok := grid.cellAt(pos)
	if !ok {
		return nil
	}
	sheet, err := tsvsheet.Parse([]byte(text))
	if err != nil {
		return nil
	}
	// cellAt only yields coordinates within the parsed grid, so Explain never
	// reports "not found" here; were it to, it returns the zero Trace, which
	// isEmptyTrace treats as a blank cell — no hover — so the error needs no
	// separate branch.
	trace, _ := tsvsheet.Explain(sheet, addr)
	if isEmptyTrace(trace) {
		return nil
	}
	rng := grid.rangeOfGridCell(rowIndex(addr.Row), colIndex(addr.Col))
	return &protocol.Hover{
		Contents: &protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: hoverMarkdown(trace)},
		Range:    &rng,
	}
}

// isEmptyTrace reports whether a trace describes a blank cell — no value and no
// formula — for which no hover is shown.
func isEmptyTrace(trace tsvsheet.Trace) bool {
	return trace.Value == "" && trace.Formula == ""
}

// hoverMarkdown renders a trace as hover Markdown: the cell and its value, then
// the formula and inputs when the cell is a formula.
func hoverMarkdown(trace tsvsheet.Trace) string {
	markdown := "**" + trace.Cell + "** = `" + trace.Value + "`"
	if trace.Formula != "" {
		markdown += "\n\nFormula: `=" + trace.Formula + "`"
	}
	return markdown + inputsMarkdown(trace.Inputs)
}

// inputsMarkdown renders a formula's resolved inputs as a Markdown list, or the
// empty string when there are none.
func inputsMarkdown(inputs []tsvsheet.TraceInput) string {
	if len(inputs) == 0 {
		return ""
	}
	markdown := "\n\nInputs:"
	for _, input := range inputs {
		markdown += "\n- `" + input.Ref + "` = `" + input.Value + "`"
	}
	return markdown
}
