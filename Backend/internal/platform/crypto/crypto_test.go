package crypto

import "testing"

// verhoeffCheckDigit computes the Verhoeff check digit for a numeric string,
// implemented independently of ValidateAadhaar so the test is a genuine
// cross-check rather than a tautology.
func verhoeffCheckDigit(num string) byte {
	inv := [10]int{0, 4, 3, 2, 1, 5, 6, 7, 8, 9}
	c := 0
	l := len(num)
	for i := 0; i < l; i++ {
		d := int(num[l-i-1] - '0')
		c = verhoeffD[c][verhoeffP[(i+1)%8][d]]
	}
	return byte('0' + inv[c])
}

func TestValidateAadhaar(t *testing.T) {
	base := "23412341234" // 11 digits; append a valid check digit
	valid := base + string(verhoeffCheckDigit(base))
	if !ValidateAadhaar(valid) {
		t.Fatalf("expected %s to be valid", valid)
	}
	// Flip the check digit → must be invalid.
	bad := base + string(byte('0'+(valid[11]-'0'+1)%10))
	if ValidateAadhaar(bad) {
		t.Fatalf("expected %s to be invalid", bad)
	}
	if ValidateAadhaar("123") {
		t.Fatal("short number must be invalid")
	}
}

func TestMask(t *testing.T) {
	if got := Mask("234123412346"); got != "XXXX-XXXX-2346" {
		t.Fatalf("Mask = %q", got)
	}
	if got := Mask("12"); got != "XXXX-XXXX-XXXX" {
		t.Fatalf("Mask short = %q", got)
	}
}

func TestCipherRoundTrip(t *testing.T) {
	c, err := NewCipher("test-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("234123412346")
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if string(enc) == string(plain) {
		t.Fatal("ciphertext must differ from plaintext")
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(dec) != string(plain) {
		t.Fatalf("round trip mismatch: %q", dec)
	}
}
