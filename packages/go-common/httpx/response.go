package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

type ErrorItem struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

type ErrorDetail struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details []ErrorItem `json:"details,omitempty"`
}

type ResponseMeta struct {
	RequestID string `json:"requestId"`
}

type ErrorResponse struct {
	Error ErrorDetail  `json:"error"`
	Meta  ResponseMeta `json:"meta"`
}

type HealthResponse struct {
	Status string `json:"status"`
}

func JSON(c *gin.Context, status int, payload any) {
	c.JSON(status, payload)
}

func Error(c *gin.Context, status int, code, message string) {
	ErrorWithDetails(c, status, code, message, nil)
}

func ErrorWithDetails(c *gin.Context, status int, code, message string, details []ErrorItem) {
	c.AbortWithStatusJSON(status, ErrorResponse{
		Error: ErrorDetail{Code: code, Message: message, Details: details},
		Meta:  ResponseMeta{RequestID: requestid.FromGin(c)},
	})
}

// ErrorWithFields writes the standard error envelope and adds extra,
// API-specific fields to the error object. code, message and requestId cannot
// be overridden by fields. This keeps additive fields inside the documented
// shape instead of hand-writing a second error format.
func ErrorWithFields(c *gin.Context, status int, code, message string, fields map[string]any) {
	body := map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
		"meta": map[string]any{
			"requestId": requestid.FromGin(c),
		},
	}
	errorObject, _ := body["error"].(map[string]any)
	for key, value := range fields {
		switch key {
		case "code", "message":
			continue
		}
		errorObject[key] = value
	}
	c.AbortWithStatusJSON(status, body)
}

// ErrorItems converts shared validation issues into the API v1.1 details
// shape. It preserves the caller's order and returns nil for no issues.
func ErrorItems(issues []validation.Issue) []ErrorItem {
	if len(issues) == 0 {
		return nil
	}
	items := make([]ErrorItem, 0, len(issues))
	for _, issue := range issues {
		items = append(items, ErrorItem{Field: issue.Field, Issue: issue.Issue})
	}
	return items
}

func Healthz(c *gin.Context) {
	JSON(c, http.StatusOK, HealthResponse{Status: "ok"})
}

func Readyz(c *gin.Context) {
	JSON(c, http.StatusOK, HealthResponse{Status: "ready"})
}
