package app

import (
	"context"
	"log/slog"
	"sync"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/tsvsheet/tsvsheet.lsp/internal/constants"
)

const (
	// serverName identifies this server to the editor in the initialize result.
	serverName = "tsvsheet-lsp"
	// serverVersion is the version reported to the editor. The released binary's
	// version is a separate main-package ldflags value; this is the protocol
	// identity string.
	serverVersion = "dev"
)

// documents is the mutable store of open document texts, keyed by URI. It wraps
// a mutex — stateful machinery that must not be copied — so its methods take a
// pointer receiver; the server holds it by pointer and stays copyable itself.
type documents struct {
	src map[uri.URI]documentText
	mu  sync.RWMutex
}

// newDocuments returns an empty document store.
func newDocuments() *documents {
	return &documents{src: map[uri.URI]documentText{}}
}

// set records the current text of a document.
func (d *documents) set(docURI uri.URI, text documentText) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.src[docURI] = text
}

// get returns a document's text and whether it is open.
func (d *documents) get(docURI uri.URI) (documentText, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	text, ok := d.src[docURI]
	return text, ok
}

// remove forgets a closed document.
func (d *documents) remove(docURI uri.URI) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.src, docURI)
}

// server implements the subset of protocol.Server the editor uses: the
// lifecycle handshake, full text-document sync, and hover. The embedded
// protocol.Server supplies the many capabilities this server does not advertise;
// a conformant client never calls them, so it is left nil. The server value is
// copyable — its document store is shared by pointer — so its methods take value
// receivers.
type server struct {
	protocol.Server
	logger *slog.Logger
	docs   *documents
}

// newServer builds a server with an empty document store.
func newServer(logger *slog.Logger) server {
	return server{logger: logger, docs: newDocuments()}
}

// Initialize advertises the server's capabilities: full text-document sync and
// hover.
func (server) Initialize(
	_ context.Context,
	_ *protocol.InitializeParams,
) (*protocol.InitializeResult, error) {
	return capabilities(), nil
}

// Initialized acknowledges the client's post-initialize notification.
func (server) Initialized(_ context.Context, _ *protocol.InitializedParams) error { return nil }

// Shutdown acknowledges a shutdown request; the connection closes on Exit.
func (server) Shutdown(_ context.Context) error { return nil }

// Exit ends the session; the transport closing stops the serve loop.
func (server) Exit(_ context.Context) error { return nil }

// SetTrace accepts and ignores the client's trace-level change.
func (server) SetTrace(_ context.Context, _ *protocol.SetTraceParams) error { return nil }

// DidOpen records a newly opened document and publishes its diagnostics.
func (s server) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {
	return s.sync(ctx, params.TextDocument.URI, documentText(params.TextDocument.Text))
}

// DidChange records a full-sync document update and republishes diagnostics. A
// change carrying no whole-document text is ignored.
func (s server) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	text, ok := latestText(params.ContentChanges)
	if !ok {
		return nil
	}
	return s.sync(ctx, params.TextDocument.URI, text)
}

// DidClose forgets a closed document and clears its diagnostics.
func (s server) DidClose(ctx context.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.docs.remove(params.TextDocument.URI)
	return publishDiagnostics(ctx, params.TextDocument.URI, []protocol.Diagnostic{})
}

// Hover returns the hover for the position, or nil when the document is not open
// or the position has no cell to explain.
func (s server) Hover(_ context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	text, ok := s.docs.get(params.TextDocument.URI)
	if !ok {
		return nil, nil //nolint:nilnil // "no hover here" is a nil result, not an error, per LSP.
	}
	s.logger.Debug("hover", slog.String("uri", string(params.TextDocument.URI)))
	return hover(text, params.Position), nil
}

// sync stores a document's text and publishes its freshly computed diagnostics.
func (s server) sync(ctx context.Context, docURI uri.URI, text documentText) error {
	s.docs.set(docURI, text)
	s.logger.Debug("sync", slog.String("uri", string(docURI)))
	return publishDiagnostics(ctx, docURI, diagnose(text))
}

// latestText extracts the most recent whole-document text from a full-sync
// change batch, reporting false when none is present.
func latestText(changes []protocol.TextDocumentContentChangeEvent) (documentText, bool) {
	for i := len(changes) - 1; i >= 0; i-- {
		if whole, ok := changes[i].(*protocol.TextDocumentContentChangeWholeDocument); ok {
			return documentText(whole.Text), true
		}
	}
	return "", false
}

// publishDiagnostics sends a document's diagnostics to the editor via the client
// dispatcher carried in the request context.
func publishDiagnostics(ctx context.Context, docURI uri.URI, diags []protocol.Diagnostic) error {
	client, ok := protocol.ClientFromContext(ctx)
	if !ok {
		return constants.ErrNoClient
	}
	return client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         docURI,
		Diagnostics: diags,
	})
}

// capabilities is the server's advertised capability set: open/close plus
// full-document change sync, and hover.
func capabilities() *protocol.InitializeResult {
	change := protocol.TextDocumentSyncKindFull
	openClose := true
	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: &openClose,
				Change:    &change,
			},
			HoverProvider: protocol.Boolean(true),
		},
		ServerInfo: protocol.ServerInfo{
			Name:    serverName,
			Version: protocol.NewOptional(serverVersion),
		},
	}
}
