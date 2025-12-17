package proplet

import "context"

var ResultsTopic = "m/%s/c/%s/messages/control/proplet/results"

type StartConfig struct {
	CLIArgs      []string
	ID           string
	FunctionName string
	Daemon       bool
	Env          map[string]string
	Args         []uint64
	WasmBinary   []byte
}

type Runtime interface {
	StartApp(ctx context.Context, config StartConfig) error
	StopApp(ctx context.Context, id string) error
}
