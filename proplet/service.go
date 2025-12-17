package proplet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"sync"
	"time"

	pkgmqtt "github.com/absmach/propeller/pkg/mqtt"
	"github.com/absmach/propeller/task"
)

const pollingInterval = 5 * time.Second

var (
	aliveTopicTemplate        = "m/%s/c/%s/messages/control/proplet/alive"
	discoveryTopicTemplate    = "m/%s/c/%s/messages/control/proplet/create"
	startTopicTemplate        = "m/%s/c/%s/messages/control/manager/start"
	stopTopicTemplate         = "m/%s/c/%s/messages/control/manager/stop"
	registryResponseTopic     = "m/%s/c/%s/messages/registry/server"
	fetchRequestTopicTemplate = "m/%s/c/%s/messages/registry/proplet"
)

type PropletService struct {
	domainID           string
	channelID          string
	clientID           string
	clientKey          string
	k8sNamespace       string
	livelinessInterval time.Duration
	pubsub             pkgmqtt.PubSub
	chunks             map[string][][]byte
	chunkMetadata      map[string]*ChunkPayload
	chunkReceivedTime  map[string]time.Time
	chunksMutex        sync.Mutex
	runtime            Runtime
	logger             *slog.Logger
}

type ChunkPayload struct {
	AppName     string `json:"app_name"`
	ChunkIdx    int    `json:"chunk_idx"`
	TotalChunks int    `json:"total_chunks"`
	Data        []byte `json:"data"`
}

func NewService(ctx context.Context, domainID, channelID, clientID, clientKey, k8sNamespace string, livelinessInterval time.Duration, pubsub pkgmqtt.PubSub, logger *slog.Logger, runtime Runtime) (*PropletService, error) {
	topic := fmt.Sprintf(discoveryTopicTemplate, domainID, channelID)
	payload := map[string]interface{}{
		"namespace":  k8sNamespace,
		"proplet_id": clientID,
	}
	if err := pubsub.Publish(ctx, topic, payload); err != nil {
		return nil, errors.Join(errors.New("failed to publish discovery"), err)
	}

	p := &PropletService{
		domainID:           domainID,
		channelID:          channelID,
		clientID:           clientID,
		clientKey:          clientKey,
		k8sNamespace:       k8sNamespace,
		livelinessInterval: livelinessInterval,
		pubsub:             pubsub,
		chunks:             make(map[string][][]byte),
		chunkMetadata:      make(map[string]*ChunkPayload),
		chunkReceivedTime:  make(map[string]time.Time),
		runtime:            runtime,
		logger:             logger,
	}

	go p.startLivelinessUpdates(ctx)
	go p.startChunkExpiryCleanup(ctx)

	return p, nil
}

func (p *PropletService) Run(ctx context.Context, logger *slog.Logger) error {
	topic := fmt.Sprintf(startTopicTemplate, p.domainID, p.channelID)
	if err := p.pubsub.Subscribe(ctx, topic, p.handleStartCommand(ctx)); err != nil {
		return fmt.Errorf("failed to subscribe to start topic: %w", err)
	}

	topic = fmt.Sprintf(stopTopicTemplate, p.domainID, p.channelID)
	if err := p.pubsub.Subscribe(ctx, topic, p.handleStopCommand(ctx)); err != nil {
		return fmt.Errorf("failed to subscribe to stop topic: %w", err)
	}

	topic = fmt.Sprintf(registryResponseTopic, p.domainID, p.channelID)
	if err := p.pubsub.Subscribe(ctx, topic, p.handleChunk(ctx)); err != nil {
		return fmt.Errorf("failed to subscribe to registry topics: %w", err)
	}

	logger.Info("Proplet service is running.")
	<-ctx.Done()

	return nil
}

func (p *PropletService) startLivelinessUpdates(ctx context.Context) {
	ticker := time.NewTicker(p.livelinessInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("stopping liveliness updates")

			return
		case <-ticker.C:
			topic := fmt.Sprintf(aliveTopicTemplate, p.domainID, p.channelID)
			payload := map[string]interface{}{
				"status":     "alive",
				"namespace":  p.k8sNamespace,
				"proplet_id": p.clientID,
			}

			if err := p.pubsub.Publish(ctx, topic, payload); err != nil {
				p.logger.Error("failed to publish liveliness message", slog.Any("error", err))
			}

			p.logger.Debug("Published liveliness message", slog.String("topic", topic))
		}
	}
}

func (p *PropletService) startChunkExpiryCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	const chunkTTL = 5 * time.Minute

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("stopping chunk expiry cleanup")
			return
		case <-ticker.C:
			p.chunksMutex.Lock()
			now := time.Now()
			for appName, receivedTime := range p.chunkReceivedTime {
				if now.Sub(receivedTime) > chunkTTL {
					p.logger.Warn("Expiring incomplete chunks",
						slog.String("app_name", appName),
						slog.Duration("age", now.Sub(receivedTime)))
					delete(p.chunks, appName)
					delete(p.chunkMetadata, appName)
					delete(p.chunkReceivedTime, appName)
				}
			}
			p.chunksMutex.Unlock()
		}
	}
}

