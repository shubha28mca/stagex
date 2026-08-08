package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"regexp"
	"time"

	platauth "github.com/iig/stagex/backend/internal/platform/auth"
	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// maxOTPAttempts is the design limit before a challenge locks (design §3.1).
const maxOTPAttempts = 3

var phonePattern = regexp.MustCompile(`^[6-9]\d{9}$`) // 10-digit Indian mobile

// Service implements the account business rules.
type Service struct {
	families FamilyRepository
	otps     OTPRepository
	tokens   *platauth.Manager
	otpTTL   time.Duration
	devMode  bool           // when true, OTPs are returned in the API response
	now      func() time.Time
}

// NewService builds an auth Service. devMode should be false in production.
func NewService(f FamilyRepository, o OTPRepository, tokens *platauth.Manager, otpTTL time.Duration, devMode bool) *Service {
	return &Service{families: f, otps: o, tokens: tokens, otpTTL: otpTTL, devMode: devMode, now: time.Now}
}

// SendOTP validates the phone, generates a 6-digit OTP, stores only its hash,
// and (in dev) returns the code so the flow is testable without an SMS gateway.
func (s *Service) SendOTP(ctx context.Context, phone, purpose string) (sendOTPResponse, error) {
	if !phonePattern.MatchString(phone) {
		return sendOTPResponse{}, httpx.ErrBadRequest("enter a valid 10-digit mobile number")
	}
	if purpose == "" {
		purpose = "login"
	}
	code, err := generateOTP()
	if err != nil {
		return sendOTPResponse{}, httpx.ErrInternal("could not generate otp")
	}
	if err := s.otps.Create(ctx, phone, hashOTP(phone, code), purpose, s.now().Add(s.otpTTL)); err != nil {
		return sendOTPResponse{}, err
	}
	resp := sendOTPResponse{Sent: true}
	if s.devMode {
		resp.DevOTP = code
	}
	return resp, nil
}

// verifyOTP consumes the latest active challenge for the phone/purpose. It
// enforces expiry and the 3-attempt lockout. Returns nil on success.
func (s *Service) verifyOTP(ctx context.Context, phone, code, purpose string) error {
	ch, err := s.otps.LatestActive(ctx, phone, purpose)
	if err != nil {
		return err
	}
	if ch == nil {
		return httpx.ErrBadRequest("no active otp — request a new one")
	}
	if s.now().After(ch.ExpiresAt) {
		return httpx.ErrBadRequest("otp expired — request a new one")
	}
	if ch.Attempts >= maxOTPAttempts {
		return httpx.NewError(429, "too_many_attempts", "too many attempts — request a new otp")
	}
	if hashOTP(phone, code) != ch.CodeHash {
		_ = s.otps.IncrementAttempts(ctx, ch.ID)
		return httpx.ErrBadRequest("incorrect otp")
	}
	return s.otps.Consume(ctx, ch.ID)
}

// VerifyOTP is the public verify endpoint (used by the reset/verify screens).
func (s *Service) VerifyOTP(ctx context.Context, phone, code, purpose string) error {
	return s.verifyOTP(ctx, phone, code, purpose)
}

// Register creates a new family after OTP verification and returns a token.
func (s *Service) Register(ctx context.Context, phone, name, password, otp string) (AuthResponse, error) {
	if !phonePattern.MatchString(phone) {
		return AuthResponse{}, httpx.ErrBadRequest("enter a valid 10-digit mobile number")
	}
	if len(password) < 8 {
		return AuthResponse{}, httpx.ErrBadRequest("password must be at least 8 characters")
	}
	if name == "" {
		return AuthResponse{}, httpx.ErrBadRequest("name is required")
	}
	existing, err := s.families.GetByPhone(ctx, phone)
	if err != nil {
		return AuthResponse{}, err
	}
	if existing != nil {
		return AuthResponse{}, httpx.ErrConflict("this number already has an account — login instead")
	}
	if err := s.verifyOTP(ctx, phone, otp, "register"); err != nil {
		return AuthResponse{}, err
	}
	hash, err := platauth.HashPassword(password)
	if err != nil {
		return AuthResponse{}, httpx.ErrInternal("could not secure password")
	}
	fam, err := s.families.Create(ctx, phone, name, hash)
	if err != nil {
		return AuthResponse{}, err
	}
	return s.issue(fam)
}

// Login authenticates by password OR OTP and returns a token.
func (s *Service) Login(ctx context.Context, phone, password, otp string) (AuthResponse, error) {
	fam, err := s.families.GetByPhone(ctx, phone)
	if err != nil {
		return AuthResponse{}, err
	}
	if fam == nil {
		return AuthResponse{}, httpx.ErrUnauthorized("no account for this number")
	}
	switch {
	case password != "":
		if !platauth.CheckPassword(fam.PasswordHash, password) {
			return AuthResponse{}, httpx.ErrUnauthorized("incorrect password")
		}
	case otp != "":
		if err := s.verifyOTP(ctx, phone, otp, "login"); err != nil {
			return AuthResponse{}, err
		}
	default:
		return AuthResponse{}, httpx.ErrBadRequest("provide a password or an otp")
	}
	return s.issue(fam)
}

func (s *Service) issue(fam *Family) (AuthResponse, error) {
	token, err := s.tokens.Issue(fam.ID, fam.Phone)
	if err != nil {
		return AuthResponse{}, httpx.ErrInternal("could not issue token")
	}
	fam.PasswordHash = ""
	return AuthResponse{Token: token, Family: *fam}, nil
}

// generateOTP returns a cryptographically-random 6-digit code.
func generateOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	s := n.String()
	for len(s) < 6 {
		s = "0" + s
	}
	return s, nil
}

// hashOTP salts the code with the phone number before hashing so the stored
// value is never the raw OTP.
func hashOTP(phone, code string) string {
	sum := sha256.Sum256([]byte(phone + ":" + code))
	return hex.EncodeToString(sum[:])
}
