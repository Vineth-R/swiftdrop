package servicehttp

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

type merchantAuditItem struct {
	ID          string  `json:"id"`
	ActorUserID string  `json:"actorUserId"`
	Action      string  `json:"action"`
	EntityType  string  `json:"entityType"`
	EntityID    *string `json:"entityId"`
	Details     any     `json:"details"`
	CreatedAt   string  `json:"createdAt"`
}

func (h *merchantHandler) listAuditLogs(c *gin.Context) {
	if h.repository == nil {
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "merchant persistence is not configured")
		return
	}
	filter, ok := parseMerchantAuditQuery(c)
	if !ok {
		return
	}
	rows, total, err := h.repository.ListAuditLogs(c.Request.Context(), filter)
	if err != nil {
		h.internalError(c, "list_audit_logs", "audit logs could not be read", err)
		return
	}
	items := make([]merchantAuditItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, merchantAuditItem{
			ID: row.ID, ActorUserID: row.ActorUserID, Action: row.Action,
			EntityType: row.EntityType, EntityID: row.EntityID,
			Details:   sanitizeMerchantAuditDetails(row.Details),
			CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	page := filter.Offset/filter.Limit + 1
	if filter.Limit <= 0 {
		page = 1
	}
	httpx.SuccessList(c, http.StatusOK, items, httpx.NewPagination(page, filter.Limit, total))
}

func parseMerchantAuditQuery(c *gin.Context) (repository.AuditLogFilter, bool) {
	issues := validation.New()
	page, pageSize := 1, 20
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			issues.Add("page", "must be a positive integer")
		} else {
			page = value
		}
	}
	if raw := strings.TrimSpace(c.Query("pageSize")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			issues.Add("pageSize", "must be between 1 and 100")
		} else {
			pageSize = value
		}
	}
	parseDay := func(field, raw string) *time.Time {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil
		}
		value, err := time.Parse("2006-01-02", raw)
		if err != nil {
			issues.Add(field, "must be a date in YYYY-MM-DD format")
			return nil
		}
		return &value
	}
	from := parseDay("from", c.Query("from"))
	to := parseDay("to", c.Query("to"))
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
		return repository.AuditLogFilter{}, false
	}
	filter := repository.AuditLogFilter{
		ActorUserID: actor, Action: action, EntityType: entityType, EntityID: entity,
		From: from, Limit: pageSize, Offset: (page - 1) * pageSize, Ascending: sort == "createdAt:asc",
	}
	if to != nil {
		end := to.AddDate(0, 0, 1)
		filter.To = &end
	}
	return filter, true
}

func sanitizeMerchantAuditDetails(raw []byte) any {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return map[string]any{}
	}
	return redactMerchantAuditValue(value, 0)
}

func redactMerchantAuditValue(value any, depth int) any {
	if depth > 8 {
		return "[truncated]"
	}
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if merchantAuditKeyIsSensitive(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = redactMerchantAuditValue(item, depth+1)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactMerchantAuditValue(item, depth+1))
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

func merchantAuditKeyIsSensitive(key string) bool {
	lower := strings.ToLower(key)
	for _, fragment := range []string{"password", "token", "secret", "credential", "authorization", "cookie", "api_key", "apikey"} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}
