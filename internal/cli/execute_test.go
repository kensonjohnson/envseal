package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kensonjohnson/envseal/internal/envelope"
	"github.com/kensonjohnson/envseal/internal/filewrite"
)

func TestRunEndToEndCommands(t *testing.T) {
	const oldPassword = "old-password"
	const newPassword = "new-password"

	t.Run("encrypt", func(t *testing.T) {
		directory := t.TempDir()
		source := filepath.Join(directory, ".env.example")
		writeTestFile(t, source, "API_TOKEN=plaintext\nPUBLIC=visible\n")
		passwords := &fakePasswords{encrypt: []byte(newPassword)}
		stdout, stderr := runService(t, []string{"encrypt", source, "API_TOKEN"}, passwords)
		if got, want := stdout, "envseal: encrypted 1 keys in "+source+"\n"; got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		contents := readTestFile(t, source)
		if strings.Contains(contents, "API_TOKEN=plaintext") || !strings.Contains(contents, "API_TOKEN=ENVSEAL[") {
			t.Fatalf("encrypted source = %q", contents)
		}
		if !strings.Contains(contents, "PUBLIC=visible\n") {
			t.Fatalf("unselected bytes changed: %q", contents)
		}
	})

	t.Run("decrypt", func(t *testing.T) {
		directory := t.TempDir()
		source := filepath.Join(directory, ".env.example")
		target := filepath.Join(directory, ".env")
		writeTestFile(t, source, sealedLine(t, "API_TOKEN", oldPassword, "plaintext"))
		stdout, stderr := runService(t, []string{"decrypt", source, target}, &fakePasswords{decrypt: []byte(oldPassword)})
		if got, want := stdout, "envseal: decrypted 1 envelopes from "+source+"\n"; got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if got, want := readTestFile(t, target), "API_TOKEN=plaintext\n"; got != want {
			t.Fatalf("plaintext output = %q, want %q", got, want)
		}
	})

	t.Run("rotate", func(t *testing.T) {
		directory := t.TempDir()
		source := filepath.Join(directory, ".env.example")
		writeTestFile(t, source, sealedLine(t, "API_TOKEN", oldPassword, "plaintext"))
		stdout, stderr := runService(t, []string{"rotate", source}, &fakePasswords{current: []byte(oldPassword), replacement: []byte(newPassword)})
		if got, want := stdout, "envseal: rotated 1 envelopes in "+source+"\n"; got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		encoded := strings.TrimSuffix(strings.TrimPrefix(readTestFile(t, source), "API_TOKEN="), "\n")
		plaintext, err := envelope.Open([]byte(newPassword), []byte("API_TOKEN"), encoded)
		if err != nil || string(plaintext) != "plaintext" {
			t.Fatalf("new password decrypt = %q, %v", plaintext, err)
		}
		if _, err := envelope.Open([]byte(oldPassword), []byte("API_TOKEN"), encoded); !errors.Is(err, envelope.ErrAuthentication) {
			t.Fatalf("old password error = %v, want authentication failure", err)
		}
	})

	t.Run("check never prompts", func(t *testing.T) {
		directory := t.TempDir()
		source := filepath.Join(directory, ".env.example")
		writeTestFile(t, source, "API_TOKEN=plaintext\n")
		passwords := &fakePasswords{}
		stdout, stderr := runService(t, []string{"check", source}, passwords)
		if got, want := stdout, "envseal: "+source+" is valid\n"; got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if passwords.calls != 0 {
			t.Fatalf("password prompts = %d, want 0", passwords.calls)
		}
	})
}

