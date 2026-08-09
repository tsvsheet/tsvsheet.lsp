package app

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/tsvsheet/tsvsheet.lsp/internal/constants"
)

// captureClient records the diagnostics the server publishes. It embeds
// protocol.Client so the notification methods the server never calls are
// satisfied without stubs.
type captureClient struct {
	protocol.Client
	published chan *protocol.PublishDiagnosticsParams
}

func (c *captureClient) PublishDiagnostics(_ context.Context, params *protocol.PublishDiagnosticsParams) error {
	c.published <- params
	return nil
}

func newCapture() *captureClient {
	return &captureClient{published: make(chan *protocol.PublishDiagnosticsParams, 8)}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func clientContext(client protocol.Client) context.Context {
	return protocol.WithClient(context.Background(), client)
}

const testURI = uri.URI("file:///test.tsvt")

func TestInitializeAdvertisesCapabilities(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	result, err := newServer(discardLogger()).Initialize(context.Background(), &protocol.InitializeParams{})
	must.NoError(err)
	must.NotNil(result)

	want.Equal(serverName, result.ServerInfo.Name)
	want.Equal(protocol.Boolean(true), result.Capabilities.HoverProvider)

	sync, ok := result.Capabilities.TextDocumentSync.(*protocol.TextDocumentSyncOptions)
	must.True(ok)
	must.NotNil(sync.OpenClose)
	want.True(*sync.OpenClose)
	must.NotNil(sync.Change)
	want.Equal(protocol.TextDocumentSyncKindFull, *sync.Change)

	ver, ok := result.ServerInfo.Version.Get()
	want.True(ok)
	want.Equal(serverVersion, ver)
}

func TestLifecycleNoOps(t *testing.T) {
	t.Parallel()
	want := assert.New(t)
	s := newServer(discardLogger())
	ctx := context.Background()

	want.NoError(s.Initialized(ctx, &protocol.InitializedParams{}))
	want.NoError(s.Shutdown(ctx))
	want.NoError(s.Exit(ctx))
	want.NoError(s.SetTrace(ctx, &protocol.SetTraceParams{}))
}

func TestDidOpenPublishesDiagnostics(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	client := newCapture()
	s := newServer(discardLogger())
	params := &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: testURI, Text: "a\tb\n1\t=1+\n"},
	}
	must.NoError(s.DidOpen(clientContext(client), params))

	published := <-client.published
	want.Equal(testURI, published.URI)
	must.Len(published.Diagnostics, 1)
	want.Equal(protocol.DiagnosticSeverityError, published.Diagnostics[0].Severity)
}

func TestDidOpenWithoutClient(t *testing.T) {
	t.Parallel()
	s := newServer(discardLogger())
	params := &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: testURI, Text: "a\n"},
	}
	assert.New(t).ErrorIs(s.DidOpen(context.Background(), params), constants.ErrNoClient)
}

func TestDidChangeWholeDocument(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	client := newCapture()
	s := newServer(discardLogger())
	params := &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: testURI},
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			&protocol.TextDocumentContentChangeWholeDocument{Text: "=1+\n"},
		},
	}
	must.NoError(s.DidChange(clientContext(client), params))

	published := <-client.published
	want.Len(published.Diagnostics, 1)

	// The document is now stored, so hover resolves against the new text.
	stored, ok := s.docs.get(testURI)
	want.True(ok)
	want.Equal(documentText("=1+\n"), stored)
}

func TestDidChangeNoWholeDocument(t *testing.T) {
	t.Parallel()
	want := assert.New(t)

	client := newCapture()
	s := newServer(discardLogger())
	params := &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: testURI},
		},
		ContentChanges: nil,
	}
	want.NoError(s.DidChange(clientContext(client), params))
	want.Empty(client.published) // nothing published for a no-op change
}

func TestDidClose(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	client := newCapture()
	s := newServer(discardLogger())
	ctx := clientContext(client)

	must.NoError(s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: testURI, Text: "1\n"},
	}))
	<-client.published // drain the open diagnostics

	must.NoError(s.DidClose(ctx, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: testURI},
	}))

	cleared := <-client.published
	want.Empty(cleared.Diagnostics) // diagnostics cleared on close

	_, ok := s.docs.get(testURI)
	want.False(ok) // document forgotten
}

func TestHoverOpenDocument(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	client := newCapture()
	s := newServer(discardLogger())
	ctx := clientContext(client)

	must.NoError(s.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: testURI, Text: "42\t7\n"},
	}))
	<-client.published

	h, err := s.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: testURI},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	must.NoError(err)
	must.NotNil(h)
	want.Equal("**A1** = `42`", h.Contents.(*protocol.MarkupContent).Value)
}

func TestHoverUnknownDocument(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	h, err := newServer(discardLogger()).Hover(context.Background(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: testURI},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	must.NoError(err)
	want.Nil(h)
}

func TestLatestText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		wantText documentText
		changes  []protocol.TextDocumentContentChangeEvent
		wantOK   bool
	}{
		{
			name: "last whole document wins",
			changes: []protocol.TextDocumentContentChangeEvent{
				&protocol.TextDocumentContentChangeWholeDocument{Text: "old"},
				&protocol.TextDocumentContentChangeWholeDocument{Text: "new"},
			},
			wantText: "new",
			wantOK:   true,
		},
		{
			name: "skips trailing partial change",
			changes: []protocol.TextDocumentContentChangeEvent{
				&protocol.TextDocumentContentChangeWholeDocument{Text: "whole"},
				&protocol.TextDocumentContentChangePartial{Text: "partial"},
			},
			wantText: "whole",
			wantOK:   true,
		},
		{
			name:    "none present",
			changes: nil,
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := assert.New(t)
			text, ok := latestText(tt.changes)
			want.Equal(tt.wantOK, ok)
			want.Equal(tt.wantText, text)
		})
	}
}

func TestDocumentsStore(t *testing.T) {
	t.Parallel()
	want := assert.New(t)
	docs := newDocuments()

	_, ok := docs.get(testURI)
	want.False(ok)

	docs.set(testURI, "hello")
	text, ok := docs.get(testURI)
	want.True(ok)
	want.Equal(documentText("hello"), text)

	docs.remove(testURI)
	_, ok = docs.get(testURI)
	want.False(ok)
}

// TestServerIsCopyableBecauseItsStoreIsSharedByPointer pins the reason its
// methods take value receivers. If copying a server duplicated its document
// store, a handler working on a copy would write into a store nobody reads, and
// the editor would see stale text with no error anywhere to explain it.
func TestServerIsCopyableBecauseItsStoreIsSharedByPointer(t *testing.T) {
	t.Parallel()
	original := newServer(discardLogger())
	duplicate := original

	original.docs.set("file:///a.tsvt", "1\t2")

	got, ok := duplicate.docs.get("file:///a.tsvt")

	assert.True(t, ok, "the copy sees the same store")
	assert.Equal(t, documentText("1\t2"), got)
}
