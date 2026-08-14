package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseCompletionInstall(t *testing.T) {
	got, err := Parse([]string{"completion", "install", "bash", "--configure-shell"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := Request{Command: CommandCompletionInstall, Shell: "bash", ConfigureShell: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
	for _, args := range [][]string{
		{"completion", "install"},
		{"completion", "install", "bash", "bash"},
		{"completion", "install", "bash", "--configure-shell", "--configure-shell"},
		{"completion", "install", "elvish"},
	} {
		if _, err := Parse(args); err == nil {
			t.Fatalf("Parse(%q) error = nil", args)
		}
	}
}

func TestCompletionInstallUsesVerifiedAutoloadLocations(t *testing.T) {
	t.Run("fish", func(t *testing.T) {
		home := t.TempDir()
		installer, filesystem := testCompletionInstaller(home)
		installer.getenv = func(key string) string {
			if key == "XDG_CONFIG_HOME" {
				return filepath.Join(home, "config")
			}
			return ""
		}
		output, err := installer.Install(Request{Command: CommandCompletionInstall, Shell: "fish"})
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(home, "config", "fish", "completions", "envseal.fish")
		if output != "envseal: installed fish completion at "+path+"\n" {
			t.Fatalf("output = %q", output)
		}
		if got := readTestFile(t, path); got != completionScript("fish") {
			t.Fatalf("completion contents differ")
		}
		if len(filesystem.writes) != 1 || filesystem.writes[0] != path {
			t.Fatalf("writes = %q", filesystem.writes)
		}
	})

	t.Run("bash only after framework detection", func(t *testing.T) {
		home := t.TempDir()
		installer, _ := testCompletionInstaller(home)
		installer.bashCompletionDetected = func() bool { return true }
		output, err := installer.Install(Request{Command: CommandCompletionInstall, Shell: "bash"})
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(home, ".local", "share", "bash-completion", "completions", "envseal.bash")
		if output != "envseal: installed bash completion at "+path+"\n" {
			t.Fatalf("output = %q", output)
		}
		if got := readTestFile(t, path); got != completionScript("bash") {
			t.Fatalf("completion contents differ")
		}
	})

	t.Run("zsh existing writable fpath", func(t *testing.T) {
		home := t.TempDir()
		fpath := filepath.Join(home, "site-functions")
		if err := os.Mkdir(fpath, 0o755); err != nil {
			t.Fatal(err)
		}
		installer, _ := testCompletionInstaller(home)
		installer.zshFpath = func() ([]string, error) { return []string{filepath.Join(home, "missing"), fpath}, nil }
		output, err := installer.Install(Request{Command: CommandCompletionInstall, Shell: "zsh"})
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(fpath, "_envseal")
		if output != "envseal: installed zsh completion at "+path+"\n" {
			t.Fatalf("output = %q", output)
		}
		if got := readTestFile(t, path); got != completionScript("zsh") {
			t.Fatalf("completion contents differ")
		}
	})
}

func TestCompletionInstallFallbackDoesNotWrite(t *testing.T) {
	home := t.TempDir()
	installer, filesystem := testCompletionInstaller(home)
	installer.bashCompletionDetected = func() bool { return false }
	output, err := installer.Install(Request{Command: CommandCompletionInstall, Shell: "bash"})
	if err != nil {
		t.Fatal(err)
	}
	want := "envseal: no verified bash completion autoload directory; no files were changed.\nTo activate in this shell, run:\n  source <(go tool envseal completion bash)\nTo persist, rerun with:\n  go tool envseal completion install bash --configure-shell\n"
	if output != want {
		t.Fatalf("output = %q, want %q", output, want)
	}
	if len(filesystem.writes) != 0 {
		t.Fatalf("writes = %q, want none", filesystem.writes)
	}
}

func TestPowerShellDefaultDoesNotWrite(t *testing.T) {
	installer, filesystem := testCompletionInstaller(t.TempDir())
	output, err := installer.Install(Request{Command: CommandCompletionInstall, Shell: "powershell"})
	if err != nil {
		t.Fatal(err)
	}
	want := "envseal: PowerShell has no completion autoload directory; no files were changed.\nTo activate in this session, run:\n  go tool envseal completion powershell | Invoke-Expression\nTo persist, rerun with:\n  go tool envseal completion install powershell --configure-shell\n"
	if output != want {
		t.Fatalf("output = %q, want %q", output, want)
	}
	if len(filesystem.writes) != 0 {
		t.Fatalf("writes = %q, want none", filesystem.writes)
	}
}

func TestCompletionConfigureShellWritesDirectBinaryIndependentScriptAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, ".bashrc")
	script := filepath.Join(home, ".config", "envseal", "completions", "envseal.bash")
	writeTestFile(t, profile, "export KEEP_ME=1\n")
	installer, filesystem := testCompletionInstaller(home)
	installer.bashCompletionDetected = func() bool { return false }
	req := Request{Command: CommandCompletionInstall, Shell: "bash", ConfigureShell: true}
	output, err := installer.Install(req)
	if err != nil {
		t.Fatal(err)
	}
	if output != "envseal: installed bash completion at "+script+" and configured "+profile+"\n" {
		t.Fatalf("output = %q", output)
	}
	if got := readTestFile(t, script); got != completionScript("bash") {
		t.Fatalf("configured script differs")
	}
	contents := readTestFile(t, profile)
	wantSource := "source '" + script + "'"
	if !strings.HasPrefix(contents, "export KEEP_ME=1\n") || strings.Count(contents, completionConfigStart) != 1 || !strings.Contains(contents, wantSource) || strings.Contains(contents, "go tool envseal") {
		t.Fatalf("profile = %q", contents)
	}
	output, err = installer.Install(req)
	if err != nil {
		t.Fatal(err)
	}
	if output != "envseal: installed bash completion at "+script+"; already configured in "+profile+"\n" {
		t.Fatalf("second output = %q", output)
	}
	if strings.Count(readTestFile(t, profile), completionConfigStart) != 1 {
		t.Fatal("profile block was duplicated")
	}
	if len(filesystem.writes) != 3 || filesystem.writes[0] != script || filesystem.writes[1] != profile || filesystem.writes[2] != script {
		t.Fatalf("writes = %q, want script, profile, script", filesystem.writes)
	}
}

func TestRunConfigureShellPrintsPlanBeforeWrites(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		home := t.TempDir()
		profile := filepath.Join(home, ".bashrc")
		script := filepath.Join(home, ".config", "envseal", "completions", "envseal.bash")
		installer, filesystem := testCompletionInstaller(home)
		installer.bashCompletionDetected = func() bool { return false }
		req := Request{Command: CommandCompletionInstall, Shell: "bash", ConfigureShell: true}
		notice, err := installer.Plan(req)
		if err != nil {
			t.Fatal(err)
		}
		wantNotice := "envseal: --configure-shell plan before writing:\n  write bash completion to " + script + "\n  append this block to " + profile + ":\n" + completionConfigBlock("bash", script)
		if notice != wantNotice || len(filesystem.writes) != 0 {
			t.Fatalf("notice = %q, writes = %q", notice, filesystem.writes)
		}
		var stdout, stderr bytes.Buffer
		if code := runWithCompletionInstaller([]string{"completion", "install", "bash", "--configure-shell"}, "devel", &stdout, &stderr, nil, installer); code != 0 {
			t.Fatalf("run() exit = %d, stderr = %q", code, stderr.String())
		}
		wantResult := "envseal: installed bash completion at " + script + " and configured " + profile + "\n"
		if got := stdout.String(); got != wantNotice+wantResult {
			t.Fatalf("stdout = %q, want notice before result", got)
		}
		if len(filesystem.writes) != 2 || filesystem.writes[0] != script || filesystem.writes[1] != profile {
			t.Fatalf("writes = %q, want script then profile", filesystem.writes)
		}
	})

	t.Run("profile failure", func(t *testing.T) {
		home := t.TempDir()
		profile := filepath.Join(home, ".bashrc")
		script := filepath.Join(home, ".config", "envseal", "completions", "envseal.bash")
		writeTestFile(t, profile, "export KEEP_ME=1\n")
		installer, filesystem := testCompletionInstaller(home)
		installer.bashCompletionDetected = func() bool { return false }
		filesystem.writeErrPath = profile
		var stdout, stderr bytes.Buffer
		code := runWithCompletionInstaller([]string{"completion", "install", "bash", "--configure-shell"}, "devel", &stdout, &stderr, nil, installer)
		wantNotice := "envseal: --configure-shell plan before writing:\n  write bash completion to " + script + "\n  append this block to " + profile + ":\n" + completionConfigBlock("bash", script)
		if code != 1 || stdout.String() != wantNotice || stderr.String() != "envseal: shell-profile-write-failed\n" {
			t.Fatalf("code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
		}
		if got := readTestFile(t, profile); got != "export KEEP_ME=1\n" {
			t.Fatalf("profile changed after planned failure: %q", got)
		}
		if got := readTestFile(t, script); got != completionScript("bash") {
			t.Fatalf("script = %q", got)
		}
	})
}