func (p *PropletService) handleStartCommand(ctx context.Context) func(topic string, msg map[string]interface{}) error {
	return func(topic string, msg map[string]interface{}) error {
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}

		var payload task.Task
		if err := json.Unmarshal(data, &payload); err != nil {
			return err
		}

		req := startRequest{
			ID:           payload.ID,
			CLIArgs:      payload.CLIArgs,
			FunctionName: payload.Name,
			WasmFile:     payload.File,
			imageURL:     payload.ImageURL,
			Params:       payload.Inputs,
			Daemon:       payload.Daemon,
			Env:          payload.Env,
		}
		if err := req.Validate(); err != nil {
			return err
		}

		p.logger.Info("Received start command", slog.String("app_name", req.FunctionName))

		if req.WasmFile != nil {
			config := StartConfig{
				CLIArgs:      req.CLIArgs,
				ID:           req.ID,
				FunctionName: req.FunctionName,
				Daemon:       req.Daemon,
				Env:          req.Env,
				Args:         req.Params,
				WasmBinary:   req.WasmFile,
			}
			if err := p.runtime.StartApp(ctx, config); err != nil {
				return err
			}

			return nil
		}

		pl := map[string]interface{}{
			"app_name": req.imageURL,
		}
		tp := fmt.Sprintf(fetchRequestTopicTemplate, p.domainID, p.channelID)
		if err := p.pubsub.Publish(ctx, tp, pl); err != nil {
			return err
		}

		go func() {
			p.logger.Info("Waiting for chunks", slog.String("app_name", req.imageURL))

			for {
				p.chunksMutex.Lock()
				metadata, exists := p.chunkMetadata[req.imageURL]
				chunks := p.chunks[req.imageURL]
				p.chunksMutex.Unlock()

				if exists && len(chunks) == metadata.TotalChunks {
					// Try to assemble - will fail if any chunks are missing
					wasmBinary, complete := assembleChunks(chunks)
					if complete {
						p.logger.Info("All chunks received, deploying app", slog.String("app_name", req.imageURL))

						config := StartConfig{
							CLIArgs:      req.CLIArgs,
							ID:           req.ID,
							FunctionName: req.FunctionName,
							Daemon:       req.Daemon,
							Env:          req.Env,
							Args:         req.Params,
							WasmBinary:   wasmBinary,
						}

						if err := p.runtime.StartApp(ctx, config); err != nil {
							p.logger.Error("Failed to start app", slog.String("app_name", req.imageURL), slog.Any("error", err))
						}

						// Cleanup chunks after successful assembly
						p.chunksMutex.Lock()
						delete(p.chunks, req.imageURL)
						delete(p.chunkMetadata, req.imageURL)
						delete(p.chunkReceivedTime, req.imageURL)
						p.chunksMutex.Unlock()

						break
					}
				}

				time.Sleep(pollingInterval)
			}
		}()

		return nil
	}
}

func (p *PropletService) handleStopCommand(ctx context.Context) func(topic string, msg map[string]interface{}) error {
	return func(topic string, msg map[string]interface{}) error {
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}

		var req stopRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return err
		}

		if err := req.Validate(); err != nil {
			return err
		}

		if err := p.runtime.StopApp(ctx, req.ID); err != nil {
			return err
		}

		return nil
	}
}

func (p *PropletService) handleChunk(_ context.Context) func(topic string, msg map[string]interface{}) error {
	return func(topic string, msg map[string]interface{}) error {
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}

		var chunk ChunkPayload
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err
		}

		if err := chunk.Validate(); err != nil {
			return err
		}

		p.chunksMutex.Lock()
		defer p.chunksMutex.Unlock()

		// Initialize chunk storage if this is the first chunk
		if _, exists := p.chunkMetadata[chunk.AppName]; !exists {
			p.chunkMetadata[chunk.AppName] = &chunk
			// Pre-allocate slice with correct size
			p.chunks[chunk.AppName] = make([][]byte, chunk.TotalChunks)
			p.chunkReceivedTime[chunk.AppName] = time.Now()
		}

		// Store chunk at correct index
		if chunk.ChunkIdx >= 0 && chunk.ChunkIdx < len(p.chunks[chunk.AppName]) {
			p.chunks[chunk.AppName][chunk.ChunkIdx] = chunk.Data
		}

		log.Printf("Received chunk %d/%d for app '%s'\n", chunk.ChunkIdx+1, chunk.TotalChunks, chunk.AppName)

		return nil
	}
}

func assembleChunks(chunks [][]byte) ([]byte, bool) {
	var wasmBinary []byte
	for i, chunk := range chunks {
		if chunk == nil {
			// Chunk missing - cannot assemble yet
			return nil, false
		}
		wasmBinary = append(wasmBinary, chunk...)
		_ = i // Keep index for potential logging
	}

	return wasmBinary, true
}

func (c *ChunkPayload) Validate() error {
	if c.AppName == "" {
		return errors.New("chunk validation: app_name is required but missing")
	}
	if c.ChunkIdx < 0 || c.TotalChunks <= 0 {
		return fmt.Errorf("chunk validation: invalid chunk_idx (%d) or total_chunks (%d)", c.ChunkIdx, c.TotalChunks)
	}
	if len(c.Data) == 0 {
		return errors.New("chunk validation: data is empty")
	}

	return nil
}
