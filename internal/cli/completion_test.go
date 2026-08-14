package cli

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseCompletion(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			got, err := Parse([]string{"completion", shell})
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			want := Request{Command: CommandCompletion, Shell: shell}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Parse() = %#v, want %#v", got, want)
			}
		})
	}

	got, err := Parse([]string{"completion", "--help"})
	if err != nil {
		t.Fatalf("Parse(completion help) error = %v", err)
	}
	if want := (Request{Command: CommandHelp, HelpFor: CommandCompletion}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse(completion help) = %#v, want %#v", got, want)
	}
}

func TestParseCompletionRejectsInvalidShell(t *testing.T) {
	for _, args := range [][]string{
		{"completion"},
		{"completion", "elvish"},
		{"completion", "bash", "extra"},
		{"completion", "--wat"},
	} {
		_, err := Parse(args)
		var usage *UsageError
		if !errors.As(err, &usage) {
			t.Fatalf("Parse(%q) error = %v, want *UsageError", args, err)
		}
	}
}

func TestCompletionUsageAndRendering(t *testing.T) {
	usage := Usage(CommandCompletion)
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		if !strings.Contains(usage, shell) {
			t.Fatalf("completion help does not document %q: %q", shell, usage)
		}
	}
	if !strings.Contains(Usage(""), "envseal completion <bash|zsh|fish|powershell>") {
		t.Fatalf("root help does not document completion: %q", Usage(""))
	}

	tests := []struct {
		shell      string
		nativeHook string
		fileHook   string
	}{
		{shell: "bash", nativeHook: "complete -F _envseal_complete envseal", fileHook: "compgen -f"},
		{shell: "zsh", nativeHook: "compdef _envseal envseal", fileHook: "_files"},
		{shell: "fish", nativeHook: "complete -c envseal", fileHook: "complete -c envseal -n '__fish_envseal_file_position' -F"},
		{shell: "powershell", nativeHook: "Register-ArgumentCompleter -Native -CommandName envseal", fileHook: "Get-ChildItem"},
	}
	for _, test := range tests {
		t.Run(test.shell, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run([]string{"completion", test.shell}, "devel", &stdout, &stderr, nil); code != 0 {
				t.Fatalf("run() exit = %d, want 0", code)
			}
			script := stdout.String()
			if script == "" || stderr.Len() != 0 {
				t.Fatalf("output = stdout %q, stderr %q", script, stderr.String())
			}
			for _, required := range []string{
				"go tool envseal", test.nativeHook, test.fileHook,
				"encrypt", "decrypt", "rotate", "check", "generate", "completion",
				"quiet", "force", "dry-run", "words", "bytes", "passphrase", "secret",
			} {
				if !strings.Contains(script, required) {
					t.Fatalf("script does not contain %q", required)
				}
			}
			for _, forbidden := range []string{"dotenv", "ReadFile", "Get-Content"} {
				if strings.Contains(script, forbidden) {
					t.Fatalf("script must not inspect dotenv keys or values: found %q", forbidden)
				}
			}
		})
	}
}

func TestZshRootCommandsUseDescriptionAwareCompletion(t *testing.T) {
	script := completionScript("zsh")
	if !strings.Contains(script, "_describe 'command' commands") {
		t.Fatal("Zsh root commands must use _describe so command text and descriptions are separate")
	}
	if strings.Contains(script, "compadd -a commands") {
		t.Fatal("Zsh root commands must not pass name:description candidates directly to compadd")
	}
}

func TestFishCompletionUsesForcedFilesOnlyAtFilePositions(t *testing.T) {
	script := completionScript("fish")
	if strings.Contains(script, "__fish_complete_path") {
		t.Fatalf("Fish completion must not use __fish_complete_path as a candidate command")
	}
	if !strings.Contains(script, "complete -c envseal -f") {
		t.Fatal("Fish completion must disable default file candidates")
	}
	if !strings.Contains(script, "complete -c envseal -n '__fish_envseal_file_position' -F") {
		t.Fatal("Fish completion must force file candidates only at source/output positions")
	}
}

func TestRunCompletionUsageErrors(t *testing.T) {
	for _, args := range [][]string{{"completion"}, {"completion", "unknown"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, "devel", &stdout, &stderr); code != 2 {
			t.Fatalf("Run(%q) exit = %d, want 2", args, code)
		}
		if stdout.Len() != 0 || !strings.Contains(stderr.String(), "envseal:") {
			t.Fatalf("Run(%q) output = stdout %q, stderr %q", args, stdout.String(), stderr.String())
		}
	}
}