func TestPowerShellConfigureShellDotSourcesStableScript(t *testing.T) {
	home := t.TempDir()
	profile := filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
	script := filepath.Join(home, ".config", "envseal", "completions", "envseal.ps1")
	installer, _ := testCompletionInstaller(home)
	installer.powerShellProfile = func() (string, error) { return profile, nil }
	output, err := installer.Install(Request{Command: CommandCompletionInstall, Shell: "powershell", ConfigureShell: true})
	if err != nil {
		t.Fatal(err)
	}
	if output != "envseal: installed powershell completion at "+script+" and configured "+profile+"\n" {
		t.Fatalf("output = %q", output)
	}
	if got := readTestFile(t, script); got != completionScript("powershell") {
		t.Fatalf("configured script differs")
	}
	contents := readTestFile(t, profile)
	if !strings.Contains(contents, ". '"+script+"'") || strings.Contains(contents, "go tool envseal") {
		t.Fatalf("profile = %q", contents)
	}
}

func TestCompletionInstallWriteFailuresDoNotPartiallyChangeExistingFiles(t *testing.T) {
	t.Run("configured script leaves profile untouched", func(t *testing.T) {
		home := t.TempDir()
		profile := filepath.Join(home, ".bashrc")
		script := filepath.Join(home, ".config", "envseal", "completions", "envseal.bash")
		writeTestFile(t, profile, "export KEEP_ME=1\n")
		installer, filesystem := testCompletionInstaller(home)
		installer.bashCompletionDetected = func() bool { return false }
		filesystem.writeErrPath = script
		_, err := installer.Install(Request{Command: CommandCompletionInstall, Shell: "bash", ConfigureShell: true})
		if err == nil || err.Error() != "completion-write-failed" {
			t.Fatalf("error = %v", err)
		}
		if got := readTestFile(t, profile); got != "export KEEP_ME=1\n" {
			t.Fatalf("profile changed after failed script write: %q", got)
		}
	})

	t.Run("completion file", func(t *testing.T) {
		home := t.TempDir()
		path := filepath.Join(home, ".config", "fish", "completions", "envseal.fish")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, path, "old completion\n")
		installer, filesystem := testCompletionInstaller(home)
		filesystem.writeErr = errors.New("disk full")
		_, err := installer.Install(Request{Command: CommandCompletionInstall, Shell: "fish"})
		if err == nil || err.Error() != "completion-write-failed" {
			t.Fatalf("error = %v", err)
		}
		if got := readTestFile(t, path); got != "old completion\n" {
			t.Fatalf("completion changed after failed write: %q", got)
		}
	})

	t.Run("profile", func(t *testing.T) {
		home := t.TempDir()
		path := filepath.Join(home, ".bashrc")
		script := filepath.Join(home, ".config", "envseal", "completions", "envseal.bash")
		writeTestFile(t, path, "export KEEP_ME=1\n")
		installer, filesystem := testCompletionInstaller(home)
		installer.bashCompletionDetected = func() bool { return false }
		filesystem.writeErrPath = path
		_, err := installer.Install(Request{Command: CommandCompletionInstall, Shell: "bash", ConfigureShell: true})
		if err == nil || err.Error() != "shell-profile-write-failed" {
			t.Fatalf("error = %v", err)
		}
		if got := readTestFile(t, path); got != "export KEEP_ME=1\n" {
			t.Fatalf("profile changed after failed write: %q", got)
		}
		if got := readTestFile(t, script); got != completionScript("bash") {
			t.Fatalf("configured script = %q", got)
		}
	})
}

