package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NHYCRaymond/go-backend-kit/config"
	"github.com/gin-gonic/gin"
)

// Server represents an HTTP server
type Server struct {
	config     config.ServerConfig
	router     *gin.Engine
	httpServer *http.Server
	logger     *slog.Logger
}

// New creates a new server instance
func New(cfg config.ServerConfig, router *gin.Engine, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	return &Server{
		config: cfg,
		router: router,
		logger: logger,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,

		// Timeouts
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,

		// Headers
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
	}

	// Start server in a goroutine
	go func() {
		s.logger.Info("Starting HTTP server", "address", addr, "env", s.config.Env)

		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	s.logger.Info("HTTP server started successfully", "address", addr)
	return nil
}

// StartWithTLS starts the HTTP server with TLS
func (s *Server) StartWithTLS(certFile, keyFile string) error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,

		// Timeouts
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,

		// Headers
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
	}

	// Start server in a goroutine
	go func() {
		s.logger.Info("Starting HTTPS server", "address", addr, "env", s.config.Env)

		if err := s.httpServer.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTPS server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	s.logger.Info("HTTPS server started successfully", "address", addr)
	return nil
}

// Stop stops the HTTP server gracefully
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Stopping HTTP server...")

	if s.httpServer == nil {
		return nil
	}

	// Shutdown server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("Server forced to shutdown", "error", err)
		return err
	}

	s.logger.Info("HTTP server stopped")
	return nil
}

// WaitForShutdown waits for shutdown signal and stops the server gracefully
func (s *Server) WaitForShutdown() {
	// Create a channel to receive OS signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// Wait for signal
	<-quit
	s.logger.Info("Received shutdown signal")

	// Create context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop the server
	if err := s.Stop(ctx); err != nil {
		s.logger.Error("Server shutdown failed", "error", err)
		os.Exit(1)
	}

	s.logger.Info("Server shutdown completed")
}

// StartAndWait starts the server and waits for shutdown signal
func (s *Server) StartAndWait() error {
	if err := s.Start(); err != nil {
		return err
	}

	s.WaitForShutdown()
	return nil
}

// StartTLSAndWait starts the server with TLS and waits for shutdown signal
func (s *Server) StartTLSAndWait(certFile, keyFile string) error {
	if err := s.StartWithTLS(certFile, keyFile); err != nil {
		return err
	}

	s.WaitForShutdown()
	return nil
}

// GetRouter returns the gin router
func (s *Server) GetRouter() *gin.Engine {
	return s.router
}

// GetHTTPServer returns the HTTP server
func (s *Server) GetHTTPServer() *http.Server {
	return s.httpServer
}

// SetupHealthCheck sets up health check endpoints
func (s *Server) SetupHealthCheck() {
	s.router.GET("/health", s.healthHandler)
	s.router.GET("/ready", s.readyHandler)
}

// healthHandler handles health check requests
func (s *Server) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"version":   "1.0.0", // This should come from build info
	})
}

// readyHandler handles readiness check requests
func (s *Server) readyHandler(c *gin.Context) {
	// Here you can add checks for database connectivity, external services, etc.
	c.JSON(http.StatusOK, gin.H{
		"status":    "ready",
		"timestamp": time.Now().Unix(),
	})
}

// Options represents server configuration options
type Options struct {
	Host              string
	Port              int
	Env               string
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	MaxHeaderBytes    int
	ShutdownTimeout   time.Duration
}

// NewWithOptions creates a new server with custom options
func NewWithOptions(router *gin.Engine, options Options, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	// Set defaults
	if options.Host == "" {
		options.Host = "0.0.0.0"
	}
	if options.Port == 0 {
		options.Port = 8080
	}
	if options.ReadTimeout == 0 {
		options.ReadTimeout = 30 * time.Second
	}
	if options.WriteTimeout == 0 {
		options.WriteTimeout = 30 * time.Second
	}
	if options.IdleTimeout == 0 {
		options.IdleTimeout = 120 * time.Second
	}
	if options.ReadHeaderTimeout == 0 {
		options.ReadHeaderTimeout = 10 * time.Second
	}
	if options.MaxHeaderBytes == 0 {
		options.MaxHeaderBytes = 1 << 20 // 1MB
	}
	if options.ShutdownTimeout == 0 {
		options.ShutdownTimeout = 30 * time.Second
	}

	return &Server{
		config: config.ServerConfig{
			Host: options.Host,
			Port: options.Port,
			Env:  options.Env,
		},
		router: router,
		logger: logger,
	}
}

// ServerManager manages multiple servers
type ServerManager struct {
	servers []*Server
	logger  *slog.Logger
}

// NewServerManager creates a new server manager
func NewServerManager(logger *slog.Logger) *ServerManager {
	if logger == nil {
		logger = slog.Default()
	}

	return &ServerManager{
		servers: make([]*Server, 0),
		logger:  logger,
	}
}

// AddServer adds a server to the manager
func (sm *ServerManager) AddServer(server *Server) {
	sm.servers = append(sm.servers, server)
}

// StartAll starts all managed servers
func (sm *ServerManager) StartAll() error {
	for _, server := range sm.servers {
		if err := server.Start(); err != nil {
			return err
		}
	}
	return nil
}

// StopAll stops all managed servers
func (sm *ServerManager) StopAll(ctx context.Context) error {
	for _, server := range sm.servers {
		if err := server.Stop(ctx); err != nil {
			sm.logger.Error("Failed to stop server", "error", err)
		}
	}
	return nil
}

// WaitForShutdown waits for shutdown signal and stops all servers
func (sm *ServerManager) WaitForShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	<-quit
	sm.logger.Info("Received shutdown signal")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := sm.StopAll(ctx); err != nil {
		sm.logger.Error("Failed to stop all servers", "error", err)
		os.Exit(1)
	}

	sm.logger.Info("All servers stopped")
}
