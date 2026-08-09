package app

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// TestServeEndToEnd drives a real JSON-RPC conversation over an in-memory pipe:
// a client initializes, opens documents, and hovers against a live Serve loop,
// and receives the pushed diagnostics — exercising the serve wiring and every
// advertised handler through the transport.
func TestServeEndToEnd(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	serverEnd, clientEnd := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	served := make(chan error, 1)
	go func() { served <- Serve(ctx, discardLogger(), serverEnd) }()

	client := newCapture()
	_, clientConn, remote := protocol.NewClient(ctx, client, jsonrpc2.NewStream(clientEnd))

	// initialize: the server advertises hover.
	init, err := remote.Initialize(ctx, &protocol.InitializeParams{})
	must.NoError(err)
	want.Equal(protocol.Boolean(true), init.Capabilities.HoverProvider)

	// didOpen a document with a bad formula: a syntax diagnostic is pushed.
	must.NoError(remote.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: testURI, Text: "a\tb\n1\t=1+\n"},
	}))
	published := waitDiagnostics(t, client.published)
	must.Len(published.Diagnostics, 1)
	want.Equal(protocol.DiagnosticSeverityError, published.Diagnostics[0].Severity)

	// didChange to a clean sheet, then hover a formula cell.
	must.NoError(remote.DidChange(ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: testURI},
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			&protocol.TextDocumentContentChangeWholeDocument{Text: "1\t2\n=A1\t=SUM(A1:B1)\n"},
		},
	}))
	cleaned := waitDiagnostics(t, client.published)
	want.Empty(cleaned.Diagnostics)

	h, err := remote.Hover(ctx, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: testURI},
			Position:     protocol.Position{Line: 1, Character: 4},
		},
	})
	must.NoError(err)
	must.NotNil(h)
	want.Contains(h.Contents.(*protocol.MarkupContent).Value, "**B2** = `3`")

	// shutdown/exit, then close the transport to end the serve loop.
	must.NoError(remote.Shutdown(ctx))
	must.NoError(remote.Exit(ctx))
	must.NoError(clientConn.Close())

	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the connection closed")
	}
}

func waitDiagnostics(t *testing.T, ch <-chan *protocol.PublishDiagnosticsParams) *protocol.PublishDiagnosticsParams {
	t.Helper()
	select {
	case params := <-ch:
		return params
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for published diagnostics")
		return nil
	}
}

func TestServeAction(t *testing.T) {
	t.Parallel()
	want, must := assert.New(t), require.New(t)

	logger := discardLogger()
	var gotLogger *slog.Logger
	var gotStream io.ReadWriteCloser
	serve := func(_ context.Context, l *slog.Logger, rwc io.ReadWriteCloser) error {
		gotLogger, gotStream = l, rwc
		return nil
	}
	stream := func() io.ReadWriteCloser { return stdioConn{} }

	command := &cli.Command{Name: "x", Metadata: map[string]any{LoggerMetadataKey: logger}}
	must.NoError(ServeAction(serve, stream)(context.Background(), command))
	want.Same(logger, gotLogger)
	want.NotNil(gotStream)
}

func TestStdio(t *testing.T) {
	t.Parallel()
	want := assert.New(t)
	rwc := Stdio()
	want.NotNil(rwc)
	want.NoError(rwc.Close())
}
