# bonum

Go client for the [Bonum payment gateway](https://psp.bonum.mn/bonum-gateway-apis.html)
(Mongolia): checkout invoices, card tokenization, token purchases,
subscriptions, QR/deeplink payments and webhook validation. Zero dependencies.

```sh
go get github.com/cia-mn/bonum
```

## Quick start

```go
import "github.com/cia-mn/bonum"

c := bonum.New(bonum.Sandbox, appSecret, terminalID) // or bonum.Production
c.Lang = "mn"                                        // Accept-Language, default "en"

inv, err := c.CreateInvoice(ctx, bonum.InvoiceRequest{
    Amount:        15000,
    Callback:      "https://shop.mn/thanks?order=42",
    TransactionID: "order-42",
    ExpiresIn:     900,
    Providers:     []string{bonum.QPay, bonum.ECommerce}, // optional
})
// redirect the customer to inv.FollowUpLink; the result arrives on your webhook
```

Auth is automatic: the client fetches an access token on first use, caches it,
and refreshes it before expiry (the gateway rate-limits token creation, so
create **one** `Client` and reuse it).

## Webhook

Bonum POSTs results to the URL registered on the merchant portal, signed with
`x-checksum-v2` (HMAC-SHA256 of the body using your `MERCHANT_CHECKSUM_KEY`).

```go
http.HandleFunc("/bonum/webhook", func(w http.ResponseWriter, r *http.Request) {
    wh, err := bonum.ReadWebhook(r, checksumKey) // returns bonum.ErrChecksum on tampering
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    switch wh.Type {
    case bonum.WebhookPayment:
        // wh.Status == bonum.StatusSuccess, wh.Body.TransactionID, wh.Body.Amount ...
    case bonum.WebhookCardToken:
        // wh.Body.Token, wh.Body.Mask, wh.Body.Bank ...
    case bonum.WebhookSubscriptionPayment, bonum.WebhookTokenPayment, bonum.WebhookUnsubscribed:
    }
    w.WriteHeader(http.StatusOK)
})
```

Frameworks that already consumed the body (Fiber, etc.): `bonum.ParseWebhook(body, checksumHeader, key)`.

## Endpoints

| Method | Gateway call |
|---|---|
| `PaymentProviders` | `GET /bonum-gateway/ecommerce/invoices/payment-providers` |
| `CreateInvoice` | `POST /bonum-gateway/ecommerce/invoices` |
| `CreateCardToken` | `POST /mpay-service/merchant/cards/tokenize/request` |
| `Purchase` | `POST /mpay-service/merchant/transaction/purchase` |
| `ReversePurchase` | `DELETE /mpay-service/merchant/transaction/reverse/{transactionId}` |
| `PaymentPlans` | `GET /mpay-service/merchant/values/payment-plans` |
| `Subscribe` | `POST /mpay-service/merchant/subscriptions/subscribe` |
| `Subscriptions` | `GET /mpay-service/merchant/subscriptions` |
| `ChangeSubscriptionToken` | `PUT /mpay-service/merchant/subscriptions/{id}/change` |
| `ChangeSubscriptionNewToken` | `PUT /mpay-service/merchant/subscriptions/{id}/change/create-new-token` |
| `Unsubscribe` | `DELETE /mpay-service/merchant/subscriptions/{id}` |
| `DeleteSubscription` | `DELETE /mpay-service/merchant/subscriptions/{id}/delete` |
| `ExecuteSubscription` (sandbox) | `PUT /mpay-service/merchant/subscriptions/{id}/execute` |
| `CreateQR` | `POST /mpay-service/merchant/transaction/qr/create` |
| `QRInvoice` | `POST /mpay-service/merchant/transaction/qr` |
| `PayQR` | `PUT /mpay-service/merchant/transaction/qr/pay` |
| `AccessToken` | `GET /bonum-gateway/ecommerce/auth/create` / `auth/refresh` |
| `Do` | anything else — authenticated JSON request to any path |

Card-token calls take the customer's token as the first argument (sent as `X-CARD-TOKEN`).

## Errors

Non-2xx responses are `*bonum.Error` with `Status`, `TraceID`, `Message` and the raw
`Data`. A declined `Purchase` is an error whose `Data` decodes into `PurchaseResult`:

```go
res, err := c.Purchase(ctx, cardToken, bonum.PurchaseRequest{Amount: 15, TransactionID: "p-1"})
var e *bonum.Error
if errors.As(err, &e) {
    var declined bonum.PurchaseResult
    json.Unmarshal(e.Data, &declined) // declined.Status == "FAILED", declined.CardStatus ...
}
// res.Status is SUCCESS or QUEUED (result comes later via TOKEN-PAYMENT webhook)
```

## Testing

`go test ./...` runs offline. To hit the sandbox:

```sh
BONUM_APP_SECRET=... BONUM_TERMINAL_ID=... go test -run Sandbox -v
```

Sandbox credentials are published in the [API docs](https://psp.bonum.mn/bonum-gateway-apis.html#environment).

## Notes

- Timestamps are strings (`2006-01-02 15:04:05`, Ulaanbaatar time) and amounts are
  MNT `float64`, exactly as the gateway sends them.
- `Get Invoice Status` / `Set Invoice Paid` (testing-only endpoints) are not wrapped:
  the sandbox currently rejects them. Use `Do` if they come back.
- Apple Pay / Google Pay V2 endpoints are a separate spec and not covered here.
