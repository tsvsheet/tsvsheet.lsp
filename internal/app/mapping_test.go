package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	tsvsheet "github.com/tsvsheet/go-tsvsheet"
	"go.lsp.dev/protocol"
)

func TestSplitLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text documentText
		want []lineText
	}{
		{name: "single line no newline", text: "a\tb", want: []lineText{"a\tb"}},
		{name: "trailing newline yields empty last line", text: "a\n", want: []lineText{"a", ""}},
		{name: "interior empty line preserved", text: "a\n\nb", want: []lineText{"a", "", "b"}},
		{name: "empty document", text: "", want: []lineText{""}},
		{name: "strips trailing carriage return", text: "a\tb\r\nc", want: []lineText{"a\tb", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.New(t).Equal(tt.want, splitLines(tt.text))
		})
	}
}

func TestGridLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		text      documentText
		lineCount docIndex
		want      docIndex
	}{
		{name: "no trailing newline keeps all", text: "a\nb", lineCount: 2, want: 2},
		{name: "trailing newline drops last", text: "a\nb\n", lineCount: 3, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.New(t).Equal(tt.want, gridLimit(tt.text, tt.lineCount))
		})
	}
}

func TestIsSkipped(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		line  lineText
		index docIndex
		want  bool
	}{
		{name: "shebang on first line", index: 0, line: "#!/usr/bin/env tsvsheet", want: true},
		{name: "shebang not on first line is a cell", index: 1, line: "#!x", want: false},
		{name: "legacy hash-space comment", index: 3, line: "# a note", want: true},
		{name: "directive marker anywhere", index: 3, line: "#.hide-cols\tB-M", want: true},
		{name: "directive marker on first line", index: 0, line: "#.header-rows\t1", want: true},
		{name: "prose after the directive marker", index: 2, line: "#.some comment", want: true},
		{name: "hash without space is a cell", index: 0, line: "#foo", want: false},
		{name: "hash before a TAB is a cell", index: 2, line: "#\tnot a comment", want: false},
		{name: "error value is a cell", index: 2, line: "#N/A\t=A2", want: false},
		{name: "ordinary line", index: 0, line: "a\tb", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.New(t).Equal(tt.want, isSkipped(tt.index, tt.line))
		})
	}
}

func TestBuildGridMap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		text          documentText
		wantGridOfDoc []int
		wantDocOfGrid []int
	}{
		{
			name:          "plain grid",
			text:          "a\tb\nc\td",
			wantGridOfDoc: []int{0, 1},
			wantDocOfGrid: []int{0, 1},
		},
		{
			name:          "comment shifts grid rows",
			text:          "# header\na\tb",
			wantGridOfDoc: []int{-1, 0},
			wantDocOfGrid: []int{1},
		},
		{
			name:          "shebang then comment then trailing newline",
			text:          "#!tsvsheet\n# note\nx\ty\n",
			wantGridOfDoc: []int{-1, -1, 0, -1},
			wantDocOfGrid: []int{2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := assert.New(t)
			m := buildGridMap(tt.text)
			want.Equal(tt.wantGridOfDoc, m.gridOfDoc)
			want.Equal(tt.wantDocOfGrid, m.docOfGrid)
		})
	}
}

func TestGridRow(t *testing.T) {
	t.Parallel()
	m := buildGridMap("# header\na\tb")

	tests := []struct {
		name    string
		docLine docIndex
		wantRow rowIndex
		wantOK  bool
	}{
		{name: "comment line has no grid row", docLine: 0, wantRow: 0, wantOK: false},
		{name: "grid line maps to row 0", docLine: 1, wantRow: 0, wantOK: true},
		{name: "negative line", docLine: -1, wantRow: 0, wantOK: false},
		{name: "line past end", docLine: 99, wantRow: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := assert.New(t)
			row, ok := m.gridRow(tt.docLine)
			want.Equal(tt.wantOK, ok)
			want.Equal(tt.wantRow, row)
		})
	}
}

