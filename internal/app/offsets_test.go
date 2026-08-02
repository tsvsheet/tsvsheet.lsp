package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Offsets inside one line, measured in the UTF-16 code units LSP speaks.

func TestColumnAt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		line   lineText
		offset utf16Offset
		want   colIndex
	}{
		{name: "start of line", line: "a\tb\tc", offset: 0, want: 0},
		{name: "inside first field", line: "a\tb\tc", offset: 1, want: 0},
		{name: "just after first tab", line: "a\tb\tc", offset: 2, want: 1},
		{name: "third field", line: "a\tbb\tc", offset: 5, want: 2},
		{name: "past end clamps", line: "a\tb", offset: 99, want: 1},
		{name: "no tabs", line: "abc", offset: 2, want: 0},
		{name: "multibyte before tab counts one column", line: "café\tx", offset: 6, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.New(t).Equal(tt.want, columnAt(tt.line, tt.offset))
		})
	}
}

func TestFieldSpan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		line      lineText
		col       colIndex
		wantStart utf16Offset
		wantEnd   utf16Offset
	}{
		{name: "first field", line: "a\tbb\tc", col: 0, wantStart: 0, wantEnd: 1},
		{name: "middle field", line: "a\tbb\tc", col: 1, wantStart: 2, wantEnd: 4},
		{name: "last field to end", line: "a\tbb\tc", col: 2, wantStart: 5, wantEnd: 6},
		{name: "empty middle field", line: "a\t\tc", col: 1, wantStart: 2, wantEnd: 2},
		{name: "column past last is zero width at end", line: "a\tb", col: 5, wantStart: 3, wantEnd: 3},
		{name: "single field whole line", line: "hello", col: 0, wantStart: 0, wantEnd: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := assert.New(t)
			start, end := fieldSpan(tt.line, tt.col)
			want.Equal(tt.wantStart, start)
			want.Equal(tt.wantEnd, end)
		})
	}
}
