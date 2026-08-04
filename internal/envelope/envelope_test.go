package envelope

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

var (
	testPassword = []byte("password-manager-generated-test-secret")
	testKey      = []byte("API_TOKEN")
)

func TestSealOpenRoundTrip(t *testing.T) {
	plaintext := []byte("line value with $literal characters")
	encoded, err := Seal(testPassword, testKey, plaintext)
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	if !strings.HasPrefix(encoded, "ENVSEAL[v1:pbkdf2-sha256:1000000:") || !strings.HasSuffix(encoded, "]") {
		t.Fatalf("Seal() = %q, want exact v1 wrapper", encoded)
	}
	if len(encoded) > MaxEnvelopeBytes {
		t.Fatalf("Seal() length = %d, exceeds %d", len(encoded), MaxEnvelopeBytes)
	}
	if err := Validate(encoded); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	got, err := Open(testPassword, testKey, encoded)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Open() = %q, want %q", got, plaintext)
	}
}

func TestSealSerializesExactV1Fields(t *testing.T) {
	encoded, err := Seal(testPassword, testKey, []byte("plaintext"))
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(encoded, "ENVSEAL["), "]"), ":")
	if len(parts) != 5 {
		t.Fatalf("serialized field count = %d, want 5", len(parts))
	}
	if got, want := strings.Join(parts[:3], ":"), "v1:pbkdf2-sha256:1000000"; got != want {
		t.Fatalf("serialized fixed fields = %q, want %q", got, want)
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(salt) != 16 {
		t.Fatalf("serialized salt = %d bytes, %v; want 16 bytes", len(salt), err)
	}
	sealed, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil || len(sealed) < 28 {
		t.Fatalf("serialized sealed data = %d bytes, %v; want nonce and tag", len(sealed), err)
	}
}

func TestSealUsesIndependentRandomMaterial(t *testing.T) {
	first, err := Seal(testPassword, testKey, []byte("same plaintext"))
	if err != nil {
		t.Fatalf("first Seal() error = %v", err)
	}
	second, err := Seal(testPassword, testKey, []byte("same plaintext"))
	if err != nil {
		t.Fatalf("second Seal() error = %v", err)
	}
	if first == second {
		t.Fatal("Seal() emitted identical envelopes for identical inputs")
	}
}

func TestOpenAuthenticatesPasswordAndKey(t *testing.T) {
	encoded, err := Seal(testPassword, testKey, []byte("plaintext"))
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	for _, test := range []struct {
		name     string
		password []byte
		key      []byte
	}{
		{name: "wrong password", password: []byte("wrong-password"), key: testKey},
		{name: "wrong key", password: testPassword, key: []byte("OTHER_KEY")},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := Open(test.password, test.key, encoded)
			if !errors.Is(err, ErrAuthentication) {
				t.Fatalf("Open() error = %v, want authentication failure", err)
			}
			if got != nil {
				t.Fatalf("Open() plaintext = %q, want nil on failure", got)
			}
		})
	}
}

func TestValidateRejectsMalformedCanonicalFields(t *testing.T) {
	valid, err := Seal(testPassword, testKey, []byte("plaintext"))
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(valid, "ENVSEAL["), "]"), ":")

	badSalt := base64.RawURLEncoding.EncodeToString(make([]byte, 15))
	canonicalSalt := parts[3]
	canonicalSealed := parts[4]
	// A 16-byte raw base64url encoding has two significant bits in its final
	// character; B sets an unused bit and is therefore non-canonical.
	nonCanonicalSalt := canonicalSalt[:len(canonicalSalt)-1] + "B"

	tests := []string{
		"not-an-envelope",
		"ENVSEAL[v2:pbkdf2-sha256:1000000:" + canonicalSalt + ":" + canonicalSealed + "]",
		"ENVSEAL[v1:pbkdf2-sha1:1000000:" + canonicalSalt + ":" + canonicalSealed + "]",
		"ENVSEAL[v1:pbkdf2-sha256:01000000:" + canonicalSalt + ":" + canonicalSealed + "]",
		"ENVSEAL[v1:pbkdf2-sha256:1000000:" + canonicalSalt + "=:" + canonicalSealed + "]",
		"ENVSEAL[v1:pbkdf2-sha256:1000000:" + badSalt + ":" + canonicalSealed + "]",
		"ENVSEAL[v1:pbkdf2-sha256:1000000:" + nonCanonicalSalt + ":" + canonicalSealed + "]",
		"ENVSEAL[v1:pbkdf2-sha256:1000000:" + canonicalSalt + ":AA]",
		"ENVSEAL[v1:pbkdf2-sha256:1000000:" + canonicalSalt + ":" + canonicalSealed + ":extra]",
	}

	for _, encoded := range tests {
		if err := Validate(encoded); !errors.Is(err, ErrMalformedEnvelope) {
			t.Fatalf("Validate(%q) error = %v, want malformed envelope", encoded, err)
		}
		got, err := Open(testPassword, testKey, encoded)
		if got != nil {
			t.Fatalf("Open(%q) plaintext = %q, want nil on failure", encoded, got)
		}
		if !errors.Is(err, ErrMalformedEnvelope) {
			t.Fatalf("Open(%q) error = %v, want malformed envelope", encoded, err)
		}
	}
}

func TestSizeLimits(t *testing.T) {
	maximum := bytes.Repeat([]byte{'x'}, MaxPlaintextBytes)
	maximumEnvelope, err := Seal(testPassword, testKey, maximum)
	if err != nil {
		t.Fatalf("Seal(maximum plaintext) error = %v", err)
	}
	if len(maximumEnvelope) > MaxEnvelopeBytes {
		t.Fatalf("Seal(maximum plaintext) length = %d, exceeds %d", len(maximumEnvelope), MaxEnvelopeBytes)
	}

	overlarge := make([]byte, MaxPlaintextBytes+1)
	if encoded, err := Seal(testPassword, testKey, overlarge); encoded != "" || !errors.Is(err, ErrPlaintextTooLarge) {
		t.Fatalf("Seal(overlarge) = %q, %v; want empty, plaintext limit", encoded, err)
	}

	if err := Validate(strings.Repeat("x", MaxEnvelopeBytes+1)); !errors.Is(err, ErrEnvelopeTooLarge) {
		t.Fatalf("Validate(overlarge envelope) error = %v, want envelope limit", err)
	}

	salt := base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	sealed := base64.RawURLEncoding.EncodeToString(make([]byte, MaxPlaintextBytes+29))
	encoded := "ENVSEAL[v1:pbkdf2-sha256:1000000:" + salt + ":" + sealed + "]"
	if len(encoded) > MaxEnvelopeBytes {
		t.Fatalf("test envelope length = %d, exceeds parser limit", len(encoded))
	}
	if err := Validate(encoded); !errors.Is(err, ErrPlaintextTooLarge) {
		t.Fatalf("Validate(overlarge plaintext envelope) error = %v, want plaintext limit", err)
	}
	plaintext, err := Open(testPassword, testKey, encoded)
	if plaintext != nil || !errors.Is(err, ErrPlaintextTooLarge) {
		t.Fatalf("Open(overlarge plaintext envelope) = %q, %v; want nil, plaintext limit", plaintext, err)
	}
}

func TestAssociatedDataIsLengthUnambiguous(t *testing.T) {
	first := associatedData([]byte("a\x00bc"))
	second := associatedData([]byte("a\x00b"))
	if bytes.Equal(first, second) {
		t.Fatal("associated data collides for distinct keys")
	}
}
