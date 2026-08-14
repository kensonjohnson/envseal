package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	completionConfigStart = "# >>> envseal completion >>>"
	completionConfigEnd   = "# <<< envseal completion <<<"
)

type completionFileSystem interface {
	ReadFile(string) ([]byte, error)
	Lstat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	WriteAtomic(string, []byte, fs.FileMode) error
}

type completionOSFileSystem struct{}

func (completionOSFileSystem) ReadFile(path string) ([]byte, error)   { return os.ReadFile(path) }
func (completionOSFileSystem) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }
func (completionOSFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (completionOSFileSystem) WriteAtomic(path string, data []byte, mode fs.FileMode) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".envseal-completion-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

type completionInstaller interface {
	Plan(Request) (string, error)
	Install(Request) (string, error)
}

type completionInstallService struct {
	fs                     completionFileSystem
	home                   func() (string, error)
	getenv                 func(string) string
	bashCompletionDetected func() bool
	zshFpath               func() ([]string, error)
	powerShellProfile      func() (string, error)
}

func newCompletionInstaller() *completionInstallService {
	return &completionInstallService{
		fs:                     completionOSFileSystem{},
		home:                   os.UserHomeDir,
		getenv:                 os.Getenv,
		bashCompletionDetected: detectBashCompletion,
		zshFpath:               discoverZshFpath,
		powerShellProfile:      discoverPowerShellProfile,
	}
}

// Plan returns the explicit configuration change that Install will make before
// any filesystem write. Safe autoload installation and default fallback need no
// pre-write notice because they never modify a shell startup/profile file.
func (s *completionInstallService) Plan(req Request) (string, error) {
	if s == nil || s.fs == nil || s.home == nil || s.getenv == nil {
		return "", completionInstallError("completion-install-failed")
	}
	if !req.ConfigureShell {
		return "", nil
	}
	if _, ok := s.autoloadPath(req.Shell); ok {
		return "", nil
	}
	profile, err := s.profilePath(req.Shell)
	if err != nil {
		return "", err
	}
	script, err := s.configuredCompletionPath(req.Shell)
	if err != nil {
		return "", err
	}
	block := completionConfigBlock(req.Shell, script)
	return fmt.Sprintf("envseal: --configure-shell plan before writing:\n  write %s completion to %s\n  append this block to %s:\n%s", req.Shell, script, profile, block), nil
}

func (s *completionInstallService) Install(req Request) (string, error) {
	if s == nil || s.fs == nil || s.home == nil || s.getenv == nil {
		return "", completionInstallError("completion-install-failed")
	}

	if path, ok := s.autoloadPath(req.Shell); ok {
		if err := s.writeCompletion(path, req.Shell, s.autoloadCreatesDirectory(req.Shell)); err != nil {
			return "", err
		}
		return fmt.Sprintf("envseal: installed %s completion at %s\n", req.Shell, path), nil
	}
	if !req.ConfigureShell {
		return completionManualInstructions(req.Shell), nil
	}
	return s.configureShell(req.Shell)
}

func (s *completionInstallService) autoloadPath(shell string) (string, bool) {
	switch shell {
	case "fish":
		home, err := s.safeHome()
		if err != nil {
			return "", false
		}
		return filepath.Join(xdgDirectory(s.getenv("XDG_CONFIG_HOME"), filepath.Join(home, ".config")), "fish", "completions", "envseal.fish"), true
	case "bash":
		if s.bashCompletionDetected == nil || !s.bashCompletionDetected() {
			return "", false
		}
		home, err := s.safeHome()
		if err != nil {
			return "", false
		}
		directory := s.getenv("BASH_COMPLETION_USER_DIR")
		if !filepath.IsAbs(directory) {
			directory = filepath.Join(xdgDirectory(s.getenv("XDG_DATA_HOME"), filepath.Join(home, ".local", "share")), "bash-completion")
		}
		return filepath.Join(directory, "completions", "envseal.bash"), true
	case "zsh":
		if s.zshFpath == nil {
			return "", false
		}
		paths, err := s.zshFpath()
		if err != nil {
			return "", false
		}
		for _, path := range paths {
			if !filepath.IsAbs(path) || !s.writableDirectory(path) {
				continue
			}
			return filepath.Join(path, "_envseal"), true
		}
	}
	return "", false
}

func (s *completionInstallService) safeHome() (string, error) {
	home, err := s.home()
	if err != nil || !filepath.IsAbs(home) {
		return "", errors.New("home directory unavailable")
	}
	return filepath.Clean(home), nil
}

func (s *completionInstallService) autoloadCreatesDirectory(shell string) bool {
	return shell == "fish" || shell == "bash"
}

func (s *completionInstallService) writableDirectory(path string) bool {
	info, err := s.fs.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&0o222 != 0 && info.Mode()&fs.ModeSymlink == 0
}

func (s *completionInstallService) writeCompletion(path, shell string, createDirectory bool) error {
	if createDirectory {
		if err := s.fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return completionInstallError("completion-directory-create-failed")
		}
	}
	if info, err := s.fs.Lstat(path); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		return completionInstallError("unsafe-completion-target")
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return completionInstallError("completion-target-check-failed")
	}
	if err := s.fs.WriteAtomic(path, []byte(completionScript(shell)), 0o644); err != nil {
		return completionInstallError("completion-write-failed")
	}
	return nil
}