func TestRunGenerate(t *testing.T) {
	t.Run("passphrase writes only credential", func(t *testing.T) {
		generator := &fakeGenerator{passphrase: "one-two-three-four-five-six-seven-eight"}
		stdout, stderr, code := runGenerate(t, []string{"generate", "passphrase"}, generator)
		if code != 0 {
			t.Fatalf("run() exit = %d, want 0", code)
		}
		if got, want := stdout, "one-two-three-four-five-six-seven-eight\n"; got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if generator.passphraseCount != 8 || generator.secretCount != 0 {
			t.Fatalf("generator calls = passphrase %d, secret %d", generator.passphraseCount, generator.secretCount)
		}
	})

	t.Run("secret writes only credential", func(t *testing.T) {
		generator := &fakeGenerator{secret: "AAECAwQFBgcICQoLDA0ODw=="}
		stdout, stderr, code := runGenerate(t, []string{"generate", "secret", "--bytes", "16"}, generator)
		if code != 0 {
			t.Fatalf("run() exit = %d, want 0", code)
		}
		if got, want := stdout, "AAECAwQFBgcICQoLDA0ODw==\n"; got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
		if generator.passphraseCount != 0 || generator.secretCount != 16 {
			t.Fatalf("generator calls = passphrase %d, secret %d", generator.passphraseCount, generator.secretCount)
		}
	})

	t.Run("generator failure emits no credential", func(t *testing.T) {
		generator := &fakeGenerator{secret: "partial-credential", err: errors.New("entropy source detail")}
		stdout, stderr, code := runGenerate(t, []string{"generate", "secret"}, generator)
		if code != 1 {
			t.Fatalf("run() exit = %d, want 1", code)
		}
		if stdout != "" {
			t.Fatalf("stdout = %q, want empty", stdout)
		}
		if got, want := stderr, "envseal: credential-generation-failed\n"; got != want {
			t.Fatalf("stderr = %q, want %q", got, want)
		}
	})

	t.Run("usage failure does not invoke generator", func(t *testing.T) {
		generator := &fakeGenerator{}
		stdout, stderr, code := runGenerate(t, []string{"generate", "secret", "--words", "6"}, generator)
		if code != 2 {
			t.Fatalf("run() exit = %d, want 2", code)
		}
		if stdout != "" || !strings.Contains(stderr, "envseal:") {
			t.Fatalf("output = stdout %q, stderr %q", stdout, stderr)
		}
		if generator.passphraseCount != 0 || generator.secretCount != 0 {
			t.Fatalf("generator was invoked")
		}
	})
}

func TestRunFailureAndNoWritePaths(t *testing.T) {
	const correct = "correct-password"
	t.Run("wrong password preserves source and forced output", func(t *testing.T) {
		directory := t.TempDir()
		source := filepath.Join(directory, ".env.example")
		target := filepath.Join(directory, ".env")
		original := sealedLine(t, "API_TOKEN", correct, "secret-value")
		writeTestFile(t, source, original)
		writeTestFile(t, target, "existing plaintext\n")
		stdout, stderr := runService(t, []string{"decrypt", "--force", source, target}, &fakePasswords{decrypt: []byte("wrong-password")})
		if stdout != "" || !strings.Contains(stderr, "transform-failed") {
			t.Fatalf("output = stdout %q, stderr %q", stdout, stderr)
		}
		if strings.Contains(stderr, "secret-value") || strings.Contains(stderr, "wrong-password") {
			t.Fatalf("diagnostic leaks secret: %q", stderr)
		}
		if got := readTestFile(t, source); got != original {
			t.Fatalf("source changed after authentication failure: %q", got)
		}
		if got := readTestFile(t, target); got != "existing plaintext\n" {
			t.Fatalf("target changed after authentication failure: %q", got)
		}
	})

	t.Run("malformed envelope creates no output", func(t *testing.T) {
		directory := t.TempDir()
		source := filepath.Join(directory, ".env.example")
		target := filepath.Join(directory, ".env")
		original := "API_TOKEN=ENVSEAL[not-valid]\n"
		writeTestFile(t, source, original)
		_, stderr := runService(t, []string{"decrypt", source, target}, &fakePasswords{decrypt: []byte(correct)})
		if !strings.Contains(stderr, "malformed-envelope") {
			t.Fatalf("stderr = %q", stderr)
		}
		if got := readTestFile(t, source); got != original {
			t.Fatalf("source changed after malformed envelope: %q", got)
		}
		if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("output exists or stat failed: %v", err)
		}
	})

	t.Run("missing and unsupported selected keys do not prompt or write", func(t *testing.T) {
		tests := []struct {
			name     string
			contents string
			key      string
			category string
		}{
			{name: "missing", contents: "PRESENT=value\n", key: "MISSING", category: "missing-key"},
			{name: "unsupported", contents: "export TOKEN=value\n", key: "TOKEN", category: "unsupported-syntax"},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				directory := t.TempDir()
				source := filepath.Join(directory, ".env.example")
				writeTestFile(t, source, test.contents)
				passwords := &fakePasswords{encrypt: []byte(correct)}
				_, stderr := runService(t, []string{"encrypt", source, test.key}, passwords)
				if !strings.Contains(stderr, test.category) {
					t.Fatalf("stderr = %q, want %s", stderr, test.category)
				}
				if passwords.calls != 0 {
					t.Fatalf("password prompts = %d, want 0", passwords.calls)
				}
				if got := readTestFile(t, source); got != test.contents {
					t.Fatalf("source changed: %q", got)
				}
			})
		}
	})

	t.Run("duplicate keys fail check without prompt", func(t *testing.T) {
		directory := t.TempDir()
		source := filepath.Join(directory, ".env.example")
		writeTestFile(t, source, "TOKEN=first\nTOKEN=second\n")
		passwords := &fakePasswords{}
		_, stderr := runService(t, []string{"check", source}, passwords)
		if strings.Count(stderr, "duplicate-key") != 2 || passwords.calls != 0 {
			t.Fatalf("stderr = %q, prompts = %d", stderr, passwords.calls)
		}
	})
}

