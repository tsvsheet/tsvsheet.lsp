package app

import (
	"context"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tsvsheet "github.com/tsvsheet/go-tsvsheet"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// testDocURI is the document every code-action test edits.
var testDocURI = uri.File("/sheets/grades.tsvt")

// at builds a cursor position from a 0-based document line and UTF-16 offset.
func at(line, character uint32) protocol.Position {
	return protocol.Position{Line: line, Character: character}
}

// onlyEdit returns the single text edit an action carries, failing when the
// action does not carry exactly one — an action that edits two places is not
// the one-cell rewrite this feature promises.
func onlyEdit(t *testing.T, action protocol.CodeAction) protocol.TextEdit {
	t.Helper()
	require.NotNil(t, action.Edit)
	edits := action.Edit.Changes[testDocURI]
	require.Len(t, edits, 1, "a fill rewrites exactly one cell")
	return edits[0]
}

// applyEdit splices an edit into a document the way an editor would, so a test
// can assert on the WHOLE resulting document rather than on the replacement
// text alone. That is what catches an edit whose range is right by accident.
func applyEdit(t *testing.T, text documentText, edit protocol.TextEdit) documentText {
	t.Helper()
	lines := strings.Split(string(text), "\n")
	require.Less(t, int(edit.Range.Start.Line), len(lines))
	line := lines[edit.Range.Start.Line]
	units := utf16.Encode([]rune(line))
	require.LessOrEqual(t, int(edit.Range.End.Character), len(units))
	patched := string(utf16.Decode(units[:edit.Range.Start.Character])) +
		edit.NewText +
		string(utf16.Decode(units[edit.Range.End.Character:]))
	lines[edit.Range.Start.Line] = patched
	return documentText(strings.Join(lines, "\n"))
}

// actionTitled finds the offered action with the given title, failing when it
// is absent. Selecting by title rather than by index is what lets a test about
// one direction stay valid when the other direction is also offered.
func actionTitled(t *testing.T, actions []protocol.CodeAction, title string) protocol.CodeAction {
	t.Helper()
	for _, action := range actions {
		if action.Title == title {
			return action
		}
	}
	require.FailNow(t, "action not offered", "%q is not among %v", title, titles(actions))
	return protocol.CodeAction{}
}

// titles lists the offered actions' titles, which is what an editor's menu
// shows and therefore what a test about "is it offered" should assert on.
func titles(actions []protocol.CodeAction) []string {
	out := make([]string, 0, len(actions))
	for _, action := range actions {
		out = append(out, action.Title)
	}
	return out
}

// TestCodeActions_FillDownRebasesAnUnpinnedReference is the feature's whole
// point, and states the R5 semantics at the editor boundary: filling down one
// row shifts an unpinned row coordinate by one, so `=A1*2` becomes `=A2*2`.
func TestCodeActions_FillDownRebasesAnUnpinnedReference(t *testing.T) {
	t.Parallel()

	doc := documentText("10\t=A1*2\n20\t\n")
	actions := codeActions(testDocURI, doc, at(1, 3))

	edit := onlyEdit(t, actionTitled(t, actions, "Fill down from the cell above"))
	assert.Equal(t, "=A2 * 2", edit.NewText, "the row shifted by one; the engine renders canonically")
	assert.Equal(t, documentText("10\t=A1*2\n20\t=A2 * 2\n"), applyEdit(t, doc, edit))
}

// TestCodeActions_FillDownKeepsAPinnedRow pins the correction that the
// originating note got backwards: a downward fill shifts ROWS, and a PINNED row
// does not move. `=A$1*2` filled one row down still reads row 1 — only the
// spelling is canonicalised. The note claimed the pin would be dropped and the
// COLUMN would shift; neither happens.
func TestCodeActions_FillDownKeepsAPinnedRow(t *testing.T) {
	t.Parallel()

	actions := codeActions(testDocURI, documentText("10\t=A$1*2\n20\t\n"), at(1, 3))
	edit := onlyEdit(t, actionTitled(t, actions, "Fill down from the cell above"))
	assert.Equal(t, "=A$1 * 2", edit.NewText, "the pinned row held and the column never moved")
}

// TestCodeActions_FillRightShiftsTheColumn states the other axis: filling right
// shifts an unpinned COLUMN, leaving the row alone.
func TestCodeActions_FillRightShiftsTheColumn(t *testing.T) {
	t.Parallel()

	// The target sits on the first row, so only the left neighbour is a
	// candidate and the assertion cannot be satisfied by a fill-down.
	doc := documentText("5\t=A1\t\n")
	actions := codeActions(testDocURI, doc, at(0, 6))
	edit := onlyEdit(t, actionTitled(t, actions, "Fill right from the cell to the left"))
	assert.Equal(t, "=B1", edit.NewText, "the column shifted by one; the row held")
}

// TestCodeActions_FirstRowOffersNoFillDown states the boundary: the top grid
// row has nothing above it, so there is nothing to fill from.
func TestCodeActions_FirstRowOffersNoFillDown(t *testing.T) {
	t.Parallel()

	actions := codeActions(testDocURI, documentText("10\t=A1*2\n20\t\n"), at(0, 3))
	assert.NotContains(t, titles(actions), "Fill down from the cell above")
}

// TestCodeActions_FirstColumnOffersNoFillRight states the other boundary: the
// leftmost column has no neighbour to its left.
func TestCodeActions_FirstColumnOffersNoFillRight(t *testing.T) {
	t.Parallel()

	actions := codeActions(testDocURI, documentText("=B1\t2\n=B2\t3\n"), at(1, 0))
	assert.NotContains(t, titles(actions), "Fill right from the cell to the left")
}

// TestCodeActions_NonGridLineOffersNothing states that a line the language does
// not treat as a grid row — a shebang, a `#.` directive, a comment — has no
// cell to fill and therefore offers nothing.
func TestCodeActions_NonGridLineOffersNothing(t *testing.T) {
	t.Parallel()

	doc := documentText("#!/usr/bin/env tsv\n#.header\trows(count(1))\n10\t=A1*2\n20\t\n")
	assert.Empty(t, codeActions(testDocURI, doc, at(0, 2)), "a shebang has no cell")
	assert.Empty(t, codeActions(testDocURI, doc, at(1, 2)), "a directive line has no cell")
}

// TestCodeActions_EmptyNeighbourOffersNothing states the rule that keeps a fill
// from being a disguised deletion: filling from an empty neighbour would CLEAR
// the target, so it is never offered.
func TestCodeActions_EmptyNeighbourOffersNothing(t *testing.T) {
	t.Parallel()

	actions := codeActions(testDocURI, documentText("10\t\n20\t=A2*2\n"), at(1, 3))
	assert.NotContains(t, titles(actions), "Fill down from the cell above",
		"filling from an empty cell would delete the target's contents")
}

// TestCodeActions_UnparseableDocumentOffersNothing states that a fill needs a
// sheet the engine could read: rebasing references over a document that does
// not parse would be guesswork.
func TestCodeActions_UnparseableDocumentOffersNothing(t *testing.T) {
	t.Parallel()

	assert.Empty(t, codeActions(testDocURI, documentText("1\t=sum(\n2\t=sum(\n"), at(1, 3)))
}

// TestCodeActions_LiteralNeighbourCopiesVerbatim states R5's other half: a
// literal has no references to rebase, so it copies unchanged.
func TestCodeActions_LiteralNeighbourCopiesVerbatim(t *testing.T) {
	t.Parallel()

	actions := codeActions(testDocURI, documentText("widget\t1\nx\t2\n"), at(1, 0))
	edit := onlyEdit(t, actionTitled(t, actions, "Fill down from the cell above"))
	assert.Equal(t, "widget", edit.NewText)
}

// TestCodeActions_EditLeavesEveryOtherByteAlone states the property the format
// exists for: the edit replaces one field, so every other byte of the document
// — including the neighbouring columns on the same line — survives untouched.
func TestCodeActions_EditLeavesEveryOtherByteAlone(t *testing.T) {
	t.Parallel()

	// The left neighbour is empty, so fill-down is the only candidate and the
	// single edit under test is unambiguous.
	doc := documentText("a\t10\t=B1*2\tkeep\nb\t\t\tkeep too\n")
	actions := codeActions(testDocURI, doc, at(1, 3))

	edit := onlyEdit(t, actionTitled(t, actions, "Fill down from the cell above"))
	assert.Equal(t, uint32(1), edit.Range.Start.Line, "the edit stays on the target's line")
	assert.Equal(t, uint32(1), edit.Range.End.Line)
	assert.Equal(t, documentText("a\t10\t=B1*2\tkeep\nb\t\t=B2 * 2\tkeep too\n"), applyEdit(t, doc, edit))
}

// TestCodeActions_OutsideTheGridOffersNothing states that a position past the
// last grid row has no cell, so nothing is offered rather than an action
// anchored at a row that does not exist.
func TestCodeActions_OutsideTheGridOffersNothing(t *testing.T) {
	t.Parallel()

	assert.Empty(t, codeActions(testDocURI, documentText("10\t=A1*2\n"), at(9, 0)))
}

// TestCodeActionUnknownDocument states that a code-action request for a
// document the server never opened is answered with nothing rather than an
// error: the editor asking about a closed document is ordinary, not a fault.
func TestCodeActionUnknownDocument(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	actions, err := newServer(discardLogger()).CodeAction(context.Background(), &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: testURI},
	})
	must.NoError(err)
	want.Empty(actions)
}

