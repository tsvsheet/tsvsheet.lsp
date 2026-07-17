package constants

import errs "github.com/gomatic/go-error"

// Keep these constants sorted alphabetically.
const (
	// ErrNoClient is returned by a notification handler that needs to push a
	// message back to the editor but finds no LSP client dispatcher in the
	// request context — the connection was not wired through protocol.NewServer.
	ErrNoClient errs.Const = "no LSP client in context"
)
