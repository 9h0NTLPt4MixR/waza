package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/microsoft/waza/internal/jsonrpc"
	"github.com/microsoft/waza/internal/mcp"
	"github.com/microsoft/waza/internal/platform/api"
	"github.com/microsoft/waza/internal/platform/auth"
	"github.com/microsoft/waza/internal/platform/db"
	"github.com/microsoft/waza/internal/projectconfig"
	"github.com/microsoft/waza/internal/webapi"
	"github.com/microsoft/waza/internal/webserver"
	"github.com/spf13/cobra"
)

func newServeCommand() *cobra.Command {
	var tcpAddr string
	var tcpAllowRemote bool
	var httpMode bool
	var httpPort int
	var noBrowser bool
	var resultsDir string
	var platformMode bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the waza server (HTTP dashboard or JSON-RPC)",
		Long: `Start the waza server.

By default, an HTTP server is started that serves the waza dashboard and API.
The browser is opened automatically (disable with --no-browser).

Use --platform to start the hosted platform server with GitHub OAuth,
Cosmos DB persistence, and optional ADC sandbox execution. Platform mode
binds to 0.0.0.0 for container deployment and disables auto-browser.

Use --tcp to start a JSON-RPC 2.0 server instead (for IDE integration).
TCP defaults to loopback (127.0.0.1) for security. Use --tcp-allow-remote to bind
to all interfaces.

JSON-RPC methods (when using --tcp or stdin/stdout):
  eval.run       Run an eval (returns run ID)
  eval.list      List available evals in a directory
  eval.get       Get eval details
  eval.validate  Validate an eval spec
  task.list      List tasks for an eval
  task.get       Get task details
  run.status     Get run status
  run.cancel     Cancel a running eval`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Apply .waza.yaml defaults when CLI flags not explicitly set.
			cfg, err := projectconfig.Load(".")
			if err != nil || cfg == nil {
				cfg = projectconfig.New()
			}
			if !cmd.Flags().Changed("port") && cfg.Server.Port > 0 {
				httpPort = cfg.Server.Port
			}
			if !cmd.Flags().Changed("results-dir") && cfg.Server.ResultsDir != "" {
				resultsDir = cfg.Server.ResultsDir
			}

			logger := slog.Default()

			// JSON-RPC TCP mode
			if tcpAddr != "" {
				return runJSONRPC(tcpAddr, tcpAllowRemote, logger)
			}

			// Platform mode — hosted multi-user server.
			if platformMode {
				return runPlatformServer(cmd, cfg, httpPort, resultsDir, logger)
			}

			// HTTP mode (default) — also start MCP on stdio.
			if httpMode || !cmd.Flags().Changed("tcp") {
				ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
				defer stop()

				// Start MCP server on stdio in the background.
				go func() {
					logger.Info("MCP server running on stdio")
					mcp.ServeStdio(ctx, os.Stdin, os.Stdout, logger)
				}()

				// Prepare storage config if enabled.
				var storageCfg *projectconfig.StorageConfig
				if cfg.Storage.Enabled {
					storageCfg = &cfg.Storage
				}

				srv, err := webserver.New(webserver.Config{
					Port:          httpPort,
					ResultsDir:    resultsDir,
					NoBrowser:     noBrowser,
					Logger:        logger,
					StorageConfig: storageCfg,
				})
				if err != nil {
					return fmt.Errorf("failed to initialize web server: %w", err)
				}
				return srv.ListenAndServe(ctx)
			}

			// Fallback: JSON-RPC on stdio
			return runJSONRPCStdio(logger)
		},
	}

	cmd.Flags().StringVar(&tcpAddr, "tcp", "", "TCP address to listen on for JSON-RPC (e.g., :9000)")
	cmd.Flags().BoolVar(&tcpAllowRemote, "tcp-allow-remote", false,
		"Allow binding to non-loopback addresses (WARNING: exposes the server to the network with no authentication)")
	cmd.Flags().BoolVar(&httpMode, "http", false, "Start HTTP dashboard server (default when --tcp is not set)")
	cmd.Flags().IntVar(&httpPort, "port", 3000, "HTTP server port")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Don't auto-open the browser")
	cmd.Flags().StringVar(&resultsDir, "results-dir", ".", "Directory to read results from")
	cmd.Flags().BoolVar(&platformMode, "platform", false,
		"Start hosted platform server (GitHub OAuth, Cosmos DB, ADC)")

	return cmd
}

