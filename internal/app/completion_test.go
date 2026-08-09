package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lsp.dev/protocol"
)

// labels lists the offered items' labels — what an editor's menu shows.
func labels(items []protocol.CompletionItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Label)
	}
	return out
}

// TestCompletions_FunctionsInAFormula states the default vocabulary: inside a
// formula cell the engine's whole callable catalog is offered — sourced from
// tsvsheet.Functions, so the offer cannot drift from what evaluates — and the
// uncallable meta function `named` is deliberately absent.
func TestCompletions_FunctionsInAFormula(t *testing.T) {
	t.Parallel()

	items := completions("1\t=su\n", at(0, 5))
	names := labels(items)
	assert.Contains(t, names, "sum")
	assert.Contains(t, names, "textjoin")
	assert.NotContains(t, names, "named", "the meta function is not callable")
	for _, item := range items {
		assert.Equal(t, protocol.CompletionItemKindFunction, item.Kind)
	}
}

// TestCompletions_NamesAfterTheSigil states the `@` vocabulary: after the
// sigil the sheet's declared value names are offered, in binding order and
// declared spelling, as variables.
func TestCompletions_NamesAfterTheSigil(t *testing.T) {
	t.Parallel()

	doc := documentText("=0.08 |@ named(Rate)\t=1 |@ named(total_q1)\t=@\n")
	items := completions(doc, at(0, 45))
	require.Equal(t, []string{"Rate", "total_q1"}, labels(items))
	for _, item := range items {
		assert.Equal(t, protocol.CompletionItemKindVariable, item.Kind)
	}
}

// TestCompletions_MidNameKeepsTheNameVocabulary states that the sigil selects
// the vocabulary even mid-word: a cursor inside `@Ra` still completes names.
func TestCompletions_MidNameKeepsTheNameVocabulary(t *testing.T) {
	t.Parallel()

	doc := documentText("=0.08 |@ named(Rate)\t=@Ra\n")
	assert.Equal(t, []string{"Rate"}, labels(completions(doc, at(0, 25))))
}

// TestCompletions_LiteralCellOffersNothing states the boundary the marker
// rule draws: a literal cell's text is data, and offering formula syntax
// inside data would teach the `=` rule backwards.
func TestCompletions_LiteralCellOffersNothing(t *testing.T) {
	t.Parallel()

	assert.Empty(t, completions("plain text\t=1\n", at(0, 5)))
}

// TestCompletions_NonGridLineOffersNothing states that comment and directive
// lines have no cells and complete nothing.
func TestCompletions_NonGridLineOffersNothing(t *testing.T) {
	t.Parallel()

	assert.Empty(t, completions("#.header\trows(count(1))\n=1\n", at(0, 4)))
}

// TestCompletions_UnreadableDocumentOffersNoNames states the name vocabulary
// needs a sheet the engine could read BEYOND the cell being edited: a syntax
// error in another cell offers nothing, while the edited cell's own transient
// brokenness never blocks its completion (the test below).
func TestCompletions_UnreadableDocumentOffersNoNames(t *testing.T) {
	t.Parallel()

	assert.Empty(t, completions("=sum(\t=@\n", at(0, 8)))
}

// TestCompletions_TheEditedCellsOwnErrorDoesNotBlock states the moment the
// feature exists for: the author has just typed `=@` — a transient syntax
// error in that cell — and the OTHER cells' declarations still complete,
// because the edited cell is blanked before the parse.
func TestCompletions_TheEditedCellsOwnErrorDoesNotBlock(t *testing.T) {
	t.Parallel()

	doc := documentText("=0.08 |@ named(Rate)\t=@\n")
	assert.Equal(t, []string{"Rate"}, labels(completions(doc, at(0, 23))))
}

// TestServerCompletion drives the handler end to end: an open document
// answers a CompletionList; an unopened one answers nothing.
func TestServerCompletion(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	s := newServer(discardLogger())
	s.docs.set(testURI, "=0.08 |@ named(Rate)\t=@\n")

	result, err := s.Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: testURI},
			Position:     protocol.Position{Line: 0, Character: 23},
		},
	})
	must.NoError(err)
	list, ok := result.(*protocol.CompletionList)
	must.True(ok)
	want.Equal([]string{"Rate"}, labels(list.Items))

	none, err := newServer(discardLogger()).Completion(context.Background(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: testURI},
		},
	})
	must.NoError(err)
	want.Nil(none)
}

// TestCapabilitiesAdvertisesCompletion states the handshake half: the sigil
// is a trigger character, so typing `@` opens the name vocabulary unprompted.
func TestCapabilitiesAdvertisesCompletion(t *testing.T) {
	t.Parallel()

	provider := capabilities().Capabilities.CompletionProvider
	require.NotNil(t, provider)
	assert.Equal(t, []string{"@"}, provider.TriggerCharacters)
}

// TestCompletions_CursorPastTheLineEndClamps states the clamp: a client may
// report a column past the line's end (a virtual-space cursor); the sigil scan
// clamps to the line and still finds the vocabulary.
func TestCompletions_CursorPastTheLineEndClamps(t *testing.T) {
	t.Parallel()

	doc := documentText("=0.08 |@ named(Rate)\t=@\n")
	assert.Equal(t, []string{"Rate"}, labels(completions(doc, at(0, 99))))
}

// TestAfterSigil_LineWithoutASigilExhaustsTheScan pins the scan's own
// boundary: a line of name runes with no sigil anywhere scans to the line
// start and answers false — the function-vocabulary default.
func TestAfterSigil_LineWithoutASigilExhaustsTheScan(t *testing.T) {
	t.Parallel()

	grid := buildGridMap("abc\n")
	assert.False(t, afterSigil(grid, at(0, 3)))
}
