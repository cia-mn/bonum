package bonum

import (
	"context"
	"net/url"
	"strconv"
)

// --- Web payment (checkout page) ---

// PaymentProviders lists the checkout options enabled for the terminal.
func (c *Client) PaymentProviders(ctx context.Context) ([]Provider, error) {
	return call[[]Provider](c, ctx, "GET", "/bonum-gateway/ecommerce/invoices/payment-providers", nil, nil)
}

// CreateInvoice creates a checkout invoice. Redirect the customer to
// Invoice.FollowUpLink; the result arrives on your webhook (type PAYMENT).
func (c *Client) CreateInvoice(ctx context.Context, req InvoiceRequest) (*Invoice, error) {
	return call[*Invoice](c, ctx, "POST", "/bonum-gateway/ecommerce/invoices", nil, req)
}

// --- Card tokenization ---

// CreateCardToken starts card tokenization. Redirect the customer to
// Tokenization.FollowUpLink; the token arrives on your webhook (type CARD-TOKEN).
func (c *Client) CreateCardToken(ctx context.Context, req CardTokenRequest) (*Tokenization, error) {
	return call[*Tokenization](c, ctx, "POST", "/mpay-service/merchant/cards/tokenize/request", nil, req)
}

// Purchase charges a card token. Currency defaults to MNT. A declined
// purchase is returned as *Error whose Data holds the PurchaseResult; a
// QUEUED result completes asynchronously via webhook (type TOKEN-PAYMENT).
func (c *Client) Purchase(ctx context.Context, cardToken string, req PurchaseRequest) (*PurchaseResult, error) {
	if req.Currency == "" {
		req.Currency = "MNT"
	}
	return data[*PurchaseResult](c, ctx, "POST", "/mpay-service/merchant/transaction/purchase", card(cardToken), req)
}

// ReversePurchase rolls back a token purchase by your transaction ID.
func (c *Client) ReversePurchase(ctx context.Context, cardToken, transactionID string) error {
	return c.Do(ctx, "DELETE", "/mpay-service/merchant/transaction/reverse/"+url.PathEscape(transactionID), card(cardToken), nil, nil)
}

// --- Subscriptions ---

// PaymentPlans lists the merchant's subscription plans (managed on the portal).
func (c *Client) PaymentPlans(ctx context.Context) ([]Plan, error) {
	return data[[]Plan](c, ctx, "GET", "/mpay-service/merchant/values/payment-plans", nil, nil)
}

// Subscribe subscribes a card token to a plan. Automatic payments are
// reported on your webhook (type SUBSCRIPTION-PAYMENT).
func (c *Client) Subscribe(ctx context.Context, cardToken string, req SubscribeRequest) (*Subscription, error) {
	return data[*Subscription](c, ctx, "POST", "/mpay-service/merchant/subscriptions/subscribe", card(cardToken), req)
}

// Subscriptions lists the subscriptions of a card token.
func (c *Client) Subscriptions(ctx context.Context, cardToken string) ([]Subscription, error) {
	return data[[]Subscription](c, ctx, "GET", "/mpay-service/merchant/subscriptions", card(cardToken), nil)
}

// ChangeSubscriptionToken moves a subscription to another existing card token.
func (c *Client) ChangeSubscriptionToken(ctx context.Context, subscriptionID int64, newCardToken string) error {
	return c.Do(ctx, "PUT", subPath(subscriptionID, "/change"), card(newCardToken), nil, nil)
}

// ChangeSubscriptionNewToken tokenizes a new card for a subscription; only
// Callback, TransactionID and Items of req are used.
func (c *Client) ChangeSubscriptionNewToken(ctx context.Context, subscriptionID int64, req CardTokenRequest) (*Tokenization, error) {
	return call[*Tokenization](c, ctx, "PUT", subPath(subscriptionID, "/change/create-new-token"), nil, req)
}

// Unsubscribe ends a subscription after its next billing date.
func (c *Client) Unsubscribe(ctx context.Context, cardToken string, subscriptionID int64) error {
	return c.Do(ctx, "DELETE", subPath(subscriptionID, ""), card(cardToken), nil, nil)
}

// DeleteSubscription ends a subscription immediately (no further billing).
func (c *Client) DeleteSubscription(ctx context.Context, cardToken string, subscriptionID int64) error {
	return c.Do(ctx, "DELETE", subPath(subscriptionID, "/delete"), card(cardToken), nil, nil)
}

// ExecuteSubscription triggers a billing run now. Sandbox only.
func (c *Client) ExecuteSubscription(ctx context.Context, subscriptionID int64) error {
	return c.Do(ctx, "PUT", subPath(subscriptionID, "/execute"), nil, nil, nil)
}

func subPath(id int64, suffix string) string {
	return "/mpay-service/merchant/subscriptions/" + strconv.FormatInt(id, 10) + suffix
}

// --- QR / deeplink payment ---

// CreateQR creates a QPay QR invoice with bank-app deeplinks.
func (c *Client) CreateQR(ctx context.Context, req QRRequest) (*QR, error) {
	return data[*QR](c, ctx, "POST", "/mpay-service/merchant/transaction/qr/create", nil, req)
}

// QRInvoice looks up the invoice behind a QPay QR code.
func (c *Client) QRInvoice(ctx context.Context, qrCode string) (*QRInvoice, error) {
	return data[*QRInvoice](c, ctx, "POST", "/mpay-service/merchant/transaction/qr", nil, map[string]string{"qrCode": qrCode})
}

// PayQR pays a QR invoice with a card token.
func (c *Client) PayQR(ctx context.Context, cardToken, qrCode, transactionID string) (*PurchaseResult, error) {
	return data[*PurchaseResult](c, ctx, "PUT", "/mpay-service/merchant/transaction/qr/pay", card(cardToken),
		map[string]string{"qrCode": qrCode, "transactionId": transactionID})
}
