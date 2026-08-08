// Package payments implements a provider-agnostic payment flow modeled on
// Razorpay: create an order for a registration, then confirm success or record
// a failure. The design rules enforced here are: entries held 30 minutes on
// failure and a maximum of 3 retry attempts before lockout (rule 6).
//
// The actual gateway call is stubbed behind the Provider interface so the flow
// runs end-to-end locally and a real Razorpay client can be dropped in later.
package payments

import "time"

const (
	maxAttempts = 3
	holdWindow  = 30 * time.Minute
)

// Order is returned to the client to render the payment sheet.
type Order struct {
	OrderRef string  `json:"orderRef"`
	Amount   float64 `json:"amount"`
	Attempt  int     `json:"attempt"`
	Provider string  `json:"provider"`
}

// ConfirmResult reports the outcome of a confirm/fail attempt.
type ConfirmResult struct {
	Status         string `json:"status"`         // paid | failed | locked
	Attempt        int    `json:"attempt"`
	AttemptsLeft   int    `json:"attemptsLeft"`
	RegistrationID string `json:"registrationId"`
}

type createOrderRequest struct {
	RegistrationID string `json:"registrationId"`
}

type confirmRequest struct {
	RegistrationID string `json:"registrationId"`
	// Success simulates the gateway outcome. A real integration would verify a
	// signed webhook payload instead of trusting the client.
	Success bool `json:"success"`
}