// TestCodeActionOffersFillOnAnOpenDocument drives the server method end to end:
// an opened document answers with the fill action, carried in the protocol's
// Command|CodeAction union the LSP client actually receives.
func TestCodeActionOffersFillOnAnOpenDocument(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	s := newServer(discardLogger())
	s.docs.set(testURI, documentText("10\t=A1*2\n20\t\n"))

	actions, err := s.CodeAction(context.Background(), &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: testURI},
		Range:        protocol.Range{Start: protocol.Position{Line: 1, Character: 3}},
	})
	must.NoError(err)
	must.NotEmpty(actions)

	action, ok := actions[0].(*protocol.CodeAction)
	must.True(ok, "the union carries a code action, not a command")
	want.Equal("Fill down from the cell above", action.Title)
}

// TestCapabilitiesAdvertisesCodeActions states the handshake half of the
// feature: a client that is never told the server provides code actions will
// never ask for one, so the capability is as load-bearing as the handler.
func TestCapabilitiesAdvertisesCodeActions(t *testing.T) {
	t.Parallel()

	assert.Equal(t, protocol.Boolean(true), capabilities().Capabilities.CodeActionProvider)
}

// TestCodeActions_RaggedGridOffersNothingPastAShortRow states what happens when
// the row above simply has fewer columns: there is no neighbour at that column,
// so nothing is offered rather than an action filling from a cell that is not
// there. TSV rows are independent lines, so ragged grids are ordinary.
func TestCodeActions_RaggedGridOffersNothingPastAShortRow(t *testing.T) {
	t.Parallel()

	actions := codeActions(testDocURI, documentText("10\n20\t=A2*2\n"), at(1, 3))
	assert.NotContains(t, titles(actions), "Fill down from the cell above",
		"the row above has no cell in this column")
}

