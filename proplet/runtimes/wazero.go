package runtimes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/absmach/propeller/pkg/mqtt"
	"github.com/absmach/propeller/proplet"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type wazeroRuntime struct {
	mutex     sync.Mutex
	runtimes  map[string]wazero.Runtime
	pubsub    mqtt.PubSub
	domainID  string
	channelID string
	logger    *slog.Logger
}

func NewWazeroRuntime(logger *slog.Logger, pubsub mqtt.PubSub, domainID, channelID string) proplet.Runtime {
	return &wazeroRuntime{
		runtimes:  make(map[string]wazero.Runtime),
		pubsub:    pubsub,
		domainID:  domainID,
		channelID: channelID,
		logger:    logger,
	}
}

func (w *wazeroRuntime) StartApp(ctx context.Context, wasmBinary []byte, cliArgs []string, id, functionName string, daemon bool, env map[string]string, args ...uint64) error {
	r := wazero.NewRuntime(ctx)

	w.mutex.Lock()
	w.runtimes[id] = r
	w.mutex.Unlock()

	// Instantiate WASI, which implements host functions needed for TinyGo to
	// implement `panic`.
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	module, err := r.InstantiateWithConfig(ctx, wasmBinary, wazero.NewModuleConfig().WithStartFunctions("_initialize"))
	if err != nil {
		return errors.Join(errors.New("failed to instantiate Wasm module"), err)
	}

	function := module.ExportedFunction(functionName)
	if function == nil {
		return errors.New("failed to find exported function")
	}

	if daemon {
		// For daemon tasks, run in background and keep runtime in map
		// Don't auto-cleanup to allow stop_app to be called later
		go func() {
			results, err := function.Call(ctx, args...)

			payload := map[string]interface{}{
				"task_id": id,
			}

			if err != nil {
				w.logger.Error("failed to call function", slog.String("id", id), slog.String("function", functionName), slog.String("error", err.Error()))
				payload["error"] = err.Error()
			} else {
				payload["results"] = results
			}

			topic := fmt.Sprintf(proplet.ResultsTopic, w.domainID, w.channelID)
			if err := w.pubsub.Publish(ctx, topic, payload); err != nil {
				w.logger.Error("failed to publish results", slog.String("id", id), slog.String("error", err.Error()))
			}

			// For daemon tasks, DO NOT call StopApp automatically
			// The runtime remains in the map until explicitly stopped
			w.logger.Info("Daemon task completed, runtime kept active for explicit stop", slog.String("id", id))
		}()

		return nil
	}

	results, err := function.Call(ctx, args...)

	if stopErr := w.StopApp(ctx, id); stopErr != nil {
		w.logger.Error("failed to stop app", slog.String("id", id), slog.String("error", stopErr.Error()))
	}

	payload := map[string]interface{}{
		"task_id": id,
	}

	if err != nil {
		w.logger.Error("failed to call function", slog.String("id", id), slog.String("function", functionName), slog.String("error", err.Error()))
		payload["error"] = err.Error()
		return err
	}

	payload["results"] = results

	topic := fmt.Sprintf(proplet.ResultsTopic, w.domainID, w.channelID)
	if err := w.pubsub.Publish(ctx, topic, payload); err != nil {
		w.logger.Error("failed to publish results", slog.String("id", id), slog.String("error", err.Error()))
		return err
	}

	w.logger.Info("Finished running app", slog.String("id", id))

	return nil
}

func (w *wazeroRuntime) StopApp(ctx context.Context, id string) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	r, exists := w.runtimes[id]
	if !exists {
		return errors.New("there is no runtime for this id")
	}

	if err := r.Close(ctx); err != nil {
		return err
	}

	delete(w.runtimes, id)

	return nil
}
