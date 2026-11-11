package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/client"
	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/config"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/conversation"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/knowledge"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/middleware"
	geminiprovider "github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/provider/gemini"
	mockprovider "github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/provider/mock"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/service"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/tools"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
)

const serviceName = "ai-assistant"

func main() {
	if err := run(); err != nil {
		logging.NewBootstrap(serviceName).Error("service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := serviceconfig.Load()
	if err != nil {
		return err
	}

	logger, err := logging.New(serviceName, cfg.AppEnv, cfg.LogLevel)
	if err != nil {
		return err
	}

	if cfg.AppEnv != "development" {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	base, err := knowledge.Load()
	if err != nil {
		return fmt.Errorf("load knowledge base: %w", err)
	}

	conversations := conversation.New(cfg.ConversationMax, cfg.ConversationTTL, cfg.ConversationMaxMessages)

	pricingClient, err := client.NewPricingClient(client.Config{BaseURL: cfg.PricingBaseURL, Timeout: cfg.PricingTimeout})
	if err != nil {
		return fmt.Errorf("configure pricing client: %w", err)
	}
	parcelClient, err := client.NewParcelClient(client.Config{BaseURL: cfg.ParcelBaseURL, Timeout: cfg.ParcelTimeout})
	if err != nil {
		return fmt.Errorf("configure parcel client: %w", err)
	}

	registry, err := tools.NewRegistry(pricingClient, parcelClient)
	if err != nil {
		return fmt.Errorf("configure tool registry: %w", err)
	}

	provider, providerStatus := buildProvider(ctx, cfg, logger)

	var verifier auth.Verifier
	if cfg.AccessSecret != "" {
		manager, err := auth.NewTokenManager(cfg.AccessSecret, time.Minute)
		if err != nil {
			return fmt.Errorf("configure access-token verifier: %w", err)
		}
		verifier = manager
	} else {
		logger.Warn("JWT_ACCESS_SECRET is not set; private parcel tools and requests carrying credentials will be unavailable")
	}

	orchestrator, err := service.New(service.Options{
		Provider:        provider,
		Tools:           registry,
		Conversations:   conversations,
		Knowledge:       base,
		Logger:          logger,
		Model:           cfg.Model,
		Temperature:     float32(cfg.Temperature),
		MaxOutputTokens: int32(cfg.MaxOutputTokens),
		RequestTimeout:  cfg.RequestTimeout,
		ToolTimeout:     cfg.ToolTimeout,
		TotalTimeout:    cfg.TotalTimeout,
		MaxToolRounds:   cfg.MaxToolRounds,
		MaxToolCalls:    cfg.MaxToolCalls,
		MaxHistory:      cfg.MaxHistoryItems,
	})
	if err != nil {
		return fmt.Errorf("configure orchestrator: %w", err)
	}

	router, err := servicehttp.New(logger, servicehttp.Options{
		Service:         orchestrator,
		Limiter:         middleware.NewLimiter(cfg.RateLimit, cfg.RateWindow, middleware.WithMaxConcurrent(cfg.MaxConcurrent)),
		Verifier:        verifier,
		CookieName:      cfg.CookieName,
		GuestCookieName: cfg.GuestCookieName,
		GuestSessionTTL: cfg.ConversationTTL,
		FrontendOrigin:  cfg.FrontendOrigin,
		TrustedProxies:  cfg.TrustedProxies,
		ProviderStatus:  providerStatus,
	})
	if err != nil {
		return fmt.Errorf("configure ai-assistant router: %w", err)
	}

	server := httpx.NewServer(httpx.ServerConfig{
		Addr:              cfg.Addr(),
		Handler:           router,
		Logger:            logger,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	})

	logger.Info("starting http service",
		"listen_addr", cfg.Addr(),
		"environment", cfg.AppEnv,
		"provider", cfg.Provider,
		"model", cfg.Model,
		"ai_enabled", cfg.AIEnabled,
		"provider_status", providerStatus,
	)
	return httpx.Serve(ctx, server, cfg.TotalTimeout+10*time.Second)
}

func buildProvider(ctx context.Context, cfg serviceconfig.Config, logger *slog.Logger) (contract.Provider, string) {
	if !cfg.AIEnabled {
		logger.Warn("AI_ENABLED is false; chat requests will return 503")
		return nil, "disabled"
	}

	switch cfg.Provider {
	case "mock":
		logger.Warn("AI_PROVIDER is mock; responses are scripted and no Gemini call is made")
		return mockprovider.New(), "ok"
	default:
		if cfg.GeminiAPIKey == "" {
			logger.Warn("GEMINI_API_KEY is not set; chat requests will return 503")
			return nil, "unconfigured"
		}
		provider, err := geminiprovider.New(ctx, geminiprovider.Config{
			APIKey:  cfg.GeminiAPIKey,
			Model:   cfg.Model,
			Timeout: cfg.RequestTimeout,
		})
		if err != nil {
			logger.Warn("Gemini provider could not be configured; chat requests will return 503", "error", err)
			return nil, "unconfigured"
		}
		return provider, "ok"
	}
}