func (s *completionInstallService) configureShell(shell string) (string, error) {
	profile, err := s.profilePath(shell)
	if err != nil {
		return "", err
	}
	script, err := s.configuredCompletionPath(shell)
	if err != nil {
		return "", err
	}
	if err := s.writeCompletion(script, shell, true); err != nil {
		return "", err
	}

	changed, err := s.appendConfig(profile, completionConfigBlock(shell, script))
	if err != nil {
		return "", err
	}
	if !changed {
		return fmt.Sprintf("envseal: installed %s completion at %s; already configured in %s\n", shell, script, profile), nil
	}
	return fmt.Sprintf("envseal: installed %s completion at %s and configured %s\n", shell, script, profile), nil
}

func (s *completionInstallService) profilePath(shell string) (string, error) {
	switch shell {
	case "bash", "zsh":
		home, err := s.safeHome()
		if err != nil {
			return "", completionInstallError("shell-profile-discovery-failed")
		}
		name := ".bashrc"
		if shell == "zsh" {
			name = ".zshrc"
		}
		return filepath.Join(home, name), nil
	case "powershell":
		if s.powerShellProfile == nil {
			return "", completionInstallError("shell-profile-discovery-failed")
		}
		path, err := s.powerShellProfile()
		if err != nil || !filepath.IsAbs(path) {
			return "", completionInstallError("shell-profile-discovery-failed")
		}
		return filepath.Clean(path), nil
	default:
		return "", completionInstallError("completion-install-failed")
	}
}

func (s *completionInstallService) configuredCompletionPath(shell string) (string, error) {
	home, err := s.safeHome()
	if err != nil {
		return "", completionInstallError("completion-path-discovery-failed")
	}
	extension := map[string]string{"bash": ".bash", "zsh": ".zsh", "powershell": ".ps1"}[shell]
	if extension == "" {
		return "", completionInstallError("completion-install-failed")
	}
	directory := xdgDirectory(s.getenv("XDG_CONFIG_HOME"), filepath.Join(home, ".config"))
	return filepath.Join(directory, "envseal", "completions", "envseal"+extension), nil
}

func (s *completionInstallService) appendConfig(path, block string) (bool, error) {
	contents, err := s.fs.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, completionInstallError("shell-profile-read-failed")
	}
	text := string(contents)
	start := strings.Count(text, completionConfigStart)
	end := strings.Count(text, completionConfigEnd)
	if start == 1 && end == 1 {
		begin := strings.Index(text, completionConfigStart)
		finish := strings.Index(text, completionConfigEnd)
		if begin < finish && strings.Contains(text[begin:finish], blockCommand(block)) {
			return false, nil
		}
		return false, completionInstallError("shell-profile-config-conflict")
	}
	if start != 0 || end != 0 {
		return false, completionInstallError("shell-profile-config-conflict")
	}
	if err := s.fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, completionInstallError("shell-profile-directory-create-failed")
	}
	if info, statErr := s.fs.Lstat(path); statErr == nil && info.Mode()&fs.ModeSymlink != 0 {
		return false, completionInstallError("unsafe-shell-profile")
	} else if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return false, completionInstallError("shell-profile-check-failed")
	}
	if len(contents) > 0 && contents[len(contents)-1] != '\n' {
		contents = append(contents, '\n')
	}
	contents = append(contents, []byte(block)...)
	if err := s.fs.WriteAtomic(path, contents, 0o644); err != nil {
		return false, completionInstallError("shell-profile-write-failed")
	}
	return true, nil
}

func completionConfigBlock(shell, script string) string {
	command := "source " + shellQuote(script)
	if shell == "powershell" {
		command = ". " + powerShellQuote(script)
	}
	return completionConfigStart + "\n# Added by envseal completion install " + shell + " --configure-shell.\n" + command + "\n" + completionConfigEnd + "\n"
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

func powerShellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "''") + "'"
}

func blockCommand(block string) string {
	lines := strings.Split(block, "\n")
	if len(lines) < 3 {
		return ""
	}
	return lines[2]
}

func completionManualInstructions(shell string) string {
	if shell == "powershell" {
		return "envseal: PowerShell has no completion autoload directory; no files were changed.\nTo activate in this session, run:\n  go tool envseal completion powershell | Invoke-Expression\nTo persist, rerun with:\n  go tool envseal completion install powershell --configure-shell\n"
	}
	return fmt.Sprintf("envseal: no verified %s completion autoload directory; no files were changed.\nTo activate in this shell, run:\n  source <(go tool envseal completion %s)\nTo persist, rerun with:\n  go tool envseal completion install %s --configure-shell\n", shell, shell, shell)
}

type completionInstallFailure string

func (e completionInstallFailure) Error() string { return string(e) }

func completionInstallError(category string) error { return completionInstallFailure(category) }

func xdgDirectory(value, fallback string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return fallback
}

func detectBashCompletion() bool {
	for _, path := range bashCompletionFrameworkPaths() {
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func bashCompletionFrameworkPaths() []string {
	paths := []string{
		"/usr/share/bash-completion/bash_completion",
		"/usr/local/share/bash-completion/bash_completion",
		"/etc/profile.d/bash_completion.sh",
	}
	if runtime.GOOS == "darwin" {
		paths = append(paths, "/opt/homebrew/etc/profile.d/bash_completion.sh")
	}
	return paths
}

func discoverZshFpath() ([]string, error) {
	path, err := exec.LookPath("zsh")
	if err != nil {
		return nil, err
	}
	output, err := exec.Command(path, "-ic", "print -rl -- $fpath").Output()
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(string(output), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

func discoverPowerShellProfile() (string, error) {
	binary, err := exec.LookPath("pwsh")
	if err != nil {
		binary, err = exec.LookPath("powershell")
		if err != nil {
			return "", err
		}
	}
	output, err := exec.Command(binary, "-NoProfile", "-Command", "$PROFILE.CurrentUserCurrentHost").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
