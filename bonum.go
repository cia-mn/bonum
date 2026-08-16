// Package bonum is a Go client for the Bonum payment gateway
// (https://psp.bonum.mn/bonum-gateway-apis.html): checkout invoices, card
// tokenization, token purchases, subscriptions, QR payments and webhook
// validation.
package bonum

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Base URLs. Pass one of them to New.
const (
	Sandbox    = "https://testapi.bonum.mn"
	Production = "https://apis.bonum.mn"
)

// tokens are renewed this long before the gateway expires them.
const skew = 30 * time.Second

// Client calls the Bonum gateway. Create one with New and reuse it: it caches
// the access token and renews it before expiry, which the gateway requires
// (token creation is rate limited). Safe for concurrent use.
type Client struct {
	// HTTPClient performs the requests. Defaults to a 30s-timeout client.
	HTTPClient *http.Client
	// Lang is sent as Accept-Language ("mn" or "en"). Defaults to "en".
	Lang string

	baseURL, appSecret, terminalID string

	mu   sync.Mutex
	tok  Token
	exp  time.Time // access token expiry
	rexp time.Time // refresh token expiry
}

// New returns a Client for baseURL (Sandbox or Production) using the
// APP_SECRET and terminal ID from the merchant portal.
func New(baseURL, appSecret, terminalID string) *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Lang:       "en",
		baseURL:    baseURL,
		appSecret:  appSecret,
		terminalID: terminalID,
	}
}

// Token is the gateway's auth response.
type Token struct {
	TokenType        string `json:"tokenType"`
	AccessToken      string `json:"accessToken"`
	ExpiresIn        int    `json:"expiresIn"` // seconds
	RefreshToken     string `json:"refreshToken"`
	RefreshExpiresIn int    `json:"refreshExpiresIn"` // seconds
	Unit             string `json:"unit"`
}

// AccessToken returns a valid access token, creating or refreshing one when
// needed. Do calls it for you; use it only to talk to the gateway directly.
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if now.Before(c.exp) {
		return c.tok.AccessToken, nil
	}
	var t Token
	err := fmt.Errorf("no refresh token")
	if now.Before(c.rexp) {
		err = c.send(ctx, "GET", "/bonum-gateway/ecommerce/auth/refresh",
			http.Header{"Authorization": {"Bearer " + c.tok.RefreshToken}}, nil, &t)
	}
	if err != nil {
		err = c.send(ctx, "GET", "/bonum-gateway/ecommerce/auth/create",
			http.Header{"Authorization": {"AppSecret " + c.appSecret}, "X-TERMINAL-ID": {c.terminalID}}, nil, &t)
	}
	if err != nil {
		return "", err
	}
	c.tok = t
	c.exp = now.Add(time.Duration(t.ExpiresIn)*time.Second - skew)
	c.rexp = now.Add(time.Duration(t.RefreshExpiresIn)*time.Second - skew)
	return t.AccessToken, nil
}

// Do sends an authenticated JSON request to path (e.g.
// "/mpay-service/merchant/subscriptions") and decodes the response into out
// (may be nil). It is the escape hatch for endpoints this package does not
// wrap. Non-2xx responses are returned as *Error.
func (c *Client) Do(ctx context.Context, method, path string, header http.Header, in, out any) error {
	tok, err := c.AccessToken(ctx)
	if err != nil {
		return err
	}
	if header == nil {
		header = http.Header{}
	}
	header.Set("Authorization", "Bearer "+tok)
	return c.send(ctx, method, path, header, in, out)
}

func (c *Client) send(ctx context.Context, method, path string, header http.Header, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", c.Lang)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		e := &Error{Body: b}
		_ = json.Unmarshal(b, e) // best effort; body may not be JSON
		e.Status = resp.StatusCode
		return e
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

// Error is a non-2xx gateway response.
type Error struct {
	Status    int             `json:"-"`         // HTTP status
	TraceID   string          `json:"traceId"`   // quote it to Bonum support
	ErrorCode string          `json:"errorCode"` // internal; do not branch on it (per docs)
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data"` // e.g. a PurchaseResult for a declined purchase
	Body      []byte          `json:"-"`    // raw response body
}

func (e *Error) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	if e.ErrorCode != "" {
		msg = e.ErrorCode + ": " + msg
	}
	if e.TraceID != "" {
		msg += " (trace " + e.TraceID + ")"
	}
	return fmt.Sprintf("bonum: %d %s", e.Status, msg)
}

// call and data keep every endpoint a one-liner: call decodes a bare JSON
// response, data unwraps the {"data": ...} envelope of /mpay-service replies.
func call[T any](c *Client, ctx context.Context, method, path string, h http.Header, in any) (T, error) {
	var out T
	err := c.Do(ctx, method, path, h, in, &out)
	return out, err
}

func data[T any](c *Client, ctx context.Context, method, path string, h http.Header, in any) (T, error) {
	r, err := call[struct {
		Data T `json:"data"`
	}](c, ctx, method, path, h, in)
	return r.Data, err
}

func card(token string) http.Header { return http.Header{"X-CARD-TOKEN": {token}} }
