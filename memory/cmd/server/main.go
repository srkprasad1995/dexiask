// Command server is the Dexiask memory service: an FS-backed store exposed as an
// MCP server (memory_view/write/search) and a small REST API, plus a periodic
// dream/consolidation scheduler that curates working memory into durable memory.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dexiask/memory/internal/config"
	"github.com/dexiask/memory/internal/consolidate"
	"github.com/dexiask/memory/internal/engineclient"
	"github.com/dexiask/memory/internal/httpapi"
	"github.com/dexiask/memory/internal/mcp"
	"github.com/dexiask/memory/internal/pivot"
)

func main() {
	cfg := config.Load()
	reg := pivot.Default()

	engine := engineclient.New(cfg.AgentURL)
	svc := consolidate.NewService(cfg, reg, engine)

	mcpHandler := mcp.NewHandler(cfg.Root, reg)
	api := httpapi.New(cfg.Root, reg, svc)
	handler := api.Handler(mcpHandler)

	srv := &http.Server{Addr: cfg.Addr(), Handler: handler}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Dream scheduler.
	scheduler := consolidate.NewScheduler(cfg, svc)
	go scheduler.Run(ctx)

	go func() {
		log.Printf("memory service listening on %s (root=%s)", cfg.Addr(), cfg.Root)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down memory service")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
		os.Exit(1)
	}
}
