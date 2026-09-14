package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mustafa-oezdemir/banking_go/internal/merchant"
)

const merchantWebhookTimeout = 5 * time.Second

type merchantWebhookSender struct{ client *http.Client }

// NewMerchantWebhookSender returns the HTTP adapter for signed merchant callbacks.
func NewMerchantWebhookSender() merchant.CompletionNotifier {
	return &merchantWebhookSender{client: &http.Client{Timeout: merchantWebhookTimeout}}
}

func (sender *merchantWebhookSender) NotifyCompletion(ctx context.Context, completion merchant.Completion) error {
	callbackURL, secret := strings.TrimSpace(completion.WebhookURL), strings.TrimSpace(completion.WebhookSecret)
	if callbackURL == "" && secret == "" {
		return nil
	}
	parsed, err := url.Parse(callbackURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || len(secret) < 32 {
		return errors.New("merchant webhook configuration is invalid")
	}
	body, err := json.Marshal(struct {
		EventID           string `json:"event_id"`
		ProviderPaymentID string `json:"provider_payment_id"`
		PaymentIntentID   string `json:"payment_intent_id"`
		MerchantReference string `json:"merchant_reference"`
		Amount            string `json:"amount"`
		Currency          string `json:"currency"`
		Status            string `json:"status"`
	}{
		EventID: "banking:" + completion.PaymentIntentID.String() + ":paid", ProviderPaymentID: completion.ProviderPaymentID.String(),
		PaymentIntentID: completion.PaymentIntentID.String(), MerchantReference: completion.MerchantReference,
		Amount: completion.Amount, Currency: completion.Currency, Status: "paid",
	})
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Payment-Signature", hex.EncodeToString(mac.Sum(nil)))
	response, err := sender.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return errors.New("merchant webhook rejected completion")
	}
	return nil
}