func TestRunCompletionInstallError(t *testing.T) {
	installer, filesystem := testCompletionInstaller(t.TempDir())
	filesystem.writeErr = errors.New("disk full")
	var stdout, stderr bytes.Buffer
	code := runWithCompletionInstaller([]string{"completion", "install", "fish"}, "devel", &stdout, &stderr, nil, installer)
	if code != 1 || stdout.Len() != 0 || stderr.String() != "envseal: completion-write-failed\n" {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

type testCompletionFileSystem struct {
	writeErr     error
	writeErrPath string
	writes       []string
}

func (f *testCompletionFileSystem) ReadFile(path string) ([]byte, error)   { return os.ReadFile(path) }
func (f *testCompletionFileSystem) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }
func (f *testCompletionFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (f *testCompletionFileSystem) WriteAtomic(path string, data []byte, mode fs.FileMode) error {
	f.writes = append(f.writes, path)
	if f.writeErr != nil || path == f.writeErrPath {
		if f.writeErr != nil {
			return f.writeErr
		}
		return errors.New("disk full")
	}
	return (completionOSFileSystem{}).WriteAtomic(path, data, mode)
}

func testCompletionInstaller(home string) (*completionInstallService, *testCompletionFileSystem) {
	filesystem := &testCompletionFileSystem{}
	return &completionInstallService{
		fs:                     filesystem,
		home:                   func() (string, error) { return home, nil },
		getenv:                 func(string) string { return "" },
		bashCompletionDetected: func() bool { return false },
		zshFpath:               func() ([]string, error) { return nil, nil },
		powerShellProfile:      func() (string, error) { return "", errors.New("not installed") },
	}, filesystem
}
