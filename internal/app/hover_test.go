package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tsvsheet "github.com/tsvsheet/go-tsvsheet"
	"go.lsp.dev/protocol"
)

func hoverValue(t *testing.T, h *protocol.Hover) string {
	t.Helper()
	require.New(t).NotNil(h)
	content, ok := h.Contents.(*protocol.MarkupContent)
	require.New(t).True(ok)
	return content.Value
}

func TestHoverFormulaCell(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	// B2 = =SUM(A1:B1); cursor on the second field of line 1.
	h := hover("1\t2\n=A1\t=SUM(A1:B1)\n", protocol.Position{Line: 1, Character: 4})
	value := hoverValue(t, h)
	want.Contains(value, "**B2** = `3`")
	want.Contains(value, "Formula: `=SUM(A1:B1)`")
	want.Contains(value, "Inputs:")
	want.Contains(value, "`A1:B1`")
	want.Equal(protocol.MarkupKindMarkdown, h.Contents.(*protocol.MarkupContent).Kind)
	want.NotNil(h.Range)
	want.Equal(uint32(1), h.Range.Start.Line)
}

func TestHoverLiteralCell(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	// A literal cell has a value but no formula and no inputs.
	value := hoverValue(t, hover("42\t7\n", protocol.Position{Line: 0, Character: 0}))
	want.Equal("**A1** = `42`", value)
}

func TestHoverReturnsNil(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text documentText
		pos  protocol.Position
	}{
		{name: "position on comment line", text: "# note\n=A2\n", pos: protocol.Position{Line: 0, Character: 1}},
		{name: "position past last line", text: "a\tb\n", pos: protocol.Position{Line: 1, Character: 0}},
		{name: "document does not parse", text: "=1+\n", pos: protocol.Position{Line: 0, Character: 0}},
		{name: "empty cell", text: "=A2\t\n\t\n", pos: protocol.Position{Line: 0, Character: 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.New(t).Nil(hover(tt.text, tt.pos))
		})
	}
}

func TestIsEmptyTrace(t *testing.T) {
	t.Parallel()
	want := assert.New(t)
	want.True(isEmptyTrace(tsvsheet.Trace{}))
	want.False(isEmptyTrace(tsvsheet.Trace{Value: "5"}))
	want.False(isEmptyTrace(tsvsheet.Trace{Formula: "A1"}))
}

func TestHoverMarkdown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		want  string
		trace tsvsheet.Trace
	}{
		{
			name:  "literal",
			trace: tsvsheet.Trace{Cell: "A1", Value: "42"},
			want:  "**A1** = `42`",
		},
		{
			name:  "formula without inputs",
			trace: tsvsheet.Trace{Cell: "B1", Value: "0", Formula: "NOW()"},
			want:  "**B1** = `0`\n\nFormula: `=NOW()`",
		},
		{
			name: "formula with inputs",
			trace: tsvsheet.Trace{
				Cell: "C1", Value: "5", Formula: "A1+B1",
				Inputs: []tsvsheet.TraceInput{{Ref: "A1", Value: "2"}, {Ref: "B1", Value: "3"}},
			},
			want: "**C1** = `5`\n\nFormula: `=A1+B1`\n\nInputs:\n- `A1` = `2`\n- `B1` = `3`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.New(t).Equal(tt.want, hoverMarkdown(tt.trace))
		})
	}
}