// runPlatformServer starts the multi-user platform server with auth, DB, and
// optional ADC sandbox execution. It binds to 0.0.0.0 for container deployment.
func runPlatformServer(cmd *cobra.Command, cfg *projectconfig.ProjectConfig, port int, resultsDir string, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Initialize Cosmos DB store ---
	cosmosEndpoint := envOrDefault("COSMOS_ENDPOINT", "")
	cosmosKey := envOrDefault("COSMOS_KEY", "")
	encryptionKey := envOrDefault("ENCRYPTION_KEY", "")

	if cosmosEndpoint == "" || encryptionKey == "" {
		return fmt.Errorf("platform mode requires COSMOS_ENDPOINT and ENCRYPTION_KEY environment variables")
	}

	var store db.Store
	if cosmosKey != "" {
		// Key-based auth (local dev)
		s, err := db.NewCosmosStore(cosmosEndpoint, cosmosKey, encryptionKey)
		if err != nil {
			return fmt.Errorf("initializing cosmos store: %w", err)
		}
		store = s
		defer s.Close()
		logger.Info("Cosmos DB store initialized (key auth)", "endpoint", cosmosEndpoint)
	} else {
		// Managed identity auth (production / Azure)
		s, err := db.NewCosmosStoreWithIdentity(cosmosEndpoint, encryptionKey)
		if err != nil {
			return fmt.Errorf("initializing cosmos store with identity: %w", err)
		}
		store = s
		defer s.Close()
		logger.Info("Cosmos DB store initialized (managed identity)", "endpoint", cosmosEndpoint)
	}

	// --- Initialize GitHub OAuth provider ---
	githubClientID := envOrDefault("GITHUB_CLIENT_ID", "")
	githubClientSecret := envOrDefault("GITHUB_CLIENT_SECRET", "")
	githubRedirectURL := envOrDefault("GITHUB_REDIRECT_URL", "")
	jwtSecret := envOrDefault("JWT_SECRET", "")

	if githubClientID == "" || githubClientSecret == "" || jwtSecret == "" {
		return fmt.Errorf("platform mode requires GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET, and JWT_SECRET environment variables")
	}

	authProvider := auth.NewGitHubProvider(auth.GitHubOAuthConfig{
		ClientID:     githubClientID,
		ClientSecret: githubClientSecret,
		RedirectURL:  githubRedirectURL,
	}, jwtSecret, store)
	authMiddleware := auth.NewAuthMiddleware(authProvider)
	logger.Info("GitHub OAuth provider initialized")

	// --- Build platform dependencies ---
	deps := &api.Dependencies{
		Store:          store,
		Auth:           authProvider,
		AuthMiddleware: authMiddleware,
		ADCEngine:      nil, // ADC engine initialized below if configured
	}

	// --- Initialize ADC engine (optional) ---
	adcAPIKey := envOrDefault("ADC_API_KEY", "")
	if adcAPIKey != "" {
		logger.Info("ADC engine configured but SDK not yet in go.mod — dispatch is no-op")
		// When the ADC SDK is added to go.mod, uncomment:
		// adcCfg := adc.ADCConfig{
		//     APIKey:    adcAPIKey,
		//     APIURL:    envOrDefault("ADC_API_URL", adc.DefaultAPIURL),
		//     DiskImage: envOrDefault("ADC_DISK_IMAGE", ""),
		// }
		// engine := adc.NewEngine(adcCfg)
		// if err := engine.Initialize(ctx); err != nil {
		//     return fmt.Errorf("initializing ADC engine: %w", err)
		// }
		// defer engine.Shutdown(ctx)
		// deps.ADCEngine = engine
	}

	// --- Build HTTP mux ---
	mux := http.NewServeMux()

	// Register platform API routes.
	api.RegisterRoutes(mux, deps)

	// Register existing webapi routes (health, summary, runs, storage status)
	// alongside platform routes so the dashboard still works.
	var storageCfg *projectconfig.StorageConfig
	if cfg.Storage.Enabled {
		storageCfg = &cfg.Storage
	}

	// Create the local file store for dashboard run data.
	// Register only non-conflicting webapi routes (summary, health, storage status).
	// The /api/runs endpoints are handled by the platform API with auth.
	fileStore := webapi.NewFileStore(resultsDir)
	dashHandlers := webapi.NewHandlers(fileStore)
	mux.HandleFunc("GET /api/health", dashHandlers.HandleHealth)
	mux.HandleFunc("GET /api/summary", dashHandlers.HandleSummary)
	if storageCfg != nil {
		dashWithStorage := webapi.NewHandlersWithStorage(fileStore, &webapi.StorageConfig{
			Configured: true,
			Provider:   storageCfg.Provider,
			Account:    storageCfg.AccountName,
		})
		mux.HandleFunc("GET /api/storage/status", dashWithStorage.HandleStorageStatus)
	} else {
		mux.HandleFunc("GET /api/storage/status", dashHandlers.HandleStorageStatus)
	}

	// Health check for container orchestrators.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","mode":"platform"}`)
	})

	// Serve the embedded React SPA for all non-API routes.
	// API routes registered above take precedence over this catch-all.
	spaHandler, err := webserver.SPAHandler()
	if err != nil {
		return fmt.Errorf("failed to initialize SPA handler: %w", err)
	}
	mux.Handle("/", spaHandler)

	// --- Start HTTP server on 0.0.0.0 ---
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("platform server starting", "address", addr)
	fmt.Printf("waza platform: http://%s\n", addr)

	// Graceful shutdown.
	go func() {
		<-ctx.Done()
		logger.Info("shutting down platform server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("platform server shutdown error", "error", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("platform server error: %w", err)
	}
	return nil
}

