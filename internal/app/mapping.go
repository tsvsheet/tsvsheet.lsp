package app

import (
	"strings"
	"unicode/utf16"

	tsvsheet "github.com/tsvsheet/go-tsvsheet"
	"go.lsp.dev/protocol"
)

// The .tsvt grid is a TAB-separated document: each document line is a grid row,
// each TAB-delimited field a grid column. Two subtleties make the position↔cell
// mapping non-trivial and are the reason it is isolated in small, tested
// functions here:
//
//   - The engine (ReadTSV) skips a first-line shebang (`#!…`) and any comment
//     line (`# …`), so grid-row indices do NOT equal document-line indices once
//     comments are present. gridMap records the bijection between the grid rows
//     the engine sees and the document lines the editor addresses.
//   - LSP Position.character is a UTF-16 code-unit offset, so field boundaries
//     are computed over UTF-16 units, not bytes or runes.

// documentText is the full text of an open document.
type documentText string

// lineText is a single document line, its trailing CR (if any) stripped so
// offsets match the content the engine parses.
type lineText string

// cellText is the raw text of one cell — a single TAB-delimited field.
type cellText string

// utf16Offset is a character offset within a line, measured in UTF-16 code
// units — the unit LSP positions use.
type utf16Offset int

// docIndex is a 0-based document-line index — the LSP line number.
type docIndex int

// rowIndex is a 0-based grid row (the engine's row, post comment/shebang skip).
type rowIndex int

// colIndex is a 0-based grid column: the index of a TAB-delimited field.
type colIndex int

// notGrid marks a document line that is not a grid row (a comment, a shebang, or
// a trailing empty line).
const notGrid = -1

// tabUnit is the UTF-16 code unit for a TAB, the field separator.
const tabUnit uint16 = '\t'

// gridMap is the bijection between the editor's document lines and the engine's
// grid rows for one document snapshot.
type gridMap struct {
	lines     []lineText // every document line, in order
	gridOfDoc []int      // document-line index → grid-row index, or notGrid
	docOfGrid []int      // grid-row index → document-line index
}

// buildGridMap splits text into lines and computes the document↔grid bijection,
// mirroring ReadTSV: a first-line shebang and every `# ` comment line are not
// grid rows, and a trailing newline does not add an empty final row.
func buildGridMap(text documentText) gridMap {
	lines := splitLines(text)
	limit := gridLimit(text, docIndex(len(lines)))
	gridOfDoc := make([]int, len(lines))
	docOfGrid := make([]int, 0, len(lines))
	for i, line := range lines {
		index := docIndex(i)
		if index >= limit || isSkipped(index, line) {
			gridOfDoc[i] = notGrid
			continue
		}
		gridOfDoc[i] = len(docOfGrid)
		docOfGrid = append(docOfGrid, i)
	}
	return gridMap{lines: lines, gridOfDoc: gridOfDoc, docOfGrid: docOfGrid}
}

// splitLines splits a document into lines on `\n`, stripping a single trailing
// `\r` per line so field offsets match the engine's CR-free line content.
func splitLines(text documentText) []lineText {
	raw := strings.Split(string(text), "\n")
	out := make([]lineText, len(raw))
	for i, r := range raw {
		out[i] = lineText(strings.TrimSuffix(r, "\r"))
	}
	return out
}

// gridLimit is the number of leading document lines that can be grid rows. A
// document ending in a newline has a trailing empty line the engine's scanner
// does not turn into a row, so it is excluded.
func gridLimit(text documentText, lineCount docIndex) docIndex {
	if strings.HasSuffix(string(text), "\n") {
		return lineCount - 1
	}
	return lineCount
}

// isSkipped reports whether a document line is one the engine drops rather than
// treating as a grid row: a first-line shebang or a `# ` comment line.
func isSkipped(index docIndex, line lineText) bool {
	if index == 0 && strings.HasPrefix(string(line), "#!") {
		return true
	}
	return strings.HasPrefix(string(line), "# ")
}

// gridRow maps a document-line index to its grid row, reporting false when the
// line is out of range or is a skipped (comment/shebang) line.
func (m gridMap) gridRow(line docIndex) (rowIndex, bool) {
	if line < 0 || int(line) >= len(m.gridOfDoc) {
		return 0, false
	}
	row := m.gridOfDoc[int(line)]
	if row == notGrid {
		return 0, false
	}
	return rowIndex(row), true
}

// docLineOfGrid maps a grid row back to its document line, or notGrid when the
// row is out of range.
func (m gridMap) docLineOfGrid(row rowIndex) docIndex {
	if row < 0 || int(row) >= len(m.docOfGrid) {
		return notGrid
	}
	return docIndex(m.docOfGrid[int(row)])
}

// lineAtDoc returns the text of a document line, or empty when out of range.
func (m gridMap) lineAtDoc(line docIndex) lineText {
	if line < 0 || int(line) >= len(m.lines) {
		return ""
	}
	return m.lines[int(line)]
}

// cellAt maps an LSP position to the grid cell under it, reporting false when
// the position is on a non-grid (comment/shebang/out-of-range) line.
func (m gridMap) cellAt(pos protocol.Position) (tsvsheet.Address, bool) {
	line := docIndex(pos.Line)
	row, ok := m.gridRow(line)
	if !ok {
		return tsvsheet.Address{}, false
	}
	col := columnAt(m.lineAtDoc(line), utf16Offset(pos.Character))
	return tsvsheet.Address{Row: int(row), Col: int(col)}, true
}

// rangeOfGridCell is the LSP range covering a grid cell: its document line and
// the UTF-16 span of that column's TAB-delimited field.
func (m gridMap) rangeOfGridCell(row rowIndex, col colIndex) protocol.Range {
	line := m.docLineOfGrid(row)
	start, end := fieldSpan(m.lineAtDoc(line), col)
	return newRange(line, start, end)
}

// newRange builds a single-line LSP range, clamping a missing document line to
// line 0 so a diagnostic is never silently dropped off the document.
func newRange(line docIndex, start, end utf16Offset) protocol.Range {
	at := max(line, 0)
	return protocol.Range{
		Start: protocol.Position{Line: uint32(at), Character: uint32(start)},
		End:   protocol.Position{Line: uint32(at), Character: uint32(end)},
	}
}

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

// fieldStart is the UTF-16 offset at which the col-th field begins, or the line
// length when the line has fewer fields.
func fieldStart(units []uint16, col colIndex) utf16Offset {
	seen := colIndex(0)
	for i, unit := range units {
		if seen == col {
			return utf16Offset(i)
		}
		if unit == tabUnit {
			seen++
		}
	}
	return utf16Offset(len(units))
}

// fieldEnd is the UTF-16 offset at which the field beginning at start ends: the
// next TAB, or the line length.
func fieldEnd(units []uint16, start utf16Offset) utf16Offset {
	for i := int(start); i < len(units); i++ {
		if units[i] == tabUnit {
			return utf16Offset(i)
		}
	}
	return utf16Offset(len(units))
}

// fields splits a line into its TAB-delimited cell values.
func fields(line lineText) []cellText {
	raw := strings.Split(string(line), "\t")
	out := make([]cellText, len(raw))
	for i, field := range raw {
		out[i] = cellText(field)
	}
	return out
}
