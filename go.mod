module github.com/tsvsheet/tsvsheet.lsp

go 1.26.5

// GPL-licensed early history; the module is MIT from v0.3.1 onward.
retract [v0.1.0, v0.3.0]

require (
	github.com/gomatic/go-error v0.3.15
	github.com/stretchr/testify v1.11.1
	github.com/tsvsheet/go-tsvsheet v0.28.1
	github.com/urfave/cli/v3 v3.10.1
	go.lsp.dev/jsonrpc2 v1.0.1
	go.lsp.dev/protocol v1.0.1
	go.lsp.dev/uri v1.0.1
)

require (
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-json-experiment/json v0.0.0-20260623181947-01eb4420fa68 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/tsvsheet/go-isnow v0.1.10 // indirect
	golang.org/x/exp v0.0.0-20260727155853-b88d891fe743 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
