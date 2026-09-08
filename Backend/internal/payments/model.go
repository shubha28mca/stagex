// Package payments implements payment integration supporting Razorpay and
// a provider-agnostic Mock provider: create an order for a registration,
// then verify the payment signature or record failure.
//
// The design rules enforced here are: entries held 30 minutes on failure and
// a maximum of 3 retry attempts before lockout (rule 6).
package payments

import "time"

const (
	maxAttempts = 3
	holdWindow  = 30 * time.Minute
)

// Order is returned to the client to render the payment sheet / Razorpay checkout.
type Order struct {
	OrderRef    string  `json:"orderRef"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Attempt     int     `json:"attempt"`
	Provider    string  `json:"provider"`
	KeyID       string  `json:"keyId,omitempty"`       // Razorpay public key ID for checkout.js
	EventName   string  `json:"eventName,omitempty"`   // Prefill info
	FamilyName  string  `json:"familyName,omitempty"`  // Prefill info
	FamilyPhone string  `json:"familyPhone,omitempty"` // Prefill info
}

// ConfirmResult reports the outcome of a payment verification or failure attempt.
type ConfirmResult struct {
	Status         string `json:"status"` // paid | failed | locked
	Attempt        int    `json:"attempt"`
	AttemptsLeft   int    `json:"attemptsLeft"`
	RegistrationID string `json:"registrationId"`
}

type createOrderRequest struct {
	RegistrationID string `json:"registrationId"`
}

// VerifyRequest is the payload sent by the frontend after Razorpay checkout completion.
type VerifyRequest struct {
	RegistrationID    string `json:"registrationId"`
	RazorpayPaymentID string `json:"razorpayPaymentId,omitempty"`
	RazorpayOrderID   string `json:"razorpayOrderId,omitempty"`
	RazorpaySignature string `json:"razorpaySignature,omitempty"`
	Success           *bool  `json:"success,omitempty"` // For mock/simulator fallback
}

// confirmRequest supports legacy confirm calls.
type confirmRequest struct {
	RegistrationID string `json:"registrationId"`
	Success        bool   `json:"success"`
}
