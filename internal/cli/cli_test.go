package cli

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

func TestParseValidRequests(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want Request
	}{
		{
			name: "root help",
			args: []string{"--help"},
			want: Request{Command: CommandHelp},
		},
		{
			name: "version",
			args: []string{"--version"},
			want: Request{Command: CommandVersion},
		},
		{
			name: "encrypt selected keys",
			args: []string{"encrypt", ".env.example", "API_TOKEN", "DATABASE_URL"},
			want: Request{Command: CommandEncrypt, Source: ".env.example", Keys: []string{"API_TOKEN", "DATABASE_URL"}},
		},
		{
			name: "encrypt unusual key after separator",
			args: []string{"encrypt", "--quiet", "--", ".env.example", "-KEY"},
			want: Request{Command: CommandEncrypt, Source: ".env.example", Keys: []string{"-KEY"}, Quiet: true},
		},
		{
			name: "decrypt",
			args: []string{"decrypt", "--force", ".env.example", ".env.local"},
			want: Request{Command: CommandDecrypt, Source: ".env.example", Output: ".env.local", Force: true},
		},
		{
			name: "decrypt dry run",
			args: []string{"decrypt", "--quiet", "--dry-run", ".env.example"},
			want: Request{Command: CommandDecrypt, Source: ".env.example", Quiet: true, DryRun: true},
		},
		{
			name: "rotate",
			args: []string{"rotate", ".env.example"},
			want: Request{Command: CommandRotate, Source: ".env.example"},
		},
		{
			name: "check",
			args: []string{"check", "--quiet", ".env.example"},
			want: Request{Command: CommandCheck, Source: ".env.example", Quiet: true},
		},
		{
			name: "command help",
			args: []string{"decrypt", "--help"},
			want: Request{Command: CommandHelp, HelpFor: CommandDecrypt},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Parse(test.args)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Parse() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestParseRejectsInvalidShapes(t *testing.T) {
	tests := [][]string{
		nil,
		{"unknown"},
		{"--help", "encrypt"},
		{"--version", "extra"},
		{"encrypt", ".env.example"},
		{"encrypt", "--force", ".env.example", "KEY"},
		{"encrypt", "--wat", ".env.example", "KEY"},
		{"decrypt", ".env.example"},
		{"decrypt", "--dry-run", ".env.example", ".env.local"},
		{"decrypt", "--force", "--dry-run", ".env.example"},
		{"rotate", "--dry-run", ".env.example"},
		{"check", ".env.example", "extra"},
		{"encrypt", "", "KEY"},
		{"encrypt", ".env.example", ""},
		{"decrypt", ".env.example", ""},
	}

	for _, args := range tests {
		t.Run("invalid", func(t *testing.T) {
			_, err := Parse(args)
			if err == nil {
				t.Fatal("Parse() error = nil")
			}
			var usage *UsageError
			if !errors.As(err, &usage) {
				t.Fatalf("Parse() error type = %T, want *UsageError", err)
			}
		})
	}
}

func TestRunHelpVersionAndUsageError(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if got := Run([]string{"check", "--help"}, "v1.20260803.1", &stdout, &stderr); got != 0 {
			t.Fatalf("Run() exit = %d, want 0", got)
		}
		if stdout.Len() == 0 || stderr.Len() != 0 {
			t.Fatalf("help output = stdout %q, stderr %q", stdout.String(), stderr.String())
		}
	})

	t.Run("version", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if got := Run([]string{"--version"}, "v1.20260803.1", &stdout, &stderr); got != 0 {
			t.Fatalf("Run() exit = %d, want 0", got)
		}
		if got, want := stdout.String(), "envseal v1.20260803.1\n"; got != want {
			t.Fatalf("version output = %q, want %q", got, want)
		}
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q, want empty", stderr.String())
		}
	})

	t.Run("usage error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if got := Run([]string{"decrypt"}, "devel", &stdout, &stderr); got != 2 {
			t.Fatalf("Run() exit = %d, want 2", got)
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout = %q, want empty", stdout.String())
		}
		if got := stderr.String(); got == "" || !bytes.Contains(stderr.Bytes(), []byte("envseal:")) {
			t.Fatalf("stderr = %q, want usage diagnostic", got)
		}
	})
}
