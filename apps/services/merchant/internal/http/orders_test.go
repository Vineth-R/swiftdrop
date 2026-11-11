package servicehttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

func queryContext(rawQuery string) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/merchant/orders?"+rawQuery, nil)
	return context
}

func TestParseOrderListDefaults(t *testing.T) {
	filter, issues := parseOrderList(queryContext(""), "merchant-1")
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if filter.Page != 1 || filter.PageSize != 20 || filter.Sort != "createdAt" || !filter.Desc || filter.MerchantID != "merchant-1" {
		t.Fatalf("filter = %+v", filter)
	}
}

func TestParseOrderListFilters(t *testing.T) {
	filter, issues := parseOrderList(queryContext("status=in_transit&from=2026-11-01&to=2026-11-05&externalOrderId=SZ-1&page=2&pageSize=50&sort=updatedAt:asc"), "merchant-1")
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if filter.Status != "IN_TRANSIT" || filter.ExternalOrderID != "SZ-1" || filter.Page != 2 || filter.PageSize != 50 || filter.Sort != "updatedAt" || filter.Desc {
		t.Fatalf("filter = %+v", filter)
	}
	if filter.From == nil || filter.To == nil || !filter.To.After(*filter.From) {
		t.Fatalf("date range = %+v", filter)
	}
}

func TestParseOrderListRejectsBadQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
		field string
	}{
		{"page", "page=0", "page"},
		{"pageSize", "pageSize=101", "pageSize"},
		{"status", "status=LOST", "status"},
		{"externalOrderId", "externalOrderId=" + longString(101), "externalOrderId"},
		{"from", "from=not-a-date", "from"},
		{"to", "to=not-a-date", "to"},
		{"date order", "from=2026-11-05&to=2026-11-01", "to"},
		{"sort field", "sort=password:asc", "sort"},
		{"sort direction", "sort=createdAt:sideways", "sort"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, issues := parseOrderList(queryContext(test.query), "merchant-1")
			found := false
			for _, issue := range issues {
				if issue.Field == test.field {
					found = true
				}
			}
			if !found {
				t.Fatalf("issues = %+v, want %s", issues, test.field)
			}
		})
	}
}

func longString(size int) string {
	buffer := make([]byte, size)
	for i := range buffer {
		buffer[i] = 'a'
	}
	return string(buffer)
}

func TestOrderResponseShapes(t *testing.T) {
	created := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	order := orders.Order{
		ID: "order-1", ExternalOrderID: "SZ-1", TrackingNumber: "SDN4T8W2LC6", Status: "IN_TRANSIT",
		Recipient:    orders.Contact{Name: "Amali", City: "Kandy"},
		Pickup:       orders.Contact{Name: "Shop", City: "Colombo"},
		WeightKG:     150,
		SizeCategory: "SMALL",
		Fee:          orders.Fee{Currency: "LKR", BaseFee: 45000, WeightCharge: 10000, Total: 55000},
		CreatedAt:    created, UpdatedAt: created,
	}
	item := orderItem(order)
	if item.OrderID != "order-1" || item.RecipientName != "Amali" || item.DropoffCity != "Kandy" || item.Total != 55000 {
		t.Fatalf("item = %+v", item)
	}
	create := orderCreateResponse(order)
	if create.OrderID != "order-1" || create.TrackingNumber != "SDN4T8W2LC6" || create.Fee.Total != 55000 {
		t.Fatalf("create = %+v", create)
	}
	detail := orderDetailResponse(orders.Detail{Order: order, Timeline: []orders.TimelineEvent{{Status: "CREATED", At: created}}})
	if detail.WeightKG != 150 || detail.SizeCategory != "SMALL" || len(detail.Timeline) != 1 {
		t.Fatalf("detail = %+v", detail)
	}
	raw, _ := json.Marshal(detail)
	if string(raw) != `{"orderId":"order-1","externalOrderId":"SZ-1","trackingNumber":"SDN4T8W2LC6","status":"IN_TRANSIT","recipientName":"Amali","dropoffCity":"Kandy","total":550.00,"currency":"LKR","createdAt":"2026-11-04T10:00:00Z","updatedAt":"2026-11-04T10:00:00Z","weightKg":1.50,"sizeCategory":"SMALL","description":null,"fee":{"currency":"LKR","baseFee":450.00,"weightCharge":100.00,"sizeSurcharge":0.00,"total":550.00},"timeline":[{"status":"CREATED","at":"2026-11-04T10:00:00Z"}]}` {
		t.Fatalf("detail json = %s", raw)
	}
}

func TestOrderErrorMapping(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"validation", &orders.Invalid{Issues: []validation.Issue{{Field: "recipient.phone", Issue: "must be a valid phone number"}}}, http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
		{"route", &orders.UpstreamInvalid{Code: "ROUTE_NOT_SERVICEABLE"}, http.StatusUnprocessableEntity, "ROUTE_NOT_SERVICEABLE"},
		{"weight", &orders.UpstreamInvalid{Code: "WEIGHT_NOT_PRICED"}, http.StatusUnprocessableEntity, "WEIGHT_NOT_PRICED"},
		{"unknown upstream", &orders.UpstreamInvalid{Code: "SOMETHING"}, http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
		{"duplicate", orders.ErrDuplicateOrder, http.StatusConflict, "DUPLICATE_ORDER"},
		{"missing", orders.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"zone", orders.ErrZoneUnavailable, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"},
		{"parcel", orders.ErrParcelUnavailable, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/merchant/orders", nil)
			handler := &merchantHandler{}
			handler.orderError(context, test.err)
			if recorder.Code != test.status || unitErrorCode(recorder.Body.Bytes()) != test.code {
				t.Fatalf("status=%d body=%s, want %d %s", recorder.Code, recorder.Body.Bytes(), test.status, test.code)
			}
		})
	}
}

func unitErrorCode(body []byte) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	return envelope.Error.Code
}
