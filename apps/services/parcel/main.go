package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/config"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/dispatch"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/outbox"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/parcels"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/reports"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/tracking"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/metrics"
	"github.com/Vineth-R/swiftdrop/packages/go-common/objectstore"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
	"github.com/Vineth-R/swiftdrop/packages/go-common/sessioncheck"
)

const (
	serviceName          = "parcel"
	relayShutdownTimeout = 10 * time.Second
)

// waitForRelay waits for the relay goroutine to stop, bounded by timeout so a
// stuck broker publish cannot block process shutdown forever.
func waitForRelay(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

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

	metricRegistry := metrics.NewRegistry()
	outboxPublished := metricRegistry.Counter("swiftdrop_outbox_publishes_total", "Outbox publish attempts by result.", "result")

	if config.AppEnv != "development" {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The relay gets its own context. It is cancelled only after the HTTP
	// server finishes draining, so an in-flight publish can complete and mark
	// its outbox row PUBLISHED before the database and broker are closed.
	relayCtx, stopRelay := context.WithCancel(context.Background())
	defer stopRelay()
	var relayWG sync.WaitGroup

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
	options := servicehttp.Options{
		TrustedProxies: config.TrustedProxies,
		AccessSecret:   config.AccessSecret,
		CookieName:     config.CookieName,
		InternalToken:  config.InternalToken,
	}
	if config.AccessSecret == "" {
		logger.Warn("JWT_ACCESS_SECRET is not set; authenticated APIs, reports and courier delivery APIs will return 503")
	}
	if config.InternalTokenProblem != "" {
		logger.Warn(config.InternalTokenProblem)
	}
	var courierClient identityclient.CourierClient
	if config.IdentityBaseURL != "" {
		client, err := identityclient.New(config.IdentityBaseURL, config.InternalToken, config.IdentityTimeout)
		if err != nil {
			logger.Warn("identity client configuration error", "error", err)
		} else {
			courierClient = client
		}
	}

	// Authenticated routes re-check the token against identity so a suspension
	// or logout takes effect within the cache TTL instead of at token expiry.
	sessionClient, err := sessioncheck.NewClient(config.IdentityBaseURL, config.InternalToken, config.IdentityTimeout)
	if err != nil {
		return fmt.Errorf("configure identity session checker: %w", err)
	}
	options.SessionCheck = sessioncheck.New(sessionClient)

	if database != nil {
		courierRepository := courier.NewRepository(database)
		options.Courier = courierRepository
		options.CourierStatus = courierRepository
		options.ProofMaxSizeBytes = config.ProofMaxSizeBytes
		if !config.Storage.Enabled() {
			logger.Warn("object storage is not configured; courier proof uploads and downloads will return 503")
		} else {
			proofStorage, storageErr := objectstore.New(ctx, config.Storage)
			if storageErr != nil {
				return fmt.Errorf("configure courier proof storage: %w", storageErr)
			}
			defer func() {
				if closeErr := proofStorage.Close(); closeErr != nil {
					logger.Warn("courier proof storage did not close cleanly")
				}
			}()
			options.CourierProof = courier.NewProofService(database, proofStorage, config.ProofMaxPerParcel).
				WithMaxPhotoBytes(config.ProofMaxSizeBytes)
			options.CourierProofPhoto = courier.NewProofPhotoService(database, proofStorage, config.Storage.PresignTTL)
		}
		options.Tracking = tracking.NewRepository(database)
		options.Reports = reports.NewRepository(database)
		pricingClient := parcels.NewPricingClient(config.PricingBaseURL, config.InternalToken, &http.Client{
			Timeout: config.PricingTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		})
		options.Parcels = parcels.NewService(parcels.NewStore(database), pricingClient)
		options.Parcels.WithCourierReader(parcels.NewCourierClient(config.IdentityBaseURL, config.InternalToken, &http.Client{
			Timeout: config.IdentityTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}))

		dispatchRepo := dispatch.NewRepository(database)
		dispatchService := dispatch.NewService(dispatchRepo, courierClient)
		options.Dispatch = dispatchService

		outboxStore := outbox.NewSQLStore(database)
		var publisher outbox.Publisher
		if config.RabbitMQURL != "" {
			amqpPub := outbox.NewAMQPPublisher(config.RabbitMQURL, config.RabbitMQExchange)
			defer amqpPub.Close()
			publisher = observedPublisher{next: amqpPub, published: outboxPublished.With("success"), failed: outboxPublished.With("failure")}
		} else {
			logger.Warn("RABBITMQ_URL is not set; outbox events will remain pending")
		}
		relayOptions := outbox.DefaultRelayOptions()
		relayOptions.Exchange = config.RabbitMQExchange
		relay := outbox.NewRelay(
			outboxStore,
			publisher,
			logger,
			relayOptions,
		)
		relayWG.Add(1)
		go func() {
			defer relayWG.Done()
			relay.Run(relayCtx)
		}()
		// Registered after the database and publisher cleanup defers, so it
		// runs first: stop the relay and wait for it before those resources
		// are closed.
		defer func() {
			stopRelay()
			if !waitForRelay(&relayWG, relayShutdownTimeout) {
				logger.Warn("outbox relay did not stop within the shutdown timeout")
			}
		}()

		pendingGauge := metricRegistry.Gauge("swiftdrop_outbox_pending_events", "Outbox events still waiting to publish.").With()
		failedGauge := metricRegistry.Gauge("swiftdrop_outbox_failed_events", "Outbox events that exhausted publication attempts.").With()
		ageGauge := metricRegistry.Gauge("swiftdrop_outbox_oldest_pending_seconds", "Age of the oldest pending outbox event in seconds.").With()
		go pollOutboxMetrics(ctx, database, pendingGauge, failedGauge, ageGauge, logger)
	}
	router, err := servicehttp.New(logger, options)
	if err != nil {
		return fmt.Errorf("configure parcel router: %w", err)
	}

	router.GET("/metrics", metrics.Handler(metricRegistry))
	router.GET("/health", postgres.HealthHandler())
	router.GET("/ready", postgres.ReadyHandler(pinger, postgres.DefaultReadyTimeout))

	server := httpx.NewServer(httpx.ServerConfig{
		Addr:              config.Addr(),
		Handler:           metrics.NewHTTPHandler(metricRegistry, router),
		Logger:            logger,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	})

	logger.Info("starting http service", "listen_addr", config.Addr(), "environment", config.AppEnv)
	return httpx.Serve(ctx, server, 10*time.Second)
}

// observedPublisher records outbox publication outcomes without changing the
// relay's behaviour.
type observedPublisher struct {
	next      outbox.Publisher
	published *metrics.CounterSeries
	failed    *metrics.CounterSeries
}

func (p observedPublisher) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	err := p.next.Publish(ctx, exchange, routingKey, body)
	if err != nil {
		p.failed.Inc()
		return err
	}
	p.published.Inc()
	return nil
}

// pollOutboxMetrics samples outbox backlog every 15 seconds. A failure is
// logged at debug level and leaves the previous value in place; metrics must
// never interfere with publishing.
func pollOutboxMetrics(ctx context.Context, database *postgres.Pool, pending, failed, age *metrics.GaugeSeries, logger *slog.Logger) {
	sample := func() {
		queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		var pendingCount, failedCount int64
		var oldestSeconds float64
		err := database.QueryRow(queryCtx, `
			SELECT
				COALESCE(SUM(CASE WHEN status = 'PENDING' THEN 1 ELSE 0 END), 0),
				COALESCE(SUM(CASE WHEN status = 'FAILED' THEN 1 ELSE 0 END), 0),
				COALESCE(EXTRACT(EPOCH FROM now() - MIN(created_at) FILTER (WHERE status = 'PENDING'))::float8, 0)
			FROM parcel.outbox_events`).Scan(&pendingCount, &failedCount, &oldestSeconds)
		if err != nil {
			logger.Debug("outbox metrics sample failed", "error", err)
			return
		}
		pending.Set(float64(pendingCount))
		failed.Set(float64(failedCount))
		age.Set(oldestSeconds)
	}

	sample()
	ticker := time.NewTicker(15 * time.Second)
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
