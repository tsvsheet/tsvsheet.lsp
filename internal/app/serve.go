package app

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// ServeFunc runs the LSP server over a transport until the connection ends.
// Serve is the production implementation; tests substitute a stub.
type ServeFunc func(ctx context.Context, logger *slog.Logger, rwc io.ReadWriteCloser) error

// StreamFunc yields the transport the server serves. Stdio is the production
// implementation; tests substitute a stub.
type StreamFunc func() io.ReadWriteCloser

// Serve wires a tsvsheet LSP server onto rwc and blocks until the JSON-RPC
// connection terminates, returning the connection's terminating error (nil on a
// clean end of stream). protocol.NewServer starts the read loop and embeds the
// client dispatcher in the request context, so handlers publish diagnostics
// without Serve holding the client.
func Serve(ctx context.Context, logger *slog.Logger, rwc io.ReadWriteCloser) error {
	srv := newServer(logger)
	_, conn, _ := protocol.NewServer(ctx, srv, jsonrpc2.NewStream(rwc))
	<-conn.Done()
	return conn.Err()
}

// ServeAction binds the serve loop and its transport into a cli.Command action,
// resolving the logger the root Before hook stored in command metadata.
func ServeAction(serve ServeFunc, stream StreamFunc) cli.ActionFunc {
	return func(ctx context.Context, command *cli.Command) error {
		return serve(ctx, GetLogger(command), stream())
	}
}

// stdioConn adapts the process's standard streams to an io.ReadWriteCloser.
// Close is a no-op: the process owns stdin/stdout, so closing them is the
// runtime's responsibility, not the connection's.
type stdioConn struct {
	io.Reader
	io.Writer
}

// Close is a no-op; the process's standard streams outlive the connection.
func (stdioConn) Close() error { return nil }

// Stdio is the production StreamFunc: the server speaks LSP over stdin/stdout.
func Stdio() io.ReadWriteCloser {
	return stdioConn{Reader: os.Stdin, Writer: os.Stdout}
}

var (
	_ ServeFunc  = Serve
	_ StreamFunc = Stdio
)
