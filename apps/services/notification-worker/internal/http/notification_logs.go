package servicehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/notificationlog"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPage         = 10000
)

// NotificationLogStore is the read side the admin handler calls.
// *notificationlog.Repository satisfies it.
type NotificationLogStore interface {
	List(ctx context.Context, filter notificationlog.Filter) ([]notificationlog.Log, int, error)
}

type notificationLogHandlers struct {
	store  NotificationLogStore
	logger *slog.Logger
}

type notificationLogItem struct {
	ID           string  `json:"id"`
	EventID      string  `json:"eventId"`
	ParcelID     string  `json:"parcelId"`
	Channel      string  `json:"channel"`
	Recipient    string  `json:"recipient"`
	Subject      *string `json:"subject"`
	Status       string  `json:"status"`
	ErrorMessage *string `json:"errorMessage"`
	CreatedAt    string  `json:"createdAt"`
	SentAt       *string `json:"sentAt"`
}

func registerNotificationLogs(router *gin.Engine, logger *slog.Logger, options Options) error {
	store := options.Store
	if store == nil {
		store = notificationlog.NewRepository(nil)
	}
	handler := &notificationLogHandlers{store: store, logger: logger}

	cookie := options.CookieName
	if cookie == "" {
		cookie = "sd_access"
	}

	admin := router.Group("/admin/notification-logs", noStore())
	if options.AccessSecret == "" {
		admin.Use(func(c *gin.Context) {
			httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "authentication is not configured")
		})
	} else {
		manager, err := auth.NewTokenManager(options.AccessSecret, time.Minute)
		if err != nil {
			return err
		}
		admin.Use(auth.Authenticate(manager, cookie), auth.RequireRoles(auth.RoleAdmin))
	}
	for _, path := range []string{"", "/"} {
		admin.GET(path, handler.list)
	}
	return nil
}

// noStore prevents browsers and intermediaries from caching ADMIN responses,
// including authentication and validation failures.
func noStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

// list handles GET /admin/notification-logs.
func (h *notificationLogHandlers) list(c *gin.Context) {
	issues := validation.New()
	page, pageSize := parsePagination(c, issues)
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && !notificationlog.ValidStatus(status) {
		issues.Add("status", "must be one of PENDING, SENT, FAILED")
	}
	if !issues.Empty() {
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more fields are invalid.", httpx.ErrorItems(issues.Issues()))
		return
	}

	logs, total, err := h.store.List(c.Request.Context(), notificationlog.Filter{
		Status: status,
		Limit:  pageSize,
		Offset: (page - 1) * pageSize,
	})
	switch {
	case errors.Is(err, notificationlog.ErrUnavailable):
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "notification logs are unavailable")
		return
	case errors.Is(err, notificationlog.ErrInvalidFilter):
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more fields are invalid.",
			[]httpx.ErrorItem{{Field: "pagination", Issue: "invalid pagination bounds"}})
		return
	case err != nil:
		// The repository discards driver text; log only a stable operation name.
		h.logger.ErrorContext(c.Request.Context(), "list notification logs failed",
			"request_id", requestid.FromGin(c),
			"operation", "list_notification_logs",
		)
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "notification logs could not be listed")
		return
	}

	items := make([]notificationLogItem, 0, len(logs))
	for _, entry := range logs {
		var sentAt *string
		if entry.SentAt != nil {
			formatted := entry.SentAt.UTC().Format(time.RFC3339)
			sentAt = &formatted
		}
		items = append(items, notificationLogItem{
			ID:           entry.ID,
			EventID:      entry.EventID,
			ParcelID:     entry.ParcelID,
			Channel:      entry.Channel,
			Recipient:    notificationlog.MaskRecipient(entry.Channel, entry.Recipient),
			Subject:      entry.Subject,
			Status:       entry.Status,
			ErrorMessage: entry.ErrorMessage,
			CreatedAt:    entry.CreatedAt.UTC().Format(time.RFC3339),
			SentAt:       sentAt,
		})
	}

	httpx.SuccessList(c, http.StatusOK, items, httpx.NewPagination(page, pageSize, total))
}

func parsePagination(c *gin.Context, issues *validation.Collector) (int, int) {
	page := defaultPage
	pageSize := defaultPageSize

	if raw, exists := c.GetQuery("page"); exists {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxPage {
			issues.Add("page", "must be between 1 and 10000")
		} else {
			page = value
		}
	}
	if raw, exists := c.GetQuery("pageSize"); exists {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > notificationlog.MaxPageSize {
			issues.Add("pageSize", "must be between 1 and 100")
		} else {
			pageSize = value
		}
	}
	return page, pageSize
}
