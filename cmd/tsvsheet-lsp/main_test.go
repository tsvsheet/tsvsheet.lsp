package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/tsvsheet/tsvsheet.lsp/internal/app"
)

// stubCommand builds a minimal cli.Command whose Action returns err. It skips
// flag parsing so the ambient test-binary args (-test.*) never reach the parser.
func stubCommand(err error) *cli.Command {
	return &cli.Command{
		Name:            "x",
		SkipFlagParsing: true,
		Writer:          io.Discard,
		Action:          func(context.Context, *cli.Command) error { return err },
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()
	assert.New(t).Equal("dev", version)
}

func TestProductionLogger(t *testing.T) {
	t.Parallel()
	assert.New(t).NotNil(productionLogger(nil))
}

func TestCreateApp(t *testing.T) {
	origServe, origStream := serveFunc, streamFunc
	t.Cleanup(func() { serveFunc, streamFunc = origServe, origStream })

	want, must := assert.New(t), require.New(t)

	served := false
	serveFunc = func(context.Context, *slog.Logger, io.ReadWriteCloser) error {
		served = true
		return nil
	}
	streamFunc = func() io.ReadWriteCloser { return nil }

	injected := slog.New(slog.NewTextHandler(io.Discard, nil))
	getLogger := func(*cli.Command) *slog.Logger { return injected }

	cmd := createApp(getLogger)
	must.NotNil(cmd)
	want.Equal(name, cmd.Name)
	want.Equal(version, cmd.Version)

	// Running with just the program name takes the normal path: flags parse, the
	// Before hook stores the injected logger, then the root action starts the
	// (stubbed) serve loop.
	cmd.Writer = &bytes.Buffer{}
	must.NoError(cmd.Run(context.Background(), []string{name}))
	want.Same(injected, cmd.Root().Metadata[app.LoggerMetadataKey])
	want.True(served)
}

func TestRun(t *testing.T) {
	origCreator, origLogger := appCreator, loggerCreator
	t.Cleanup(func() { appCreator, loggerCreator = origCreator, origLogger })

	loggerCreator = func(*cli.Command) *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

	tests := []struct {
		actionErr error
		name      string
		want      int
	}{
		{name: "success returns zero", actionErr: nil, want: 0},
		{name: "action error returns one", actionErr: assert.AnError, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appCreator = func(app.GetLoggerFunc) *cli.Command { return stubCommand(tt.actionErr) }
			assert.New(t).Equal(tt.want, run([]string{"x"}))
		})
	}
}

func TestMainFunc(t *testing.T) {
	origCreator, origLogger, origExit := appCreator, loggerCreator, osExit
	t.Cleanup(func() { appCreator, loggerCreator, osExit = origCreator, origLogger, origExit })

	loggerCreator = func(*cli.Command) *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
	appCreator = func(app.GetLoggerFunc) *cli.Command { return stubCommand(nil) }

	var code int
	osExit = func(c int) { code = c }

	main()
	assert.New(t).Equal(0, code)
}
