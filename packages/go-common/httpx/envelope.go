package httpx

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

// SuccessMeta is the meta object of every success response (API v1.1 section 1.5).
type SuccessMeta struct {
	RequestID  string      `json:"requestId"`
	Timestamp  string      `json:"timestamp"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// SuccessResponse is the success envelope: data plus meta.
type SuccessResponse struct {
	Data any         `json:"data"`
	Meta SuccessMeta `json:"meta"`
}

// Pagination describes one page of a list response.
type Pagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
}

// NewPagination computes totalPages for a page of pageSize items out of
// totalItems. A non-positive pageSize yields zero pages instead of dividing by
// zero.
func NewPagination(page, pageSize, totalItems int) Pagination {
	totalPages := 0
	if pageSize > 0 {
		totalPages = (totalItems + pageSize - 1) / pageSize
	}
	return Pagination{Page: page, PageSize: pageSize, TotalItems: totalItems, TotalPages: totalPages}
}

// Success writes a success envelope with the request ID and a UTC timestamp.
func Success(c *gin.Context, status int, data any) {
	c.JSON(status, SuccessResponse{Data: data, Meta: successMeta(c, nil)})
}

// SuccessList writes a list envelope; data should be a slice, never nil.
func SuccessList(c *gin.Context, status int, data any, pagination Pagination) {
	c.JSON(status, SuccessResponse{Data: data, Meta: successMeta(c, &pagination)})
}

func successMeta(c *gin.Context, pagination *Pagination) SuccessMeta {
	return SuccessMeta{
		RequestID:  requestid.FromGin(c),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Pagination: pagination,
	}
}
