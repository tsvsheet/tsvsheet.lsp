package app

import (
	"errors"

	tsvsheet "github.com/tsvsheet/go-tsvsheet"
	"go.lsp.dev/protocol"
)

const (
	// diagnosticSource labels every diagnostic this server emits.
	diagnosticSource = "tsvsheet"
	// syntaxMessage prefixes a formula syntax-error diagnostic, followed by the
	// offending cell's text.
	syntaxMessage = "syntax error in formula: "
	// readMessage prefixes a whole-document read-failure diagnostic (a row past
	// the engine's size bound), followed by the engine's error text.
	readMessage = "cannot read sheet: "
)

// fatalFlag is a diagnostic's fatality: fatal findings are errors, advisory
// findings are warnings.
type fatalFlag bool

// diagnose computes the LSP diagnostics for a document. A syntax error routes to
// per-cell pinpointing; any other parse failure — a row past the engine's size
// bound (ErrReadInput), or a document past the resident budget (ErrDocTooLarge:
// the load is bounded, spec 018, closing the gap where this was the one
// ecosystem load path that materialized unbounded) — is a whole-document
// diagnostic rather than silently dropped by the per-cell fallback, which would
// find nothing because each cell parses on its own. A clean parse yields the
// engine's Check findings mapped to their cells.
func diagnose(text documentText) []protocol.Diagnostic {
	grid := buildGridMap(text)
	sheet, err := tsvsheet.ParseWith([]byte(text), tsvsheet.DefaultLimits())
	switch {
	case errors.Is(err, tsvsheet.ErrSyntax):
		return grid.syntaxDiagnostics()
	case err != nil:
		return []protocol.Diagnostic{readErrorDiagnostic(err)}
	default:
		return grid.checkDiagnostics(sheet)
	}
}

// readErrorDiagnostic reports a whole-document read failure that is not a
// per-cell syntax error, anchored at the document origin because the failure
// belongs to the sheet as a whole rather than any single cell.
func readErrorDiagnostic(err error) protocol.Diagnostic {
	return protocol.Diagnostic{
		Range:    protocol.Range{},
		Severity: protocol.DiagnosticSeverityError,
		Source:   protocol.NewOptional(diagnosticSource),
		Message:  protocol.String(readMessage + err.Error()),
	}
}

// syntaxDiagnostics pinpoints every grid cell whose formula fails to compile by
// parsing each cell in isolation — independent of the others, exactly as the
// engine compiles them — so all offending cells are reported, each at its own
// range rather than only the first the whole-sheet parse rejected.
func (m gridMap) syntaxDiagnostics() []protocol.Diagnostic {
	out := []protocol.Diagnostic{}
	for grid, line := range m.docOfGrid {
		out = m.appendLineSyntax(out, rowIndex(grid), docIndex(line))
	}
	return out
}

// appendLineSyntax appends a diagnostic for each cell on one grid row whose
// formula has a syntax error.
func (m gridMap) appendLineSyntax(
	out []protocol.Diagnostic,
	row rowIndex,
	line docIndex,
) []protocol.Diagnostic {
	for col, field := range fields(m.lineAtDoc(line)) {
		if isFormulaSyntaxError(field) {
			out = append(out, syntaxDiagnostic(m.rangeOfGridCell(row, colIndex(col)), field))
		}
	}
	return out
}

// isFormulaSyntaxError reports whether a single cell's text is a formula the
// engine rejects with a syntax error. A literal, an empty cell, or a valid
// formula returns false.
func isFormulaSyntaxError(field cellText) bool {
	_, err := tsvsheet.Parse([]byte(field))
	return errors.Is(err, tsvsheet.ErrSyntax)
}

// syntaxDiagnostic builds an error-severity diagnostic naming the offending
// formula text.
func syntaxDiagnostic(rng protocol.Range, field cellText) protocol.Diagnostic {
	return protocol.Diagnostic{
		Range:    rng,
		Severity: protocol.DiagnosticSeverityError,
		Source:   protocol.NewOptional(diagnosticSource),
		Message:  protocol.String(syntaxMessage + string(field)),
	}
}

// checkDiagnostics maps each engine Check finding to an LSP diagnostic at its
// cell.
func (m gridMap) checkDiagnostics(sheet tsvsheet.Sheet) []protocol.Diagnostic {
	findings := tsvsheet.Check(sheet)
	out := make([]protocol.Diagnostic, 0, len(findings))
	for _, finding := range findings {
		out = append(out, m.checkDiagnostic(finding))
	}
	return out
}

// checkDiagnostic maps one Check finding to a diagnostic. Check guarantees a
// valid A1 cell, so the address always parses; a contract violation degrades to
// the zero address (cell A1) rather than dropping the finding.
func (m gridMap) checkDiagnostic(finding tsvsheet.Diagnostic) protocol.Diagnostic {
	addr, _ := tsvsheet.ParseAddress(tsvsheet.AddressText(finding.Cell))
	return protocol.Diagnostic{
		Range:    m.rangeOfGridCell(rowIndex(addr.Row), colIndex(addr.Col)),
		Severity: severityOf(fatalFlag(finding.IsFatal)),
		Source:   protocol.NewOptional(diagnosticSource),
		Message:  protocol.String(finding.Message),
	}
}

// severityOf maps a finding's fatality to an LSP severity: fatal → Error,
// advisory → Warning.
func severityOf(isFatal fatalFlag) protocol.DiagnosticSeverity {
	if isFatal {
		return protocol.DiagnosticSeverityError
	}
	return protocol.DiagnosticSeverityWarning
}