func TestDocLineOfGrid(t *testing.T) {
	t.Parallel()
	m := buildGridMap("# header\na\tb")
	want := assert.New(t)
	want.Equal(docIndex(1), m.docLineOfGrid(0))
	want.Equal(docIndex(notGrid), m.docLineOfGrid(-1))
	want.Equal(docIndex(notGrid), m.docLineOfGrid(5))
}

func TestLineAtDoc(t *testing.T) {
	t.Parallel()
	m := buildGridMap("a\tb\nc")
	want := assert.New(t)
	want.Equal(lineText("a\tb"), m.lineAtDoc(0))
	want.Equal(lineText(""), m.lineAtDoc(-1))
	want.Equal(lineText(""), m.lineAtDoc(9))
}

func TestCellAt(t *testing.T) {
	t.Parallel()
	m := buildGridMap("# note\na\tbb\tc\n")

	tests := []struct {
		name     string
		pos      protocol.Position
		wantAddr tsvsheet.Address
		wantOK   bool
	}{
		{name: "comment line has no cell", pos: protocol.Position{Line: 0, Character: 0}, wantOK: false},
		{
			name:     "first field",
			pos:      protocol.Position{Line: 1, Character: 0},
			wantAddr: tsvsheet.Address{Row: 0, Col: 0},
			wantOK:   true,
		},
		{
			name:     "second field",
			pos:      protocol.Position{Line: 1, Character: 3},
			wantAddr: tsvsheet.Address{Row: 0, Col: 1},
			wantOK:   true,
		},
		{
			name:     "third field",
			pos:      protocol.Position{Line: 1, Character: 6},
			wantAddr: tsvsheet.Address{Row: 0, Col: 2},
			wantOK:   true,
		},
		{
			name:     "cursor past end clamps to last field",
			pos:      protocol.Position{Line: 1, Character: 99},
			wantAddr: tsvsheet.Address{Row: 0, Col: 2},
			wantOK:   true,
		},
		{name: "trailing empty line has no cell", pos: protocol.Position{Line: 2, Character: 0}, wantOK: false},
		{name: "line past end", pos: protocol.Position{Line: 9, Character: 0}, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := assert.New(t)
			addr, ok := m.cellAt(tt.pos)
			want.Equal(tt.wantOK, ok)
			want.Equal(tt.wantAddr, addr)
		})
	}
}

func TestRangeOfGridCell(t *testing.T) {
	t.Parallel()
	m := buildGridMap("# note\na\tbb\tc")
	want := assert.New(t)

	// grid row 0 is document line 1; column 1 is the "bb" field.
	got := m.rangeOfGridCell(0, 1)
	want.Equal(protocol.Range{
		Start: protocol.Position{Line: 1, Character: 2},
		End:   protocol.Position{Line: 1, Character: 4},
	}, got)

	// an out-of-range grid row clamps to line 0 with a zero-width span rather
	// than dropping the diagnostic off the document.
	clamped := m.rangeOfGridCell(99, 0)
	want.Equal(protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End:   protocol.Position{Line: 0, Character: 0},
	}, clamped)
}

func TestFields(t *testing.T) {
	t.Parallel()
	assert.New(t).Equal([]cellText{"a", "bb", ""}, fields("a\tbb\t"))
}

// TestNewRangeClampsAMissingLineRatherThanDroppingTheDiagnostic pins the
// fallback. A diagnostic anchored past the end of the document is one the
// editor cannot show, so the author never learns about the problem — a silent
// drop is strictly worse than a diagnostic on the wrong line.
func TestNewRangeClampsAMissingLineRatherThanDroppingTheDiagnostic(t *testing.T) {
	t.Parallel()
	// A cell the document map cannot place reports a negative line; anchoring a
	// diagnostic there would put it outside the document, where no editor shows
	// it, so it clamps to the origin instead.
	missing := newRange(-1, 0, 3)
	present := newRange(2, 0, 3)

	assert.Equal(t, uint32(0), missing.Start.Line, "an unplaceable line clamps to the document origin")
	assert.Equal(t, missing.Start.Line, missing.End.Line, "and stays a single-line range")
	assert.Equal(t, uint32(2), present.Start.Line, "while a real line is left where it is")
}