func TestRunDecryptForceDryRunAndQuiet(t *testing.T) {
	const secret = "password"
	directory := t.TempDir()
	source := filepath.Join(directory, ".env.example")
	target := filepath.Join(directory, ".env")
	original := sealedLine(t, "TOKEN", secret, "plaintext")
	writeTestFile(t, source, original)
	writeTestFile(t, target, "old\n")

	passwords := &fakePasswords{decrypt: []byte(secret)}
	_, stderr := runService(t, []string{"decrypt", source, target}, passwords)
	if !strings.Contains(stderr, "target-exists") || passwords.calls != 0 {
		t.Fatalf("collision stderr = %q, prompts = %d", stderr, passwords.calls)
	}

	stdout, stderr := runService(t, []string{"decrypt", "--force", source, target}, &fakePasswords{decrypt: []byte(secret)})
	if stderr != "" || !strings.Contains(stdout, "decrypted 1 envelopes") {
		t.Fatalf("forced output = stdout %q, stderr %q", stdout, stderr)
	}
	if got := readTestFile(t, target); got != "TOKEN=plaintext\n" {
		t.Fatalf("forced target = %q", got)
	}

	stdout, stderr = runService(t, []string{"decrypt", "--dry-run", source}, &fakePasswords{decrypt: []byte(secret)})
	if stderr != "" || !strings.Contains(stdout, "verified 1 envelopes") {
		t.Fatalf("dry-run output = stdout %q, stderr %q", stdout, stderr)
	}
	if got := readTestFile(t, source); got != original {
		t.Fatalf("dry-run changed source: %q", got)
	}

	stdout, stderr = runService(t, []string{"decrypt", "--quiet", "--dry-run", source}, &fakePasswords{decrypt: []byte(secret)})
	if stdout != "" || stderr != "" {
		t.Fatalf("quiet dry-run output = stdout %q, stderr %q", stdout, stderr)
	}
}

func runService(t *testing.T, args []string, passwords *fakePasswords) (stdout, stderr string) {
	t.Helper()
	service := &service{readFile: os.ReadFile, passwords: passwords, writer: filewrite.New()}
	var out, err bytes.Buffer
	if code := run(args, "devel", &out, &err, service); code != 0 && code != 1 {
		t.Fatalf("run(%q) exit = %d", args, code)
	}
	return out.String(), err.String()
}

func runGenerate(t *testing.T, args []string, generator credentialGenerator) (stdout, stderr string, code int) {
	t.Helper()
	service := &service{generator: generator}
	var out, err bytes.Buffer
	code = run(args, "devel", &out, &err, service)
	return out.String(), err.String(), code
}

func sealedLine(t *testing.T, key, secret, plaintext string) string {
	t.Helper()
	encoded, err := envelope.Seal([]byte(secret), []byte(key), []byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}
	return key + "=" + encoded + "\n"
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

type fakeGenerator struct {
	passphrase      string
	secret          string
	err             error
	passphraseCount int
	secretCount     int
}

func (g *fakeGenerator) Passphrase(count int) (string, error) {
	g.passphraseCount = count
	return g.passphrase, g.err
}

func (g *fakeGenerator) Secret(count int) (string, error) {
	g.secretCount = count
	return g.secret, g.err
}

type fakePasswords struct {
	encrypt     []byte
	decrypt     []byte
	current     []byte
	replacement []byte
	err         error
	calls       int
}

func (p *fakePasswords) Encrypt() ([]byte, error) {
	p.calls++
	return append([]byte(nil), p.encrypt...), p.err
}
func (p *fakePasswords) Decrypt() ([]byte, error) {
	p.calls++
	return append([]byte(nil), p.decrypt...), p.err
}
func (p *fakePasswords) Rotate() ([]byte, []byte, error) {
	p.calls++
	return append([]byte(nil), p.current...), append([]byte(nil), p.replacement...), p.err
}
