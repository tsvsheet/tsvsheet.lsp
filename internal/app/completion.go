package app

import (
	"strings"

	tsvsheet "github.com/tsvsheet/go-tsvsheet"
	"go.lsp.dev/protocol"
)

// Completion for the two identifier vocabularies a formula speaks: function
// names from the engine's own enumerable catalog (tsvsheet.Functions — the
// registry the evaluator dispatches on, so the offer cannot drift from what
// computes), and the sheet's declared value names after an `@` sigil
// (tsvsheet.Sheet.Names, SPECIFICATION §5.6). Only formula cells complete:
// a literal cell's text is data, and offering syntax inside data would teach
// the marker rule backwards.

// completions answers a completion request at a position: `@`-names when the
// cursor follows a sigil, function names otherwise, nothing outside a formula
// cell or in an unreadable document.
func completions(text documentText, pos protocol.Position) []protocol.CompletionItem {
	grid := buildGridMap(text)
	if !inFormulaCell(grid, pos) {
		return nil
	}
	if afterSigil(grid, pos) {
		return nameItems(text, pos)
	}
	return functionItems()
}

// inFormulaCell reports whether the position sits in a cell whose text begins
// with the formula marker.
func inFormulaCell(grid gridMap, pos protocol.Position) bool {
	addr, ok := grid.cellAt(pos)
	if !ok {
		return false
	}
	// cellAt's column is the count of TABs before the cursor on this line, so
	// it always indexes within the line's own fields.
	line := grid.lineAtDoc(docIndex(pos.Line))
	return strings.HasPrefix(string(fields(line)[addr.Col]), "=")
}

// afterSigil reports whether the character immediately before the cursor is
// the `@` sigil (or the cursor sits inside a `@name` token), which selects the
// name vocabulary over the function one.
func afterSigil(grid gridMap, pos protocol.Position) bool {
	line := []rune(string(grid.lineAtDoc(docIndex(pos.Line))))
	at := int(pos.Character)
	if at > len(line) {
		at = len(line)
	}
	for i := at - 1; i >= 0; i-- {
		switch {
		case line[i] == '@':
			return true
		case isNameRune(nameRune(line[i])):
			continue
		default:
			return false
		}
	}
	return false
}

// nameRune is one character of a candidate `@name` token under the cursor.
type nameRune rune

// isNameRune reports whether the rune may appear in a name after its first
// character (the §5.6 charset).
func isNameRune(r nameRune) bool {
	return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// nameItems offers the sheet's declared value names, in binding order, each in
// its declared spelling. The cell being edited is blanked before the parse:
// at the very moment `@`-completion matters — right after the sigil is typed —
// that cell is a transient syntax error (`=@` alone does not parse), and the
// names live in OTHER cells — the edited one declares nothing for its own
// completion (TestCompletions_TheEditedCellsOwnErrorDoesNotBlock). A document
// unreadable beyond the edited cell offers nothing
// (TestCompletions_UnreadableDocumentOffersNoNames).
func nameItems(text documentText, pos protocol.Position) []protocol.CompletionItem {
	sheet, err := tsvsheet.ParseWith([]byte(blankFieldAt(text, pos)), tsvsheet.DefaultLimits())
	if err != nil {
		return nil
	}
	names := sheet.Names()
	items := make([]protocol.CompletionItem, len(names))
	for i, name := range names {
		items[i] = protocol.CompletionItem{
			Label:  name,
			Kind:   protocol.CompletionItemKindVariable,
			Detail: protocol.NewOptional("named value (SPECIFICATION §5.6)"),
		}
	}
	return items
}

// blankFieldAt returns the document with the TAB-separated field under pos
// emptied, every other byte — and so every other cell's declarations — left
// as written. The caller (completions) has already resolved pos to a grid
// cell, so the line and field indices hold by that check.
func blankFieldAt(text documentText, pos protocol.Position) documentText {
	grid := buildGridMap(text)
	addr, _ := grid.cellAt(pos)
	lines := strings.Split(string(text), "\n")
	cells := strings.Split(lines[pos.Line], "\t")
	cells[addr.Col] = ""
	lines[pos.Line] = strings.Join(cells, "\t")
	return documentText(strings.Join(lines, "\n"))
}

// functionItems offers every callable builtin from the engine's catalog. The
// meta function `named` is absent by the catalog's own contract — it is not
// callable, and offering it would teach the misplacement Check refuses.
func functionItems() []protocol.CompletionItem {
	fns := tsvsheet.Functions()
	items := make([]protocol.CompletionItem, len(fns))
	for i, fn := range fns {
		items[i] = protocol.CompletionItem{
			Label: string(fn),
			Kind:  protocol.CompletionItemKindFunction,
		}
	}
	return items
}
