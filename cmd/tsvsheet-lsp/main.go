package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sort"

	"github.com/urfave/cli/v3"

	"github.com/tsvsheet/tsvsheet.lsp/internal/app"
)

const (
	argUsage    = ``
	description = `tsvsheet-lsp is the Language Server Protocol (LSP) server for .tsvt spreadsheets.

It speaks LSP over stdio (JSON-RPC) so editors can offer live tsvsheet support,
consuming the go-tsvsheet engine as the single source of semantics.

Capabilities:
  diagnostics  - formula syntax errors and Check findings, mapped to cells
  hover        - a cell's computed value, formula, and resolved inputs`
	envName   = "TSVSHEET_LSP"
	envPrefix = envName + "_"
	name      = `tsvsheet-lsp`
	usage     = `Language Server Protocol server for .tsvt spreadsheets.`
)

var (
	appCreator    = createApp
	loggerConfig  app.LoggerConfig
	loggerCreator = productionLogger
	// serveFunc and streamFunc are indirected so tests exercise the root action
	// without starting the real stdio server (which would block on stdin).
	serveFunc  app.ServeFunc  = app.Serve
	streamFunc app.StreamFunc = app.Stdio
)

// productionLogger builds the application logger from the parsed logging flags.
// It is invoked from the root Before hook, after flag parsing has populated
// loggerConfig, so --log-level and --log-format take effect.
func productionLogger(_ *cli.Command) *slog.Logger {
	logger := app.NewLogger(os.Stderr, loggerConfig)
	return &logger
}

// version is the application version. Set via ldflags: -X main.version=1.0.0
var version = "dev"

// osExit is indirected so tests can observe the process exit code.
var osExit = os.Exit

func main() { osExit(run(os.Args)) }

// run builds and executes the CLI, returning the process exit code. Keeping the
// exit code as a return value (rather than calling os.Exit here) makes the whole
// run path testable.
func run(args []string) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	if err := appCreator(loggerCreator).Run(ctx, args); err != nil {
		slog.Error("Application error", "error", err)
		return 1
	}
	return 0
}

// createApp constructs the definition of the CLI. The root Action starts the
// stdio LSP server; the logging flags are bound into loggerConfig for the
// Before hook.
func createApp(getLogger app.GetLoggerFunc) *cli.Command {
	cliApp := &cli.Command{
		Name:                  name,
		Usage:                 usage,
		ArgsUsage:             argUsage,
		Description:           description,
		Version:               version,
		EnableShellCompletion: true,
		Action:                app.ServeAction(serveFunc, streamFunc),
		Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
			c.Root().Metadata[app.LoggerMetadataKey] = getLogger(c)
			return ctx, nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "log-level",
				Sources:     cli.EnvVars(envPrefix + "LOG_LEVEL"),
				Value:       "info",
				Usage:       "Set the logging level (debug, info, warn, error)",
				Destination: (*string)(&loggerConfig.LogLevel),
			},
			&cli.StringFlag{
				Name:        "log-format",
				Sources:     cli.EnvVars(envPrefix + "LOG_FORMAT"),
				Value:       "text",
				Usage:       "Set the log output format (text, json)",
				Destination: (*string)(&loggerConfig.LogFormat),
			},
		},
	}

	sort.Sort(cli.FlagsByName(cliApp.Flags))

	return cliApp
}
