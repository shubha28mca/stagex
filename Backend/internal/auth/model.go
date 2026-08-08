// Package auth (domain) implements the participant account flows: OTP send /
// verify, registration (phone + OTP + password) and dual login (password or
// OTP). It builds on platform/auth for JWT issuing and password hashing.
//
// Account model: one phone number = one family account (ClientDesignWeb §2).
package auth

import "time"

// Family is a participant account keyed to a mobile number.
type Family struct {
	ID           string    `json:"id"`
	Phone        string    `json:"phone"`
	DisplayName  string    `json:"displayName"`
	PasswordHash string    `json:"-"` // never serialized
	CreatedAt    time.Time `json:"createdAt"`
}

// OTPChallenge is a pending one-time-password verification.
type OTPChallenge struct {
	ID        string
	Phone     string
	CodeHash  string
	Purpose   string
	Attempts  int
	ExpiresAt time.Time
	Consumed  bool
}

// ---- Request / response DTOs ----

type sendOTPRequest struct {
	Phone   string `json:"phone"`
	Purpose string `json:"purpose"` // register | login | reset
}

type sendOTPResponse struct {
	Sent bool `json:"sent"`
	// DevOTP is only populated in non-production so the flow is testable
	// without a real SMS gateway. It is omitted in production.
	DevOTP string `json:"devOtp,omitempty"`
}

type verifyOTPRequest struct {
	Phone   string `json:"phone"`
	Code    string `json:"code"`
	Purpose string `json:"purpose"`
}

type registerRequest struct {
	Phone    string `json:"phone"`
	Name     string `json:"name"`
	Password string `json:"password"`
	OTP      string `json:"otp"`
}

type loginRequest struct {
	Phone    string `json:"phone"`
	Password string `json:"password"` // either password ...
	OTP      string `json:"otp"`      // ... or OTP
}

// AuthResponse is returned on successful register/login.
type AuthResponse struct {
	Token  string `json:"token"`
	Family Family `json:"family"`
}
