package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const StopWaitTime = 30 * time.Second

type Config struct {
	Host              string        `env:"HOST"                       envDefault:"localhost"`
	Port              string        `env:"PORT"                       envDefault:""`
	CertFile          string        `env:"SERVER_CERT"                envDefault:""`
	KeyFile           string        `env:"SERVER_KEY"                 envDefault:""`
	ReadTimeout       time.Duration `env:"SERVER_READ_TIMEOUT"        envDefault:"15s"`
	WriteTimeout      time.Duration `env:"SERVER_WRITE_TIMEOUT"       envDefault:"15s"`
	ReadHeaderTimeout time.Duration `env:"SERVER_READ_HEADER_TIMEOUT" envDefault:"5s"`
	IdleTimeout       time.Duration `env:"SERVER_IDLE_TIMEOUT"        envDefault:"60s"`
	MaxHeaderBytes    int           `env:"SERVER_MAX_HEADER_BYTES"    envDefault:"1048576"`
}

type Server interface {
	Start() error
	Stop() error
}

type HTTPServer struct {
	server *http.Server
	name   string
	logger *slog.Logger
	cfg    Config
	ctx    context.Context //nolint:containedctx
	cancel context.CancelFunc
}

func NewServer(ctx context.Context, cancel context.CancelFunc, name string, cfg Config, handler http.Handler, logger *slog.Logger) Server {
	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)

	return &HTTPServer{
		server: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			IdleTimeout:       cfg.IdleTimeout,
			MaxHeaderBytes:    cfg.MaxHeaderBytes,
		},
		name:   name,
		logger: logger,
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *HTTPServer) Start() error {
	errCh := make(chan error, 1)
	switch {
	case s.cfg.CertFile != "" || s.cfg.KeyFile != "":
		certs, err := tls.LoadX509KeyPair(s.cfg.CertFile, s.cfg.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load TLS cert: %w", err)
		}
		if s.server.TLSConfig == nil {
			s.server.TLSConfig = &tls.Config{}
		}
		s.server.TLSConfig.Certificates = append(s.server.TLSConfig.Certificates, certs)
		s.logger.Info(fmt.Sprintf("%s service HTTPS server listening at %s", s.name, s.server.Addr))
		go func() { errCh <- s.server.ListenAndServeTLS("", "") }()
	default:
		s.logger.Info(fmt.Sprintf("%s service HTTP server listening at %s", s.name, s.server.Addr))
		go func() { errCh <- s.server.ListenAndServe() }()
	}

	select {
	case <-s.ctx.Done():
		return s.Stop()
	case err := <-errCh:
		return err
	}
}

func (s *HTTPServer) Stop() error {
	defer s.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), StopWaitTime)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error(fmt.Sprintf("%s service server shutdown error at %s: %s", s.name, s.server.Addr, err))

		return fmt.Errorf("%s service server shutdown error at %s: %w", s.name, s.server.Addr, err)
	}
	s.logger.Info(fmt.Sprintf("%s service server stopped at %s", s.name, s.server.Addr))

	return nil
}

func StopSignalHandler(ctx context.Context, cancel context.CancelFunc, logger *slog.Logger, svcName string, servers ...Server) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		cancel()
		logger.Info(fmt.Sprintf("%s service shutdown by signal: %s", svcName, sig))
		var errs []error
		for _, s := range servers {
			if err := s.Stop(); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("shutdown errors: %v", errs)
		}

		return nil
	case <-ctx.Done():
		return nil
	}
}