func runJSONRPC(tcpAddr string, allowRemote bool, logger *slog.Logger) error {
	registry := jsonrpc.NewMethodRegistry()
	hctx := jsonrpc.NewHandlerContext()
	jsonrpc.RegisterHandlers(registry, hctx)

	server := jsonrpc.NewServer(registry, logger)

	tcpAddr = resolveTCPAddr(tcpAddr, allowRemote, logger)
	listener, err := jsonrpc.NewTCPListener(tcpAddr, server)
	if err != nil {
		return fmt.Errorf("failed to start TCP server: %w", err)
	}
	defer listener.Close() //nolint:errcheck
	fmt.Fprintf(os.Stderr, "JSON-RPC server listening on %s\n", listener.Addr())
	return listener.Serve()
}

func runJSONRPCStdio(logger *slog.Logger) error {
	registry := jsonrpc.NewMethodRegistry()
	hctx := jsonrpc.NewHandlerContext()
	jsonrpc.RegisterHandlers(registry, hctx)

	server := jsonrpc.NewServer(registry, logger)
	fmt.Fprintln(os.Stderr, "JSON-RPC server running on stdio")
	server.ServeStdio(os.Stdin, os.Stdout)
	return nil
}

// resolveTCPAddr ensures TCP addresses default to loopback unless --tcp-allow-remote is set.
func resolveTCPAddr(addr string, allowRemote bool, logger *slog.Logger) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// Likely just a port like "9000"; treat as ":9000".
		host = ""
		port = addr
	}

	if allowRemote {
		logger.Warn("TCP server binding to all interfaces — no authentication is provided",
			"address", addr)
		return addr
	}

	// Default to loopback if no host specified or if 0.0.0.0/:: is used without --tcp-allow-remote.
	if host == "" || host == "0.0.0.0" || host == "::" {
		logger.Info("JSON-RPC server listening on TCP (local only)")
		return net.JoinHostPort("127.0.0.1", port)
	}

	return addr
}

// envOrDefault returns the value of the environment variable named key,
// or defaultVal if the variable is empty or unset.
func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
