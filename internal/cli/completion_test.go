package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

func TestCompletionScriptsMatchCLIGrammar(t *testing.T) {
	bash := completionScript("bash")
	for _, required := range []string{
		"bash zsh fish powershell install", "--configure-shell --help",
		"if (( ! after_options )) && [[ \"$cur\" == -* ]]", "--words --bytes --help",
	} {
		if !strings.Contains(bash, required) {
			t.Fatalf("Bash completion does not contain %q", required)
		}
	}

	zsh := completionScript("zsh")
	for _, forbidden := range []string{"--words=", "--bytes="} {
		if strings.Contains(zsh, forbidden) {
			t.Fatalf("Zsh completion offers parser-invalid %q", forbidden)
		}
	}
	for _, required := range []string{
		"'--words[Passphrase word count]:count:'", "'--bytes[Secret byte count]:count:'",
		"if (( ${words[(I)--dry-run]} )); then", "'1:source file:_files'",
		"'1:target:(bash zsh fish powershell install)'",
	} {
		if !strings.Contains(zsh, required) {
			t.Fatalf("Zsh completion does not contain %q", required)
		}
	}

	fish := completionScript("fish")
	for _, required := range []string{
		"set -l after_options 0", "case --words --bytes", "__fish_envseal_completion_position 0",
		"__fish_envseal_completion_install; and __fish_envseal_completion_position 1",
		"__fish_envseal_command_is generate; and not __fish_seen_subcommand_from secret",
		"not __fish_seen_argument -l words", "not __fish_seen_argument -l bytes",
	} {
		if !strings.Contains(fish, required) {
			t.Fatalf("Fish completion does not contain %q", required)
		}
	}

	powershell := completionScript("powershell")
	for _, required := range []string{
		"$quoteCompletion =", "$value.Replace(\"'\", \"''\")",
		"CompletionResult]::new($insertionText, $_.Name", "$expectsOptionValue = $true",
		"'--' { $afterOptions = $true; $consumedOption = $true }", "'--configure-shell', '--help'",
	} {
		if !strings.Contains(powershell, required) {
			t.Fatalf("PowerShell completion does not contain %q", required)
		}
	}
}

func TestPowerShellFileCompletionQuotesAdversarialPaths(t *testing.T) {
	script := completionScript("powershell")
	for _, path := range []string{
		`C:\\tmp\\plain.env`,
		`C:\\tmp\\space name.env`,
		`C:\\tmp\\quote' ; Remove-Item -Recurse -Force C:\\`,
		`C:\\tmp\\$(Invoke-Expression 'bad').env`,
	} {
		want := "'" + strings.ReplaceAll(path, "'", "''") + "'"
		if !strings.HasPrefix(want, "'") || !strings.HasSuffix(want, "'") || (strings.Contains(path, "'") && !strings.Contains(want, "''")) {
			t.Fatalf("path %q is not safely single-quoted as %q", path, want)
		}
	}
	if strings.Contains(script, "CompletionResult]::new($_.FullName") {
		t.Fatal("PowerShell file completion must never use an unquoted path as insertion text")
	}
}

func TestBashCompletionBehavior(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is unavailable")
	}

	temp := t.TempDir()
	completion := filepath.Join(temp, "envseal.bash")
	if err := os.WriteFile(completion, []byte(completionScript("bash")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temp, "-dash-prefixed.env"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	run := func(words ...string) string {
		t.Helper()
		command := `source "$1"
shift
COMP_WORDS=("$@")
COMP_CWORD=$((${#COMP_WORDS[@]} - 1))
_envseal_complete
printf '%s\n' "${COMPREPLY[@]}"`
		result := exec.Command("bash", "-c", command, "bash", completion)
		result.Args = append(result.Args, words...)
		result.Dir = temp
		output, err := result.CombinedOutput()
		if err != nil {
			t.Fatalf("bash completion %q: %v\n%s", words, err, output)
		}
		return string(output)
	}

	if got := run("envseal", "completion", "install", ""); got != "bash\nzsh\nfish\npowershell\n" {
		t.Fatalf("completion install shell candidates = %q", got)
	}
	if got := run("envseal", "completion", "install", "bash", "--"); !strings.Contains(got, "--configure-shell") {
		t.Fatalf("completion install option candidates = %q", got)
	}
	if got := run("envseal", "generate", "--"); !strings.Contains(got, "--words") || !strings.Contains(got, "--bytes") {
		t.Fatalf("generate pre-mode option candidates = %q", got)
	}
	if got := run("envseal", "encrypt", "--", "-dash"); got != "-dash-prefixed.env\n" {
		t.Fatalf("dash-prefixed path candidate after -- = %q", got)
	}
}

func TestCompletionScriptsShellSyntax(t *testing.T) {
	for shell, flag := range map[string]string{"bash": "-n", "zsh": "-n"} {
		shell, flag := shell, flag
		t.Run(shell, func(t *testing.T) {
			if _, err := exec.LookPath(shell); err != nil {
				t.Skip(shell + " is unavailable")
			}
			path := filepath.Join(t.TempDir(), "completion")
			if err := os.WriteFile(path, []byte(completionScript(shell)), 0o600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(shell, flag, path).CombinedOutput(); err != nil {
				t.Fatalf("%s syntax check: %v\n%s", shell, err, output)
			}
			var command *exec.Cmd
			if shell == "bash" {
				command = exec.Command("bash", "-c", `source "$1"; complete -p envseal`, "bash", path)
			} else {
				command = exec.Command("zsh", "-f", "-c", `autoload -Uz compinit; compinit -D; source "$1"; (( $+functions[_envseal] ))`, "zsh", path)
			}
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("%s completion registration check: %v\n%s", shell, err, output)
			}
		})
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
