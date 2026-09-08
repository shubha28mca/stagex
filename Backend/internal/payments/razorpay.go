package payments

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RazorpayProvider implements the Provider interface against the Razorpay Orders API.
type RazorpayProvider struct {
	keyID     string
	keySecret string
	client    *http.Client
	apiBase   string
}

// NewRazorpayProvider creates a new Razorpay provider instance.
func NewRazorpayProvider(keyID, keySecret string) *RazorpayProvider {
	return &RazorpayProvider{
		keyID:     keyID,
		keySecret: keySecret,
		client:    &http.Client{Timeout: 10 * time.Second},
		apiBase:   "https://api.razorpay.com/v1",
	}
}

// Name returns the provider name.
func (r *RazorpayProvider) Name() string {
	return "razorpay"
}

// KeyID returns the public Razorpay Key ID for client-side checkout.
func (r *RazorpayProvider) KeyID() string {
	return r.keyID
}

// razorpayOrderRequest is the payload sent to POST /v1/orders.
type razorpayOrderRequest struct {
	Amount   int64             `json:"amount"` // in paise
	Currency string            `json:"currency"`
	Receipt  string            `json:"receipt,omitempty"`
	Notes    map[string]string `json:"notes,omitempty"`
}

// razorpayOrderResponse is the response from Razorpay /v1/orders.
type razorpayOrderResponse struct {
	ID       string `json:"id"`
	Entity   string `json:"entity"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
	Error    *struct {
		Code        string `json:"code"`
		Description string `json:"description"`
	} `json:"error,omitempty"`
}

// CreateOrder calls Razorpay API to create an order in test or live mode.
func (r *RazorpayProvider) CreateOrder(amount float64, currency, receipt string) (string, error) {
	if r.keyID == "" || r.keySecret == "" {
		return "", fmt.Errorf("razorpay keys not configured")
	}

	paise := int64(amount * 100)
	if paise < 100 {
		paise = 100 // minimum 1 INR
	}
	if currency == "" {
		currency = "INR"
	}

	reqBody, err := json.Marshal(razorpayOrderRequest{
		Amount:   paise,
		Currency: currency,
		Receipt:  receipt,
		Notes: map[string]string{
			"platform": "IIG StageX",
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal razorpay order request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, r.apiBase+"/orders", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("create razorpay request: %w", err)
	}

	httpReq.SetBasicAuth(r.keyID, r.keySecret)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call razorpay api: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read razorpay response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("razorpay order creation failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var orderResp razorpayOrderResponse
	if err := json.Unmarshal(bodyBytes, &orderResp); err != nil {
		return "", fmt.Errorf("unmarshal razorpay order response: %w", err)
	}

	if orderResp.ID == "" {
		return "", fmt.Errorf("razorpay order ID missing in response")
	}

	return orderResp.ID, nil
}

// VerifySignature verifies the Razorpay payment signature using HMAC-SHA256.
// expected_signature = hex(HMAC_SHA256(order_id + "|" + payment_id, secret))
func (r *RazorpayProvider) VerifySignature(orderID, paymentID, signature string) bool {
	if r.keySecret == "" || orderID == "" || paymentID == "" || signature == "" {
		return false
	}

	data := orderID + "|" + paymentID
	h := hmac.New(sha256.New, []byte(r.keySecret))
	h.Write([]byte(data))
	expected := hex.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}
