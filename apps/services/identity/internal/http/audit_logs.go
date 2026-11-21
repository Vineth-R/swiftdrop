package servicehttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

type auditLogItem struct {
	ID          string  `json:"id"`
	ActorUserID string  `json:"actorUserId"`
	Action      string  `json:"action"`
	EntityType  string  `json:"entityType"`
	EntityID    *string `json:"entityId"`
	Details     any     `json:"details"`
	CreatedAt   string  `json:"createdAt"`
}

// listAuditLogs handles GET /admin/audit-logs/identity. Audit data is
// read-only and ADMIN-only; the response never includes a password, token,
// secret or credential value.
func (h *adminHandlers) listAuditLogs(c *gin.Context) {
	input, ok := parseAuditLogQuery(c)
	if !ok {
		return
	}
	page, err := h.service.ListAuditLogs(c.Request.Context(), input)
	if err != nil {
		var invalid *service.ValidationError
		if errors.As(err, &invalid) {
			httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "one or more query parameters are invalid", httpx.ErrorItems(invalid.Issues))
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "audit log read failed", "error", err)
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "audit logs could not be read")
		return
	}

	items := make([]auditLogItem, 0, len(page.Entries))
	for _, entry := range page.Entries {
		items = append(items, auditLogItem{
			ID: entry.ID, ActorUserID: entry.ActorUserID, Action: entry.Action,
			EntityType: entry.EntityType, EntityID: entry.EntityID,
			Details:   sanitizeAuditDetails(entry.Details),
			CreatedAt: entry.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	httpx.SuccessList(c, http.StatusOK, items, httpx.NewPagination(page.Page, page.PageSize, page.TotalItems))
}

func parseAuditLogQuery(c *gin.Context) (service.ListAuditLogsInput, bool) {
	issues := validation.New()
	page, pageSize := 1, 20
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 1 {
			issues.Add("page", "must be a positive integer")
		} else {
			page = value
		}
	}
	if raw := strings.TrimSpace(c.Query("pageSize")); raw != "" {
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 1 || value > 100 {
			issues.Add("pageSize", "must be between 1 and 100")
		} else {
			pageSize = value
		}
	}

	var from, to *time.Time
	parseDay := func(field, raw string) *time.Time {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil
		}
		value, parseErr := time.Parse("2006-01-02", raw)
		if parseErr != nil {
			issues.Add(field, "must be a date in YYYY-MM-DD format")
			return nil
		}
		return &value
	}
	from = parseDay("from", c.Query("from"))
	to = parseDay("to", c.Query("to"))
	if from != nil && to != nil && from.After(*to) {
		issues.Add("to", "must not be before from")
	}

	actor := strings.TrimSpace(c.Query("actorUserId"))
	entity := strings.TrimSpace(c.Query("entityId"))
	if actor != "" && !validation.UUID(actor) {
		issues.Add("actorUserId", "must be a UUID")
	}
	if entity != "" && !validation.UUID(entity) {
		issues.Add("entityId", "must be a UUID")
	}
	action := strings.TrimSpace(c.Query("action"))
	if validation.RuneLength(action) > 60 {
		issues.Add("action", "must be at most 60 characters")
	}
	entityType := strings.TrimSpace(c.Query("entityType"))
	if validation.RuneLength(entityType) > 40 {
		issues.Add("entityType", "must be at most 40 characters")
	}
	sort := strings.TrimSpace(c.Query("sort"))
	if sort != "" && sort != "createdAt:asc" && sort != "createdAt:desc" {
		issues.Add("sort", "must be createdAt:asc or createdAt:desc")
	}
	if !issues.Empty() {
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "one or more query parameters are invalid", httpx.ErrorItems(issues.Issues()))
		return service.ListAuditLogsInput{}, false
	}

	input := service.ListAuditLogsInput{
		ActorUserID: actor,
		Action:      action,
		EntityType:  entityType,
		EntityID:    entity,
		Sort:        sort,
		Page:        page,
		PageSize:    pageSize,
	}
	if from != nil {
		input.From = from
	}
	if to != nil {
		end := to.AddDate(0, 0, 1)
		input.To = &end
	}
	return input, true
}

// sanitizeAuditDetails removes credential-like keys and bounds the rendered
// value. The stored rows are already controlled by this service, but the read
// API is defensive: a future audit writer must not be able to leak a secret
// through the ADMIN screen.
func sanitizeAuditDetails(raw []byte) any {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return map[string]any{}
	}
	return redactAuditValue(value, 0)
}

func redactAuditValue(value any, depth int) any {
	if depth > 8 {
		return "[truncated]"
	}
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if auditKeyIsSensitive(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = redactAuditValue(item, depth+1)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactAuditValue(item, depth+1))
		}
		return out
	case string:
		if len(typed) > 2000 {
			return typed[:2000] + "…"
		}
		return typed
	default:
		return typed
	}
}

func auditKeyIsSensitive(key string) bool {
	lower := strings.ToLower(key)
	for _, fragment := range []string{"password", "token", "secret", "credential", "authorization", "cookie", "api_key", "apikey"} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}
