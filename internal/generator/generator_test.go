package generator

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestPassphraseUsesRequestedWords(t *testing.T) {
	entropy := append([]byte{0, 0, 0x1e, 0x5f}, make([]byte, 8)...)
	generator := NewWithEntropy(bytes.NewReader(entropy))

	passphrase, err := generator.Passphrase(MinPassphraseWords)
	if err != nil {
		t.Fatalf("Passphrase() error = %v", err)
	}

	want := "abacus-zoom-abacus-abacus-abacus-abacus"
	if passphrase != want {
		t.Fatalf("Passphrase() = %q, want %q", passphrase, want)
	}
	for _, word := range strings.Split(passphrase, "-") {
		if _, found := wordsIndex(word); !found {
			t.Fatalf("Passphrase() returned word %q outside the EFF word list", word)
		}
	}
}

func TestPassphraseSupportsBounds(t *testing.T) {
	for _, count := range []int{MinPassphraseWords, MaxPassphraseWords} {
		t.Run("words", func(t *testing.T) {
			generator := NewWithEntropy(bytes.NewReader(make([]byte, count*2)))
			passphrase, err := generator.Passphrase(count)
			if err != nil {
				t.Fatalf("Passphrase(%d) error = %v", count, err)
			}
			parts := strings.Split(passphrase, "-")
			if len(parts) != count {
				t.Fatalf("Passphrase(%d) produced %d words", count, len(parts))
			}
			for _, word := range parts {
				if word != "abacus" {
					t.Fatalf("Passphrase(%d) word = %q, want abacus", count, word)
				}
			}
		})
	}
}

func TestPassphraseRejectsOutOfRangeEntropy(t *testing.T) {
	// rand.Int reads 13 random bits for the 7,776 entries. 0x1fff is outside
	// the range, so it must be discarded rather than reduced modulo 7,776.
	generator := NewWithEntropy(bytes.NewReader([]byte{0x1f, 0xff, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}))

	passphrase, err := generator.Passphrase(MinPassphraseWords)
	if err != nil {
		t.Fatalf("Passphrase() error = %v", err)
	}
	if want := strings.Repeat("abacus-", MinPassphraseWords-1) + "abacus"; passphrase != want {
		t.Fatalf("Passphrase() = %q, want rejection followed by %q", passphrase, want)
	}
}

func TestPassphraseBoundsAndEntropyFailureAreValueFree(t *testing.T) {
	secret := "do-not-return-this"
	tests := []struct {
		name      string
		generator *Generator
		count     int
		want      error
	}{
		{name: "below minimum", generator: NewWithEntropy(bytes.NewReader(nil)), count: MinPassphraseWords - 1, want: ErrPassphraseWords},
		{name: "above maximum", generator: NewWithEntropy(bytes.NewReader(nil)), count: MaxPassphraseWords + 1, want: ErrPassphraseWords},
		{name: "entropy failure", generator: NewWithEntropy(errorReader{errors.New(secret)}), count: MinPassphraseWords, want: ErrEntropy},
		{name: "nil generator", generator: nil, count: MinPassphraseWords, want: ErrEntropy},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			passphrase, err := test.generator.Passphrase(test.count)
			if passphrase != "" {
				t.Fatalf("Passphrase() = %q, want no credential", passphrase)
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("Passphrase() error = %v, want %v", err, test.want)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("Passphrase() error leaks entropy source detail: %q", err)
			}
		})
	}
}

func TestSecretUsesStandardPaddedBase64(t *testing.T) {
	entropy := make([]byte, MinSecretBytes)
	for i := range entropy {
		entropy[i] = byte(i)
	}
	generator := NewWithEntropy(bytes.NewReader(entropy))

	secret, err := generator.Secret(MinSecretBytes)
	if err != nil {
		t.Fatalf("Secret() error = %v", err)
	}
	if want := "AAECAwQFBgcICQoLDA0ODw=="; secret != want {
		t.Fatalf("Secret() = %q, want %q", secret, want)
	}
	decoded, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		t.Fatalf("Secret() Base64 decode error = %v", err)
	}
	if !bytes.Equal(decoded, entropy) {
		t.Fatalf("Secret() decoded = %x, want %x", decoded, entropy)
	}
}

func TestSecretSupportsMaximumBytes(t *testing.T) {
	generator := NewWithEntropy(bytes.NewReader(make([]byte, MaxSecretBytes)))

	secret, err := generator.Secret(MaxSecretBytes)
	if err != nil {
		t.Fatalf("Secret(%d) error = %v", MaxSecretBytes, err)
	}
	decoded, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		t.Fatalf("Secret(%d) Base64 decode error = %v", MaxSecretBytes, err)
	}
	if len(decoded) != MaxSecretBytes {
		t.Fatalf("Secret(%d) decoded length = %d", MaxSecretBytes, len(decoded))
	}
}

func TestSecretBoundsAndEntropyFailureAreValueFree(t *testing.T) {
	secret := "do-not-return-this"
	tests := []struct {
		name      string
		generator *Generator
		count     int
		want      error
	}{
		{name: "below minimum", generator: NewWithEntropy(bytes.NewReader(nil)), count: MinSecretBytes - 1, want: ErrSecretBytes},
		{name: "above maximum", generator: NewWithEntropy(bytes.NewReader(nil)), count: MaxSecretBytes + 1, want: ErrSecretBytes},
		{name: "entropy failure", generator: NewWithEntropy(errorReader{errors.New(secret)}), count: MinSecretBytes, want: ErrEntropy},
		{name: "nil entropy", generator: NewWithEntropy(nil), count: MinSecretBytes, want: ErrEntropy},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.generator.Secret(test.count)
			if got != "" {
				t.Fatalf("Secret() = %q, want no credential", got)
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("Secret() error = %v, want %v", err, test.want)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("Secret() error leaks entropy source detail: %q", err)
			}
		})
	}
}

func TestEmbeddedWordListIntegrity(t *testing.T) {
	const sourceSHA256 = "addd35536511597a02fa0a9ff1e5284677b8883b83e986e43f15a3db996b903e"

	lf := normalizeWordListLineEndings(effLargeWordList)
	for _, test := range []struct {
		name string
		data string
	}{
		{name: "LF", data: lf},
		{name: "CRLF", data: strings.ReplaceAll(lf, "\n", "\r\n")},
	} {
		t.Run(test.name, func(t *testing.T) {
			sum := sha256.Sum256([]byte(normalizeWordListLineEndings(test.data)))
			if got := hex.EncodeToString(sum[:]); got != sourceSHA256 {
				t.Fatalf("EFF Large Wordlist SHA-256 = %s, want %s", got, sourceSHA256)
			}
			parsed, err := parseWordList(test.data)
			if err != nil {
				t.Fatalf("parseWordList() error = %v", err)
			}
			if len(parsed) != wordListEntries {
				t.Fatalf("word list entries = %d, want %d", len(parsed), wordListEntries)
			}
			if parsed[0] != "abacus" || parsed[len(parsed)-1] != "zoom" {
				t.Fatalf("word list boundaries = %q, %q; want abacus, zoom", parsed[0], parsed[len(parsed)-1])
			}
		})
	}
}

func wordsIndex(want string) (int, bool) {
	for index, word := range words {
		if word == want {
			return index, true
		}
	}
	return 0, false
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}
