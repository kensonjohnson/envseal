// Package envelope implements Envseal's versioned per-value encryption format.
package envelope

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
)

const (
	// MaxPlaintextBytes is the maximum raw dotenv value size Envseal v1 seals.
	MaxPlaintextBytes = 1 << 20

	// MaxEnvelopeBytes bounds an encoded ENVSEAL value before parsing, decoding,
	// or deriving a password key.
	MaxEnvelopeBytes = 1_400_000

	iterations = 1_000_000
	saltBytes  = 16
	keyBytes   = 32

	wrapperPrefix = "ENVSEAL["
	wrapperSuffix = "]"
	version       = "v1"
	kdf           = "pbkdf2-sha256"
)

var (
	// ErrPlaintextTooLarge means a value exceeds Envseal's v1 plaintext limit.
	ErrPlaintextTooLarge = errors.New("plaintext exceeds envelope size limit")
	// ErrEnvelopeTooLarge means an encoded value exceeds the pre-KDF limit.
	ErrEnvelopeTooLarge = errors.New("envelope exceeds size limit")
	// ErrMalformedEnvelope means an encoded value does not satisfy the strict v1 grammar.
	ErrMalformedEnvelope = errors.New("malformed envelope")
	// ErrAuthentication means a password or key did not authenticate an otherwise valid envelope.
	ErrAuthentication = errors.New("envelope authentication failed")
)

// Seal encrypts plaintext for the exact case-sensitive dotenv key. It creates
// independent random salt and GCM nonce material for every call.
func Seal(password, key, plaintext []byte) (string, error) {
	if len(plaintext) > MaxPlaintextBytes {
		return "", ErrPlaintextTooLarge
	}

	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	derived, err := deriveKey(password, salt)
	if err != nil {
		return "", err
	}
	defer clear(derived)

	aead, err := newAEAD(derived)
	if err != nil {
		return "", err
	}
	sealed := aead.Seal(nil, nil, plaintext, associatedData(key))

	envelope := wrapperPrefix + strings.Join([]string{
		version,
		kdf,
		"1000000",
		base64.RawURLEncoding.EncodeToString(salt),
		base64.RawURLEncoding.EncodeToString(sealed),
	}, ":") + wrapperSuffix
	if len(envelope) > MaxEnvelopeBytes {
		// This should be unreachable for a MaxPlaintextBytes value, but preserves
		// the serialization boundary if the AEAD representation changes.
		return "", ErrEnvelopeTooLarge
	}
	return envelope, nil
}

// Open validates and decrypts an Envseal v1 value for the exact case-sensitive
// dotenv key. It never returns plaintext alongside an error.
func Open(password, key []byte, encoded string) ([]byte, error) {
	parsed, err := parse(encoded)
	if err != nil {
		return nil, err
	}

	derived, err := deriveKey(password, parsed.salt)
	if err != nil {
		return nil, err
	}
	defer clear(derived)

	aead, err := newAEAD(derived)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, nil, parsed.sealed, associatedData(key))
	if err != nil {
		return nil, ErrAuthentication
	}
	if len(plaintext) > MaxPlaintextBytes {
		return nil, ErrPlaintextTooLarge
	}
	return plaintext, nil
}

// Validate checks the strict v1 envelope grammar and size limits without
// deriving a password key or attempting authentication.
func Validate(encoded string) error {
	_, err := parse(encoded)
	return err
}

type parsedEnvelope struct {
	salt   []byte
	sealed []byte
}

func parse(encoded string) (parsedEnvelope, error) {
	if len(encoded) > MaxEnvelopeBytes {
		return parsedEnvelope{}, ErrEnvelopeTooLarge
	}
	if !strings.HasPrefix(encoded, wrapperPrefix) || !strings.HasSuffix(encoded, wrapperSuffix) {
		return parsedEnvelope{}, ErrMalformedEnvelope
	}

	body := strings.TrimSuffix(strings.TrimPrefix(encoded, wrapperPrefix), wrapperSuffix)
	parts := strings.Split(body, ":")
	if len(parts) != 5 || parts[0] != version || parts[1] != kdf || parts[2] != "1000000" {
		return parsedEnvelope{}, ErrMalformedEnvelope
	}

	salt, ok := decodeCanonical(parts[3])
	if !ok || len(salt) != saltBytes {
		return parsedEnvelope{}, ErrMalformedEnvelope
	}
	sealed, ok := decodeCanonical(parts[4])
	// NewGCMWithRandomNonce prepends a 12-byte nonce and appends a 16-byte tag.
	if !ok || len(sealed) < 28 {
		return parsedEnvelope{}, ErrMalformedEnvelope
	}
	if len(sealed)-28 > MaxPlaintextBytes {
		return parsedEnvelope{}, ErrPlaintextTooLarge
	}
	return parsedEnvelope{salt: salt, sealed: sealed}, nil
}

func decodeCanonical(encoded string) ([]byte, bool) {
	if encoded == "" {
		return nil, false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != encoded {
		return nil, false
	}
	return decoded, true
}

func deriveKey(password, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, string(password), salt, iterations, keyBytes)
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}

// associatedData is a fixed, length-unambiguous protocol identifier followed
// by exact key bytes. The binary key length makes this unambiguous even when
// called outside the dotenv parser.
func associatedData(key []byte) []byte {
	const prefix = "envseal\x00v1\x00"
	data := make([]byte, len(prefix)+4+len(key))
	n := copy(data, prefix)
	binary.BigEndian.PutUint32(data[n:n+4], uint32(len(key)))
	copy(data[n+4:], key)
	return data
}

func clear(bytes []byte) {
	for i := range bytes {
		bytes[i] = 0
	}
}