// TestCellSourceOffTheGridIsEmpty states cellSource's contract at each edge of
// the grid it indexes. The helper answers empty rather than panicking, which is
// what lets every caller treat "no neighbour" and "empty neighbour" alike.
func TestCellSourceOffTheGridIsEmpty(t *testing.T) {
	t.Parallel()

	sheet, err := tsvsheet.Parse([]byte("a\tb\nc\td\n"))
	require.NoError(t, err)

	assert.Equal(t, cellText("a"), cellSource(sheet, tsvsheet.Address{Row: 0, Col: 0}), "an on-grid cell reads back")
	assert.Empty(t, cellSource(sheet, tsvsheet.Address{Row: -1, Col: 0}), "above the first row")
	assert.Empty(t, cellSource(sheet, tsvsheet.Address{Row: 9, Col: 0}), "past the last row")
	assert.Empty(t, cellSource(sheet, tsvsheet.Address{Row: 0, Col: -1}), "left of the first column")
	assert.Empty(t, cellSource(sheet, tsvsheet.Address{Row: 0, Col: 9}), "past the last column")
}

// TestCodeActions_FillDropsTheMetaClause pins the 022×023 seam: filling from
// a named cell yields the rebased expression WITHOUT the clause — the 023
// ruling that a copy takes the expression and never the cell's identity,
// arriving in the editor through the engine with no special handling here.
func TestCodeActions_FillDropsTheMetaClause(t *testing.T) {
	t.Parallel()

	doc := documentText("=A2*2 |@ named(Double)\t5\n\t7\n")
	actions := codeActions(testDocURI, doc, at(1, 0))
	edit := onlyEdit(t, actionTitled(t, actions, "Fill down from the cell above"))
	assert.Equal(t, "=A3 * 2", edit.NewText, "the reference rebased; the name stayed with its cell")
}
