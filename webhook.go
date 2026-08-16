package bonum

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Webhook types (Webhook.Type).
const (
	WebhookPayment             = "PAYMENT"              // checkout invoice paid/expired/cancelled
	WebhookCardToken           = "CARD-TOKEN"           // card tokenized (or failed)
	WebhookTokenPayment        = "TOKEN-PAYMENT"        // queued Purchase completed
	WebhookSubscriptionPayment = "SUBSCRIPTION-PAYMENT" // automatic billing result
	WebhookUnsubscribed        = "UNSUBSCRIBED"         // billing retries exhausted, subscription cancelled
)

// ChecksumHeader carries hex(HMAC-SHA256(MERCHANT_CHECKSUM_KEY, body)).
const ChecksumHeader = "x-checksum-v2"

// ErrChecksum means the webhook body does not match its x-checksum-v2 header.
var ErrChecksum = errors.New("bonum: webhook checksum mismatch")

// Webhook is a message Bonum POSTs to the merchant's registered URL.
type Webhook struct {
	Type    string      `json:"type"`
	Status  string      `json:"status"`  // SUCCESS or FAILED
	Message string      `json:"message"` // free text (error description); do not branch on it
	Body    WebhookBody `json:"body"`
}

// WebhookBody is the union of all webhook payloads; which fields are set
// depends on Webhook.Type.
type WebhookBody struct {
	TransactionID string `json:"transactionId"` // your ID from the originating request

	// PAYMENT, TOKEN-PAYMENT, SUBSCRIPTION-PAYMENT
	InvoiceID     Text         `json:"invoiceId"` // hex string for checkout invoices, number for subscriptions
	Amount        float64      `json:"amount"`
	Currency      string       `json:"currency"`
	CompletedAt   string       `json:"completedAt"`
	UpdatedAt     Text         `json:"updatedAt"` // datetime string or epoch millis
	TerminalID    Text         `json:"terminalId"`
	Status        string       `json:"status"`        // PAID, EXPIRED, CANCELLED, ERROR
	InvoiceStatus string       `json:"invoiceStatus"` // EXPIRED, ...
	PaymentVendor string       `json:"paymentVendor"` // QPAY, E_COMMERCE, ...
	InitType      string       `json:"initType"`
	RespCode      string       `json:"respCode"` // card network response code on failure
	PaymentApp    *PaymentApp  `json:"paymentApp"`
	ExtrasInputs  []ExtraValue `json:"extras-inputs"` // customer answers to InvoiceRequest.Extras
	Extras        []ExtraValue `json:"extras"`

	// CARD-TOKEN (also PAYMENT when paid with a token)
	Token         string            `json:"token"`
	Mask          string            `json:"mask"`
	Expiry        string            `json:"expiry"` // "2026/11"
	Bank          *Bank             `json:"bank"`
	Amounts       []Money           `json:"amounts"`
	Subscriptions []SubscriptionRef `json:"subscriptions"`
	CardStatus    string            `json:"cardStatus"` // CANCELLED on failure

	// SUBSCRIPTION-PAYMENT, UNSUBSCRIBED
	SubscriptionID int64 `json:"subscriptionId"`
	PlanID         int64 `json:"planId"`
}

type ExtraValue struct {
	ID          int64  `json:"id"`
	ExtraID     int64  `json:"extraId"`
	Placeholder string `json:"placeholder"`
	Value       string `json:"value"`
}

type PaymentApp struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	LogoURL string `json:"logoUrl"`
}

type Bank struct {
	ID           int64  `json:"id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Icon         string `json:"icon"`
	IBANCode     string `json:"iBanCode"`
	TransferCode string `json:"transferCode"`
}

type Money struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type SubscriptionRef struct {
	SubscriptionID  int64  `json:"subscriptionId"`
	PlanID          int64  `json:"planId"`
	NextBillingDate string `json:"nextBillingDate"`
}

// ReadWebhook validates and decodes an incoming webhook request using your
// MERCHANT_CHECKSUM_KEY. Reply 200 once you have stored the result.
func ReadWebhook(r *http.Request, checksumKey string) (*Webhook, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return ParseWebhook(body, r.Header.Get(ChecksumHeader), checksumKey)
}

// ParseWebhook validates body against the x-checksum-v2 header value and
// decodes it. Use it when your framework has already consumed the body.
func ParseWebhook(body []byte, checksum, checksumKey string) (*Webhook, error) {
	if !ValidChecksum(body, checksum, checksumKey) {
		return nil, ErrChecksum
	}
	var w Webhook
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

// Checksum returns hex(HMAC-SHA256(key, body)).
func Checksum(body []byte, key string) string {
	m := hmac.New(sha256.New, []byte(key))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

// ValidChecksum reports whether checksum matches body. Bonum signs the
// compact re-serialized JSON, so the raw body and its whitespace-stripped
// form are both accepted.
func ValidChecksum(body []byte, checksum, key string) bool {
	want := []byte(checksum)
	if hmac.Equal([]byte(Checksum(body, key)), want) {
		return true
	}
	var buf bytes.Buffer
	return json.Compact(&buf, body) == nil && hmac.Equal([]byte(Checksum(buf.Bytes(), key)), want)
}
