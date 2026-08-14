// Package generator creates cryptographically random Envseal credentials.
package generator

import (
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"strings"
)

const (
	// MinPassphraseWords is the smallest supported passphrase length.
	MinPassphraseWords = 6
	// MaxPassphraseWords is the largest supported passphrase length.
	MaxPassphraseWords = 64
	// MinSecretBytes is the smallest supported machine-secret size.
	MinSecretBytes = 16
	// MaxSecretBytes is the largest supported machine-secret size.
	MaxSecretBytes = 4_096

	wordListEntries = 7_776
)

var (
	// ErrPassphraseWords means a passphrase word count is outside the supported range.
	ErrPassphraseWords = errors.New("passphrase word count must be between 6 and 64")
	// ErrSecretBytes means a machine-secret byte count is outside the supported range.
	ErrSecretBytes = errors.New("secret byte count must be between 16 and 4096")
	// ErrEntropy means the system could not read cryptographic randomness.
	ErrEntropy = errors.New("cryptographic random read failed")
)

//go:embed eff_large_wordlist.txt
var effLargeWordList string

var words = mustParseWordList(effLargeWordList)

// Generator creates credentials from an entropy source.
type Generator struct {
	entropy io.Reader
}

// New returns a Generator backed by the operating system's cryptographic random source.
func New() *Generator {
	return NewWithEntropy(rand.Reader)
}

// NewWithEntropy returns a Generator backed by entropy. It is intended for controlled
// internal use, including deterministic tests; production callers should use New.
func NewWithEntropy(entropy io.Reader) *Generator {
	return &Generator{entropy: entropy}
}

// Passphrase returns words independently selected from the EFF Large Wordlist and
// joined with hyphens. It never returns a partial passphrase with an error.
func (g *Generator) Passphrase(count int) (string, error) {
	if count < MinPassphraseWords || count > MaxPassphraseWords {
		return "", ErrPassphraseWords
	}
	if g == nil || g.entropy == nil {
		return "", ErrEntropy
	}

	selected := make([]string, count)
	limit := big.NewInt(int64(len(words)))
	for i := range selected {
		index, err := rand.Int(g.entropy, limit)
		if err != nil {
			return "", ErrEntropy
		}
		selected[i] = words[index.Int64()]
	}
	return strings.Join(selected, "-"), nil
}

// Secret returns byteCount random bytes encoded with standard padded Base64. It never
// returns a partial secret with an error.
func (g *Generator) Secret(byteCount int) (string, error) {
	if byteCount < MinSecretBytes || byteCount > MaxSecretBytes {
		return "", ErrSecretBytes
	}
	if g == nil || g.entropy == nil {
		return "", ErrEntropy
	}

	secret := make([]byte, byteCount)
	if _, err := io.ReadFull(g.entropy, secret); err != nil {
		return "", ErrEntropy
	}
	return base64.StdEncoding.EncodeToString(secret), nil
}

func mustParseWordList(data string) []string {
	words, err := parseWordList(data)
	if err != nil {
		panic("invalid embedded EFF Large Wordlist")
	}
	return words
}

func parseWordList(data string) ([]string, error) {
	data = normalizeWordListLineEndings(data)
	lines := strings.Split(strings.TrimSuffix(data, "\n"), "\n")
	if len(lines) != wordListEntries {
		return nil, errors.New("unexpected word list entry count")
	}

	words := make([]string, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for i, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) != 2 || fields[0] != diceRoll(i) || fields[1] == "" {
			return nil, errors.New("invalid word list entry")
		}
		if _, exists := seen[fields[1]]; exists {
			return nil, errors.New("duplicate word list entry")
		}
		seen[fields[1]] = struct{}{}
		words[i] = fields[1]
	}
	return words, nil
}

// normalizeWordListLineEndings preserves the canonical LF representation when Git
// checks out the embedded source with Windows CRLF line endings.
func normalizeWordListLineEndings(data string) string {
	return strings.ReplaceAll(data, "\r\n", "\n")
}

func diceRoll(index int) string {
	roll := [5]byte{}
	for i := len(roll) - 1; i >= 0; i-- {
		roll[i] = byte(index%6) + '1'
		index /= 6
	}
	return string(roll[:])
}
