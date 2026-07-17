package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tsvsheet "github.com/tsvsheet/go-tsvsheet"
	"go.lsp.dev/protocol"
)

func TestDiagnoseSyntaxError(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	// B2 (row 1, col 1) holds a formula the engine rejects.
	diags := diagnose("a\tb\n1\t=1+\n")
	must.Len(diags, 1)

	d := diags[0]
	want.Equal(protocol.DiagnosticSeverityError, d.Severity)
	want.Equal(protocol.NewOptional(diagnosticSource), d.Source)
	want.Equal(protocol.String("syntax error in formula: =1+"), d.Message)
	want.Equal(protocol.Range{
		Start: protocol.Position{Line: 1, Character: 2},
		End:   protocol.Position{Line: 1, Character: 5},
	}, d.Range)
}

func TestDiagnoseMultipleSyntaxErrors(t *testing.T) {
	t.Parallel()
	// Every offending cell is reported, not only the first the whole-sheet
	// parse would reject.
	diags := diagnose("=1+\t=*\n")
	assert.New(t).Len(diags, 2)
}

func TestDiagnoseReadError(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	// A single row past the engine's 1 MiB scan bound fails to parse with
	// ErrReadInput — a whole-document read error, not a per-cell syntax error.
	// It must surface as one visible diagnostic, never be silently dropped by
	// the per-cell fallback (each field parses fine on its own).
	oversized := documentText(strings.Repeat("x", (1<<20)+1) + "\n")
	diags := diagnose(oversized)
	must.Len(diags, 1)

	d := diags[0]
	want.Equal(protocol.DiagnosticSeverityError, d.Severity)
	want.Equal(protocol.NewOptional(diagnosticSource), d.Source)
	msg, ok := d.Message.(protocol.String)
	must.True(ok)
	want.True(strings.HasPrefix(string(msg), readMessage))
	want.Equal(protocol.Range{}, d.Range)
}

func TestDiagnoseCheckFinding(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	// A comment line shifts the grid; the unknown function lands on document
	// line 1, column 1.
	diags := diagnose("# header\nx\t=BADFUNC(1)\n")
	must.Len(diags, 1)

	d := diags[0]
	want.Equal(protocol.DiagnosticSeverityWarning, d.Severity)
	want.Equal(protocol.String("unknown function: BADFUNC"), d.Message)
	want.Equal(uint32(1), d.Range.Start.Line)
	want.Equal(uint32(2), d.Range.Start.Character)
}

func TestDiagnoseCleanSheet(t *testing.T) {
	t.Parallel()
	diags := diagnose("1\t2\n=A1\t=B1\n")
	assert.New(t).Empty(diags)
}

func TestIsFormulaSyntaxError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		field cellText
		want  bool
	}{
		{name: "bad formula", field: "=1+", want: true},
		{name: "valid formula", field: "=SUM(A1:A2)", want: false},
		{name: "literal", field: "hello", want: false},
		{name: "empty", field: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.New(t).Equal(tt.want, isFormulaSyntaxError(tt.field))
		})
	}
}

func TestSeverityOf(t *testing.T) {
	t.Parallel()
	want := assert.New(t)
	want.Equal(protocol.DiagnosticSeverityError, severityOf(fatalFlag(true)))
	want.Equal(protocol.DiagnosticSeverityWarning, severityOf(fatalFlag(false)))
}

func TestCheckDiagnosticInvalidCellDegradesToOrigin(t *testing.T) {
	t.Parallel()
	// Check guarantees a valid A1 cell; a hypothetical malformed cell must not
	// drop the finding — it degrades to the document origin.
	m := buildGridMap("x\ty")
	d := m.checkDiagnostic(tsvsheet.Diagnostic{Cell: "not-a-cell", Message: "boom", IsFatal: true})
	want := assert.New(t)
	want.Equal(protocol.String("boom"), d.Message)
	want.Equal(protocol.DiagnosticSeverityError, d.Severity)
	want.Equal(uint32(0), d.Range.Start.Line)
}
