package bonum

import "encoding/json"

// Payment providers shown on the checkout page (InvoiceRequest.Providers).
const (
	QPay      = "QPAY"       // QR payment; every bank and fintech app
	ECommerce = "E_COMMERCE" // online card payment
	WeChat    = "WE_CHAT"    // WeChat wallet
	SonoShop  = "SONO_SHOP"  // buy now, pay later
)

// Statuses used across responses and webhooks.
const (
	StatusSuccess = "SUCCESS"
	StatusFailed  = "FAILED"
	StatusQueued  = "QUEUED" // purchase accepted; result comes via webhook
)

// Timestamps are strings in "2006-01-02 15:04:05" form (Ulaanbaatar time),
// amounts are MNT float64s, exactly as the gateway sends them.

type Provider struct {
	Provider string `json:"provider"`
	Enabled  bool   `json:"enabled"`
}

// Item is a line shown on the checkout/tokenization page.
type Item struct {
	Image  string  `json:"image,omitempty"`
	Title  string  `json:"title"`
	Remark string  `json:"remark,omitempty"`
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
}

// Extra is an input the customer fills in on the checkout page.
type Extra struct {
	Placeholder string `json:"placeholder"`
	Type        string `json:"type"` // NUMBER, PHONE, EMAIL, TEXT, ALL
	Required    bool   `json:"required"`
}

type InvoiceRequest struct {
	Amount        float64  `json:"amount"`
	Callback      string   `json:"callback"`            // browser is redirected here afterwards
	TransactionID string   `json:"transactionId"`       // your unique ID; echoed in the webhook
	ExpiresIn     int      `json:"expiresIn,omitempty"` // seconds
	Providers     []string `json:"providers,omitempty"` // restrict payment options
	Items         []Item   `json:"items,omitempty"`
	Extras        []Extra  `json:"extras,omitempty"`
}

type Invoice struct {
	InvoiceID    string `json:"invoiceId"`
	FollowUpLink string `json:"followUpLink"`
}

type CardTokenRequest struct {
	Callback      string            `json:"callback"`
	TransactionID string            `json:"transactionId"`
	Payment       *Payment          `json:"payment,omitempty"`      // charge while tokenizing (else 0.01 MNT verification)
	Subscription  *SubscribeRequest `json:"subscription,omitempty"` // subscribe the new token right away
	Items         []Item            `json:"items,omitempty"`
}

type Payment struct {
	Amount float64 `json:"amount"`
}

type Tokenization struct {
	ID           string `json:"id"`
	FollowUpLink string `json:"followUpLink"`
}

type PurchaseRequest struct {
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency,omitempty"` // default MNT
	TransactionID string  `json:"transactionId"`
}

type PurchaseResult struct {
	ID          int64  `json:"id"`
	CompletedAt string `json:"completedAt"`
	Status      string `json:"status"` // SUCCESS, FAILED, QUEUED
	Description string `json:"description"`
	CardStatus  string `json:"cardStatus"` // ACTIVE, INACTIVE
	RespCode    string `json:"respCode"`
}

type Plan struct {
	PlanID        int64   `json:"planId"`
	Name          string  `json:"name"`
	Remark        string  `json:"remark"`
	CreatedAt     string  `json:"createdAt"`
	RecurringType string  `json:"recurringType"` // WEEKLY, MONTHLY, YEARLY
	Amount        float64 `json:"amount"`
	Status        string  `json:"status"`
	CardCount     int     `json:"cardCount"`
	RetryCount    int     `json:"retryCount"`
}

type SubscribeRequest struct {
	PlanID     int64  `json:"planId"`
	CycleValue int    `json:"cycleValue,omitempty"` // billing day: 1-7 weekly (Mon-Sun), 1-31 monthly, 1-366 yearly
	Cycles     int    `json:"cycles,omitempty"`     // 0 = until cancelled
	PayNow     bool   `json:"payNow,omitempty"`     // bill today, ignore CycleValue
	CustEmail  string `json:"custEmail,omitempty"`
}

type Subscription struct {
	SubscriptionID int64  `json:"subscriptionId"`
	SubscribedAt   string `json:"subscribedAt"`
	CardMask       string `json:"cardMask"`
	Plan           *Plan  `json:"plan"`
	NextBillAt     string `json:"nextBillAt"`
	LastBilledAt   string `json:"lastBilledAt"`
	Status         string `json:"status"`
}

type QRRequest struct {
	Amount        float64 `json:"amount"`
	TransactionID string  `json:"transactionId"`
	ExpiresIn     int     `json:"expiresIn,omitempty"` // seconds
}

type QR struct {
	InvoiceID string   `json:"invoiceId"`
	QRCode    string   `json:"qrCode"`
	QRImage   string   `json:"qrImage"` // base64 PNG
	Links     []QRLink `json:"links"`
}

type QRLink struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	Logo               string `json:"logo"`
	Link               string `json:"link"` // bank-app deeplink
	AppStoreID         string `json:"appStoreId"`
	AndroidPackageName string `json:"androidPackageName"`
}

type QRInvoice struct {
	Merchant struct {
		ID         int64  `json:"id"`
		MerchantID string `json:"merchantId"`
		Name       string `json:"name"`
		Logo       string `json:"logo"`
	} `json:"merchant"`
	Invoice struct {
		InvoiceID   int64   `json:"invoiceId"`
		Amount      float64 `json:"amount"`
		Currency    string  `json:"currency"`
		CreatedAt   string  `json:"createdAt"`
		Status      string  `json:"status"` // OPEN, PAID, ...
		InitType    string  `json:"initType"`
		TerminalID  string  `json:"terminalId"`
		Description string  `json:"description"`
	} `json:"invoice"`
}

// Text is a string that also accepts JSON numbers and booleans; Bonum sends
// some IDs and timestamps as either.
type Text string

func (t *Text) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*t = Text(s)
		return nil
	}
	if string(b) != "null" {
		*t = Text(b)
	}
	return nil
}
