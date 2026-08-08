package app

import (
	tsvsheet "github.com/tsvsheet/go-tsvsheet"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// fillTitle is the label an editor shows for one fill action.
type fillTitle string

// rowOffset and colOffset are a neighbour's displacement from the target cell,
// in grid rows and columns. They are named rather than bare ints so a
// direction's two axes stay distinguishable at every call site.
type (
	rowOffset int
	colOffset int
)

// fillDirection is one neighbour a cell can be filled from: the label to show
// and where that neighbour sits relative to the target.
type fillDirection struct {
	title fillTitle
	row   rowOffset
	col   colOffset
}

// fillDirections are the neighbours offered, matching the two directions a
// spreadsheet binds to Ctrl+D and Ctrl+R. `below` and `right` are deliberately
// absent: filling FROM a cell the reader has not reached yet inverts the
// document's top-to-bottom reading order, and work order 022 defers it.
var fillDirections = []fillDirection{
	{title: "Fill down from the cell above", row: -1},
	{title: "Fill right from the cell to the left", col: -1},
}

// codeActions offers the fill actions available at a position. A position off
// the grid — past the end, or on a shebang, comment or `#.` directive line —
// offers nothing, and so does a document that does not parse: a fill rebases
// references, which is meaningless over a sheet the engine could not read.
func codeActions(docURI uri.URI, text documentText, pos protocol.Position) []protocol.CodeAction {
	grid := buildGridMap(text)
	target, onGrid := grid.cellAt(pos)
	if !onGrid {
		return nil
	}
	doc, err := tsvsheet.ParseDocument([]byte(text))
	if err != nil {
		return nil
	}
	actions := make([]protocol.CodeAction, 0, len(fillDirections))
	for _, direction := range fillDirections {
		if action, ok := fillAction(docURI, grid, doc, target, direction); ok {
			actions = append(actions, action)
		}
	}
	return actions
}

// fillAction builds one direction's action, reporting false when that fill has
// nothing to offer: no neighbour on that side, or an EMPTY neighbour — filling
// from an empty cell clears the target, which is a deletion wearing a fill's
// label — TestCodeActions_EmptyNeighbourOffersNothing states that it is not
// offered.
//
// Beyond those two, the action is offered whether or not the resulting text
// differs, as a spreadsheet's own fill is. Suppressing an apparent no-op was
// tried and removed: the engine re-renders a filled formula in canonical form
// (`=B1*2` returns as `=B2 * 2`, [R10]), so a text comparison answers
// "different" even for a pure reformat — it suppressed almost nothing while
// implying a semantic check that was not happening.
func fillAction(
	docURI uri.URI,
	grid gridMap,
	doc tsvsheet.Document,
	target tsvsheet.Address,
	direction fillDirection,
) (protocol.CodeAction, bool) {
	source := tsvsheet.Address{Row: target.Row + int(direction.row), Col: target.Col + int(direction.col)}
	if source.Row < 0 || source.Col < 0 || cellSource(doc.Sheet(), source) == "" {
		return protocol.CodeAction{}, false
	}
	filled := cellSource(doc.Fill(source, tsvsheet.Span{From: target, To: target}).Sheet(), target)
	at := grid.rangeOfGridCell(rowIndex(target.Row), colIndex(target.Col))
	return rewriteAction(docURI, direction.title, at, filled), true
}

// rewriteAction wraps one cell's replacement as a refactor the editor can
// apply. The edit spans the target field alone, so undo is one step and the
// diff is one column — the property the whole format exists for, stated by
// TestCodeActions_EditLeavesEveryOtherByteAlone.
func rewriteAction(docURI uri.URI, title fillTitle, at protocol.Range, text cellText) protocol.CodeAction {
	kind := protocol.CodeActionKindRefactorRewrite
	return protocol.CodeAction{
		Title: string(title),
		Kind:  &kind,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[uri.URI][]protocol.TextEdit{
				docURI: {{Range: at, NewText: string(text)}},
			},
		},
	}
}

// cellSource is a cell's SOURCE text — the `=formula` an author wrote, not its
// computed value — or empty when the address is off the grid. The fill is a
// source-to-source rewrite, so the computed side does not come into it.
func cellSource(sheet tsvsheet.Sheet, at tsvsheet.Address) cellText {
	grid := sheet.Source()
	if at.Row < 0 || at.Row >= len(grid) {
		return ""
	}
	row := grid[at.Row]
	if at.Col < 0 || at.Col >= len(row) {
		return ""
	}
	return cellText(row[at.Col])
}
