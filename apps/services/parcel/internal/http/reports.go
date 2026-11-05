package servicehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/reports"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

// ReportStore is the read side the report handlers call.
// *reports.Repository satisfies it.
type ReportStore interface {
	Daily(context.Context, reports.Range) ([]reports.DailyRow, error)
	Outcomes(context.Context, reports.Range) (reports.Outcomes, error)
}

type reportHandlers struct {
	store  ReportStore
	logger *slog.Logger
}

// registerReports adds GET /reports/daily and GET /reports/outcomes (API v1.1
// section 7). Both are for DISPATCHER and ADMIN (ADMIN passes every DISPATCHER
// check): the access token is checked first, then the role. Without a configured
// access secret they answer 503 rather than run open.
func registerReports(router *gin.Engine, logger *slog.Logger, options Options) error {
	store := options.Reports
	if store == nil {
		store = reports.NewRepository(nil)
	}
	handler := &reportHandlers{store: store, logger: logger}

	cookie := options.CookieName
	if cookie == "" {
		cookie = "sd_access"
	}

	group := router.Group("/reports", noStore())
	if options.AccessSecret == "" {
		group.Use(func(c *gin.Context) {
			httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "authentication is not configured")
			c.Abort()
		})
	} else {
		manager, err := auth.NewTokenManager(options.AccessSecret, time.Minute)
		if err != nil {
			return err
		}
		group.Use(auth.Authenticate(manager, cookie), auth.RequireRoles(auth.RoleDispatcher))
	}
	group.GET("/daily", handler.daily)
	group.GET("/outcomes", handler.outcomes)
	return nil
}

// noStore keeps browsers and proxies from caching ADMIN responses, including errors.
func noStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

// daily handles GET /reports/daily.
func (h *reportHandlers) daily(c *gin.Context) {
	rng, ok := parseReportRange(c, reports.DefaultDailyDays)
	if !ok {
		return
	}
	rows, err := h.store.Daily(c.Request.Context(), rng)
	if h.failed(c, "daily_report", err) {
		return
	}
	respondReport(c, rows)
}

// outcomes handles GET /reports/outcomes.
func (h *reportHandlers) outcomes(c *gin.Context) {
	rng, ok := parseReportRange(c, reports.DefaultOutcomeDays)
	if !ok {
		return
	}
	result, err := h.store.Outcomes(c.Request.Context(), rng)
	if h.failed(c, "outcomes_report", err) {
		return
	}
	respondReport(c, result)
}

func parseReportRange(c *gin.Context, defaultDays int) (reports.Range, bool) {
	issues := validation.New()
	rng, ok := reports.ParseRange(issues, c.Query("from"), c.Query("to"), defaultDays, time.Now())
	if !ok {
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more fields are invalid.", httpx.ErrorItems(issues.Issues()))
		return reports.Range{}, false
	}
	return rng, true
}

// failed answers the request and returns true when err is not nil.
func (h *reportHandlers) failed(c *gin.Context, operation string, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, reports.ErrUnavailable):
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "reports are unavailable")
	default:
		// Log a stable operation name only, never driver text or query values.
		h.logger.ErrorContext(c.Request.Context(), "report failed",
			"request_id", requestid.FromGin(c),
			"operation", operation,
		)
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "the report could not be produced")
	}
	return true
}

func respondReport(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"data": data,
		"meta": gin.H{"requestId": requestid.FromGin(c), "timestamp": time.Now().UTC()},
	})
}
