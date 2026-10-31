package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
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

func Healthz(c *gin.Context) {
	JSON(c, http.StatusOK, HealthResponse{Status: "ok"})
}

func Readyz(c *gin.Context) {
	JSON(c, http.StatusOK, HealthResponse{Status: "ready"})
}
