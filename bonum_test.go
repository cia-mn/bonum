package bonum

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// fake gateway: counts token calls, checks headers, serves a few endpoints.
func fake(t *testing.T) (*Client, *int) {
	t.Helper()
	creates, refreshes := 0, 0
	mux := http.NewServeMux()
	mux.HandleFunc("/bonum-gateway/ecommerce/auth/create", func(w http.ResponseWriter, r *http.Request) {
		creates++
		if r.Header.Get("Authorization") != "AppSecret sec" || r.Header.Get("X-TERMINAL-ID") != "term" {
			t.Errorf("bad auth headers: %v", r.Header)
		}
		fmt.Fprint(w, `{"tokenType":"Bearer ","accessToken":"A1","expiresIn":1800,"refreshToken":"R1","refreshExpiresIn":2000}`)
	})
	mux.HandleFunc("/bonum-gateway/ecommerce/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		refreshes++
		if r.Header.Get("Authorization") != "Bearer R1" {
			t.Errorf("bad refresh header: %v", r.Header)
		}
		fmt.Fprint(w, `{"accessToken":"A2","expiresIn":1800,"refreshToken":"R2","refreshExpiresIn":2000}`)
	})
	mux.HandleFunc("/mpay-service/merchant/values/payment-plans", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer A1" && r.Header.Get("Authorization") != "Bearer A2" {
			t.Errorf("bad bearer: %q", r.Header.Get("Authorization"))
		}
		fmt.Fprint(w, `{"traceId":"t","data":[{"planId":1,"name":"Weekly","amount":5.00}],"status":200}`)
	})
	mux.HandleFunc("/mpay-service/merchant/transaction/purchase", func(w http.ResponseWriter, r *http.Request) {
		var in PurchaseRequest
		json.NewDecoder(r.Body).Decode(&in)
		if r.Header.Get("X-CARD-TOKEN") != "ct" || in.Currency != "MNT" || r.Header.Get("Accept-Language") != "mn" {
			t.Errorf("bad purchase request: %v %+v", r.Header, in)
		}
		w.WriteHeader(400)
		fmt.Fprint(w, `{"traceId":"tr1","errorCode":"${invalid.bonum.response.56}","message":"Card payment not possible (56)","data":{"id":171044,"status":"FAILED","cardStatus":"INACTIVE"},"status":400}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "sec", "term")
	c.Lang = "mn"
	_ = refreshes
	return c, &creates
}

func TestTokenCachedAndRefreshed(t *testing.T) {
	c, creates := fake(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := c.PaymentPlans(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if *creates != 1 {
		t.Fatalf("token created %d times, want 1", *creates)
	}
	c.exp = time.Time{} // expire access token; refresh token still valid
	tok, err := c.AccessToken(ctx)
	if err != nil || tok != "A2" || *creates != 1 {
		t.Fatalf("refresh: tok=%q err=%v creates=%d", tok, err, *creates)
	}
	c.exp, c.rexp = time.Time{}, time.Time{} // both expired -> create again
	if tok, _ = c.AccessToken(ctx); tok != "A1" || *creates != 2 {
		t.Fatalf("recreate: tok=%q creates=%d", tok, *creates)
	}
}

func TestEnvelopeAndError(t *testing.T) {
	c, _ := fake(t)
	ctx := context.Background()
	plans, err := c.PaymentPlans(ctx)
	if err != nil || len(plans) != 1 || plans[0].Name != "Weekly" || plans[0].Amount != 5 {
		t.Fatalf("plans=%+v err=%v", plans, err)
	}
	_, err = c.Purchase(ctx, "ct", PurchaseRequest{Amount: 15, TransactionID: "x"})
	var e *Error
	if !errors.As(err, &e) || e.Status != 400 || e.TraceID != "tr1" || !strings.Contains(e.Error(), "(56)") {
		t.Fatalf("err=%#v", err)
	}
	var res PurchaseResult
	if json.Unmarshal(e.Data, &res); res.Status != StatusFailed || res.CardStatus != "INACTIVE" {
		t.Fatalf("data=%+v", res)
	}
}

func TestWebhook(t *testing.T) {
	key := "755753df1f8fb16da1131cc318f1bcec9b5df3e39ae5dee902900cd186e7ece8"
	pretty := []byte("{\n  \"type\": \"PAYMENT\",\n  \"status\": \"SUCCESS\",\n  \"body\": {\"invoiceId\": 786, \"updatedAt\": 1769657291559, \"amount\": 10000.00, \"transactionId\": \"N1\"}\n}")
	compact := []byte(`{"type":"PAYMENT","status":"SUCCESS","body":{"invoiceId":786,"updatedAt":1769657291559,"amount":10000.00,"transactionId":"N1"}}`)

	// known answer: HMAC-SHA256("key", "The quick brown fox jumps over the lazy dog")
	if got := Checksum([]byte("The quick brown fox jumps over the lazy dog"), "key"); got != "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8" {
		t.Fatalf("checksum=%s", got)
	}
	// signature over compact JSON must validate a pretty-printed body, and vice versa
	sig := Checksum(compact, key)
	req := httptest.NewRequest("POST", "/hook", strings.NewReader(string(pretty)))
	req.Header.Set(ChecksumHeader, sig)
	w, err := ReadWebhook(req, key)
	if err != nil {
		t.Fatal(err)
	}
	if w.Type != WebhookPayment || w.Body.InvoiceID != "786" || w.Body.UpdatedAt != "1769657291559" || w.Body.Amount != 10000 || w.Body.TransactionID != "N1" {
		t.Fatalf("webhook=%+v", w)
	}
	if _, err := ParseWebhook(pretty, sig, "wrong-key"); !errors.Is(err, ErrChecksum) {
		t.Fatalf("want ErrChecksum, got %v", err)
	}
	if _, err := ParseWebhook(pretty, Checksum(pretty, key), key); err != nil {
		t.Fatalf("raw-body signature rejected: %v", err)
	}
}

// TestSandbox hits the real sandbox. Enable with:
//
//	BONUM_APP_SECRET=... BONUM_TERMINAL_ID=... go test -run Sandbox -v
func TestSandbox(t *testing.T) {
	sec, term := os.Getenv("BONUM_APP_SECRET"), os.Getenv("BONUM_TERMINAL_ID")
	if sec == "" || term == "" {
		t.Skip("BONUM_APP_SECRET / BONUM_TERMINAL_ID not set")
	}
	c := New(Sandbox, sec, term)
	ctx := context.Background()
	txn := fmt.Sprintf("bonum-%d", time.Now().UnixNano())

	prov, err := c.PaymentProviders(ctx)
	if err != nil || len(prov) == 0 {
		t.Fatalf("providers: %v %v", prov, err)
	}
	inv, err := c.CreateInvoice(ctx, InvoiceRequest{Amount: 1, Callback: "https://example.com/cb", TransactionID: txn, ExpiresIn: 300})
	if err != nil || inv.InvoiceID == "" || inv.FollowUpLink == "" {
		t.Fatalf("invoice: %+v %v", inv, err)
	}
	plans, err := c.PaymentPlans(ctx)
	if err != nil || len(plans) == 0 {
		t.Fatalf("plans: %v %v", plans, err)
	}
	qr, err := c.CreateQR(ctx, QRRequest{Amount: 1, TransactionID: txn + "q", ExpiresIn: 300})
	if err != nil || qr.QRCode == "" || len(qr.Links) == 0 {
		t.Fatalf("qr: %+v %v", qr, err)
	}
	qi, err := c.QRInvoice(ctx, qr.QRCode)
	if err != nil || qi.Invoice.Amount != 1 || qi.Invoice.Status == "" {
		t.Fatalf("qr invoice: %+v %v", qi, err)
	}
	tok, err := c.CreateCardToken(ctx, CardTokenRequest{Callback: "https://example.com/cb", TransactionID: txn + "t"})
	if err != nil || tok.ID == "" || tok.FollowUpLink == "" {
		t.Fatalf("tokenize: %+v %v", tok, err)
	}
	var e *Error
	if _, err = c.Subscriptions(ctx, "not-a-token"); !errors.As(err, &e) || e.Status != 400 {
		t.Fatalf("want 400 *Error for bad card token, got %v", err)
	}
	t.Logf("ok: invoice=%s qr=%s tokenize=%s", inv.FollowUpLink, qr.InvoiceID, tok.FollowUpLink)
}

// The gateway builds each Item into a DTO whose remark is a non-nullable constructor parameter:
// drop the key and the whole invoice 500s with "missing (therefore NULL) value for creator
// parameter remark". An empty remark must still serialize.
func TestItemAlwaysSendsRemark(t *testing.T) {
	b, err := json.Marshal(Item{Title: "Americano", Amount: 5000, Count: 1})
	if err != nil || !strings.Contains(string(b), `"remark"`) {
		t.Fatalf("remark must be present even when empty: %s %v", b, err)
	}
}
