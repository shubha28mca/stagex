// Package crypto provides the small security primitives the domain needs:
// symmetric encryption for Aadhaar-at-rest (AES-256-GCM), a masking helper, and
// Verhoeff checksum validation for Aadhaar numbers. Centralizing these keeps the
// sensitive-data rules (DPDP compliance, ClientDesignWeb §7) in one audited spot.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

// Cipher encrypts and decrypts small secrets with AES-256-GCM. The key is
// derived from a passphrase so operators can supply any-length secret.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher derives a 256-bit key from the passphrase and returns a Cipher.
func NewCipher(passphrase string) (*Cipher, error) {
	key := sha256.Sum256([]byte(passphrase))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns nonce||ciphertext. Safe to store directly as bytea.
func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt reverses Encrypt.
func (c *Cipher) Decrypt(data []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(data) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := data[:ns], data[ns:]
	return c.aead.Open(nil, nonce, ct, nil)
}

// Mask renders an Aadhaar as XXXX-XXXX-1234, revealing only the last 4 digits.
func Mask(aadhaar string) string {
	digits := onlyDigits(aadhaar)
	if len(digits) < 4 {
		return "XXXX-XXXX-XXXX"
	}
	return "XXXX-XXXX-" + digits[len(digits)-4:]
}

// ValidateAadhaar checks the number is 12 digits with a valid Verhoeff checksum
// (the algorithm UIDAI uses). This is a client-side style sanity check, not a
// UIDAI verification.
func ValidateAadhaar(aadhaar string) bool {
	digits := onlyDigits(aadhaar)
	if len(digits) != 12 {
		return false
	}
	c := 0
	l := len(digits)
	for i := 0; i < l; i++ {
		d := int(digits[l-i-1] - '0')
		c = verhoeffD[c][verhoeffP[i%8][d]]
	}
	return c == 0
}

func onlyDigits(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

// Verhoeff multiplication (d) and permutation (p) tables.
var verhoeffD = [10][10]int{
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
	{1, 2, 3, 4, 0, 6, 7, 8, 9, 5},
	{2, 3, 4, 0, 1, 7, 8, 9, 5, 6},
	{3, 4, 0, 1, 2, 8, 9, 5, 6, 7},
	{4, 0, 1, 2, 3, 9, 5, 6, 7, 8},
	{5, 9, 8, 7, 6, 0, 4, 3, 2, 1},
	{6, 5, 9, 8, 7, 1, 0, 4, 3, 2},
	{7, 6, 5, 9, 8, 2, 1, 0, 4, 3},
	{8, 7, 6, 5, 9, 3, 2, 1, 0, 4},
	{9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
}

var verhoeffP = [8][10]int{
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
	{1, 5, 7, 6, 2, 8, 3, 0, 9, 4},
	{5, 8, 0, 3, 7, 9, 6, 1, 4, 2},
	{8, 9, 1, 6, 0, 4, 3, 5, 2, 7},
	{9, 4, 5, 3, 1, 2, 6, 8, 7, 0},
	{4, 2, 8, 6, 5, 7, 3, 9, 0, 1},
	{2, 7, 9, 3, 8, 0, 6, 4, 1, 5},
	{7, 0, 4, 6, 9, 1, 3, 2, 5, 8},
}
