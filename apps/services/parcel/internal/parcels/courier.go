package parcels

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

type Courier struct {
	ID       *string `json:"id,omitempty"`
	FullName string  `json:"fullName"`
	Phone    string  `json:"phone"`
}
type CourierReader interface {
	Lookup(context.Context, string) (Courier, error)
}
type CourierClient struct {
	baseURL, token string
	client         *http.Client
}

func NewCourierClient(baseURL, token string, client *http.Client) *CourierClient {
	return &CourierClient{strings.TrimRight(baseURL, "/"), token, client}
}
func (c *CourierClient) Lookup(ctx context.Context, id string) (Courier, error) {
	if !validation.UUID(id) || c.client == nil || c.token == "" {
		return Courier{}, ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/internal/users/"+id+"/contact", nil)
	if err != nil {
		return Courier{}, ErrUnavailable
	}
	req.Header.Set("X-Internal-Token", c.token)
	if id := requestid.FromContext(ctx); id != "" {
		req.Header.Set(requestid.Header, id)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return Courier{}, ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		// The assigned user no longer exists. The parcel itself is still valid;
		// the caller decides whether to omit the courier or fail.
		return Courier{}, ErrCourierNotFound
	}
	if res.StatusCode != http.StatusOK {
		return Courier{}, ErrUnavailable
	}
	var envelope struct {
		Data Courier `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&envelope); err != nil || strings.TrimSpace(envelope.Data.FullName) == "" {
		return Courier{}, ErrUnavailable
	}
	envelope.Data.ID = nil
	return envelope.Data, nil
}
