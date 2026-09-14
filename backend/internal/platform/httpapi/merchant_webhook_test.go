package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/mustafa-oezdemir/banking_go/internal/merchant"
)

func TestMerchantWebhookSenderSignsCompletion(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		if got := request.Header.Get("X-Payment-Signature"); got != hex.EncodeToString(mac.Sum(nil)) {
			t.Fatalf("signature = %q", got)
		}
		if request.Method != http.MethodPost || request.URL.Path != "/webhooks/payments/pehlione_bank" {
			t.Fatalf("unexpected callback %s %s", request.Method, request.URL.Path)
		}
		var event struct {
			EventID           string `json:"event_id"`
			ProviderPaymentID string `json:"provider_payment_id"`
			PaymentIntentID   string `json:"payment_intent_id"`
			MerchantReference string `json:"merchant_reference"`
			Amount            string `json:"amount"`
			Currency          string `json:"currency"`
			Status            string `json:"status"`
		}
		if err := json.Unmarshal(body, &event); err != nil {
			t.Errorf("decode callback: %v", err)
		}
		if event.EventID == "" || event.ProviderPaymentID == "" || event.PaymentIntentID == "" || event.MerchantReference != "ECOM-ORD-42" || event.Amount != "149.99" || event.Currency != "EUR" || event.Status != "paid" {
			t.Errorf("unexpected callback contract: %#v", event)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	err := NewMerchantWebhookSender().NotifyCompletion(t.Context(), merchant.Completion{
		MerchantID: "pehlione-ecommerce", WebhookURL: server.URL + "/webhooks/payments/pehlione_bank", WebhookSecret: secret,
		PaymentIntentID: uuid.New(), ProviderPaymentID: uuid.New(), MerchantReference: "ECOM-ORD-42", Amount: "149.99", Currency: "EUR",
	})
	if err != nil {
		t.Fatalf("NotifyCompletion() error = %v", err)
	}
}
