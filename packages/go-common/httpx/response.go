package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type HealthResponse struct {
	Status string `json:"status"`
}

func JSON(c *gin.Context, status int, payload any) {
	c.JSON(status, payload)
}

func Error(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{
		Error: ErrorDetail{Code: code, Message: message},
	})
}

func Healthz(c *gin.Context) {
	JSON(c, http.StatusOK, HealthResponse{Status: "ok"})
}

func Readyz(c *gin.Context) {
	JSON(c, http.StatusOK, HealthResponse{Status: "ready"})
}
