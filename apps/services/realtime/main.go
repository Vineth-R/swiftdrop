package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/realtime/internal/config"
	"github.com/Vineth-R/swiftdrop/apps/services/realtime/internal/consumer"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/realtime/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/realtime/internal/live"
	"github.com/Vineth-R/swiftdrop/apps/services/realtime/internal/ws"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/metrics"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
	"github.com/Vineth-R/swiftdrop/packages/go-common/sessioncheck"
)

const serviceName = "realtime"

func main() {
	if err := run(); err != nil {
		logging.NewBootstrap(serviceName).Error("service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := serviceconfig.Load()
	if err != nil {
		return err
	}

	logger, err := logging.New(serviceName, config.AppEnv, config.LogLevel)
	if err != nil {
		return err
	}

	if config.AppEnv != "development" {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	router := servicehttp.New(logger)
	metricRegistry := metrics.NewRegistry()
	router.GET("/metrics", metrics.Handler(metricRegistry))

	var database *postgres.Pool
	var pinger postgres.Pinger
	databaseConfig, configErr := postgres.LoadConfigFromURL(config.DatabaseURL)
	if configErr != nil {
		return fmt.Errorf("configure database: %w", configErr)
	}
	if databaseConfig.Configured() {
		database, err = postgres.Open(ctx, databaseConfig)
		if err != nil {
			return fmt.Errorf("connect database: %w", err)
		}
		defer database.Close()
		pinger = database
		logger.Info("database connection established")
	} else {
		logger.Warn("DATABASE_URL is not set; readiness will report database down")
	}

	statusHealth := messaging.NewHealth()
	assignedHealth := messaging.NewHealth()

	router.GET("/health", postgres.HealthHandler())
	router.GET("/ready", servicehttp.ReadyHandler(pinger, postgres.DefaultReadyTimeout,
		servicehttp.Dependency{Name: "consumer.status_changed", Ready: statusHealth.Ready},
		servicehttp.Dependency{Name: "consumer.assigned", Ready: assignedHealth.Ready},
	))

	// The verifier stays an untyped nil without a secret: a nil
	// *auth.TokenManager inside the interface would not compare equal to nil.
	var verifier ws.Verifier
	if config.AccessSecret != "" {
		// The lifetime only matters for signing; this service only verifies.
		manager, err := auth.NewTokenManager(config.AccessSecret, 15*time.Minute)
		if err != nil {
			return fmt.Errorf("configure token verifier: %w", err)
		}
		verifier = manager
	} else {
		logger.Warn("JWT_ACCESS_SECRET is not set; every WebSocket client is anonymous and may only follow a single tracking number")
	}

	var sessionValidator ws.SessionValidator
	if verifier != nil {
		sessionClient, err := sessioncheck.NewClient(config.IdentityBaseURL, config.InternalToken, config.IdentityTimeout)
		if err != nil {
			return fmt.Errorf("configure identity session checker: %w", err)
		}
		if config.InternalTokenProblem != "" {
			logger.Warn(config.InternalTokenProblem + "; authenticated WebSocket connections will fail closed")
		}
		sessionValidator = sessioncheck.New(sessionClient)
	}

	hub := live.NewHub()
	router.GET("/ws", gin.WrapH(ws.NewHandler(ctx, ws.Options{
		Hub:                  hub,
		Verifier:             verifier,
		CookieName:           config.CookieName,
		OriginPatterns:       config.AllowedOrigins,
		Logger:               logger,
		SessionValidator:     sessionValidator,
		SessionCheckInterval: config.SessionCheckInterval,
	})))

	server := httpx.NewServer(httpx.ServerConfig{
		Addr:              config.Addr(),
		Handler:           metrics.NewHTTPHandler(metricRegistry, router),
		Logger:            logger,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       120 * time.Second,
	})

	connectionsGauge := metricRegistry.Gauge("swiftdrop_realtime_connections", "Open WebSocket connections.").With()
	statusConsumerGauge := metricRegistry.Gauge("swiftdrop_realtime_consumer_ready", "Whether a realtime broker consumer is attached.", "consumer")
	go sampleRealtimeMetrics(ctx, hub, statusHealth, assignedHealth, connectionsGauge, statusConsumerGauge)

	logger.Info("starting http service", "listen_addr", config.Addr(), "environment", config.AppEnv)
	logger.Info("starting event consumer",
		"exchange", config.Exchange,
		"queue", config.Queue,
		"routing_key", config.RoutingKey,
	)
	logger.Info("starting assignment event consumer",
		"exchange", config.Exchange,
		"queue", config.AssignedQueue,
		"routing_key", config.AssignedRoutingKey,
	)

	processedCounter := metricRegistry.Counter("swiftdrop_consumer_messages_processed_total", "Broker messages processed successfully.", "queue")
	failedCounter := metricRegistry.Counter("swiftdrop_consumer_messages_failed_total", "Broker messages that could not be processed.", "queue", "class")
	retriedCounter := metricRegistry.Counter("swiftdrop_consumer_messages_retried_total", "Broker messages scheduled for retry.", "queue")
	processingDuration := metricRegistry.Histogram("swiftdrop_consumer_processing_duration_seconds", "Broker message handling duration in seconds.", "queue")
	observer := &messaging.Observer{
		Processed: func(queue string, duration time.Duration) {
			processedCounter.With(queue).Inc()
			processingDuration.With(queue).Observe(duration.Seconds())
		},
		Failed: func(queue string, permanent bool) {
			class := "retryable"
			if permanent {
				class = "permanent"
			}
			failedCounter.With(queue, class).Inc()
		},
		Retried: func(queue string) { retriedCounter.With(queue).Inc() },
	}

	consumerConfig := messaging.ConsumerConfig{
		URL:            config.RabbitMQURL,
		Exchange:       config.Exchange,
		Queue:          config.Queue,
		RoutingKey:     config.RoutingKey,
		Prefetch:       config.Prefetch,
		HandlerTimeout: config.HandlerTimeout,
		MaxRetries:     config.MaxRetries,
		RetryDelay:     config.RetryDelay,
		ReconnectMin:   config.ReconnectMin,
		ReconnectMax:   config.ReconnectMax,
		Health:         statusHealth,
		Observer:       observer,
	}
	assignedConfig := consumerConfig
	assignedConfig.Queue = config.AssignedQueue
	assignedConfig.RoutingKey = config.AssignedRoutingKey
	assignedConfig.Health = assignedHealth
	assignedConfig.Observer = observer

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan error, 3)
	go func() { results <- httpx.Serve(runCtx, server, 10*time.Second) }()
	go func() { results <- messaging.Run(runCtx, consumerConfig, consumer.New(hub), logger) }()
	go func() {
		results <- messaging.Run(runCtx, assignedConfig, consumer.NewAssignment(hub), logger)
	}()

	// Whichever stops first takes the others down with it.
	firstErr := <-results
	cancel()
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// sampleRealtimeMetrics publishes a small, fixed-cardinality health snapshot.
func sampleRealtimeMetrics(ctx context.Context, hub *live.Hub, statusHealth, assignedHealth *messaging.Health, connections *metrics.GaugeSeries, consumers *metrics.Gauge) {
	sample := func() {
		connections.Set(float64(hub.Connections()))
		consumers.With("status_changed").Set(boolValue(statusHealth.Ready()))
		consumers.With("assigned").Set(boolValue(assignedHealth.Ready()))
	}
	sample()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sample()
		}
	}
}

func boolValue(ok bool) float64 {
	if ok {
		return 1
	}
	return 0
}
