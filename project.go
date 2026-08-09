// Package tsvsheetlsp is the module root for tsvsheet-lsp — the Language Server
// Protocol server for the .tsvt single-file spreadsheet. The runnable entry
// point is cmd/tsvsheet-lsp; all logic lives under internal/ (internal/app for
// the server, handlers, and position↔cell mapping; internal/constants for the
// error sentinels).
//
// The server speaks LSP over stdio (JSON-RPC) and consumes the go-tsvsheet
// engine as its single source of semantics: it advertises full text-document
// sync and hover, parses each open document with the engine, and republishes
// diagnostics (formula syntax errors and Check findings) plus hover traces
// (engine Explain output) on every change.
package tsvsheetlsp
