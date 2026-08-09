// Offsets within a single line.
//
// mapping.go answers "which grid cell is this document position?" — a question
// about the document's shape. This file answers the question one level down:
// where a field starts and ends inside one line, counted in the UTF-16 code
// units LSP positions are measured in. That unit is the whole reason these are
// separate: an editor counts UTF-16, Go counts bytes, and a cell holding an
// emoji makes the two disagree.
package app

import "unicode/utf16"

// columnAt returns the grid column at a UTF-16 offset: the number of TABs before
// that offset on the line. An offset past the line's end clamps to its end.
func columnAt(line lineText, offset utf16Offset) colIndex {
	units := utf16.Encode([]rune(string(line)))
	limit := min(max(int(offset), 0), len(units))
	tabs := 0
	for _, unit := range units[:limit] {
		if unit == tabUnit {
			tabs++
		}
	}
	return colIndex(tabs)
}

// fieldSpan returns the [start, end) UTF-16 offsets of the col-th TAB-delimited
// field on a line. A column past the last field yields a zero-width span at the
// line's end.
func fieldSpan(line lineText, col colIndex) (utf16Offset, utf16Offset) {
	units := utf16.Encode([]rune(string(line)))
	start := fieldStart(units, col)
	return start, fieldEnd(units, start)
}
