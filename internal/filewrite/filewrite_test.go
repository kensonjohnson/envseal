package filewrite

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteEncryptedFailureLeavesOriginalUntouched(t *testing.T) {
	failures := []struct {
		name   string
		file   func(*os.File) tempFile
		rename func(string, string) error
		want   error
	}{
		{name: "write", file: func(file *os.File) tempFile { return failingTemp{File: file, writeErr: errors.New("write")} }, want: ErrWrite},
		{name: "sync", file: func(file *os.File) tempFile { return failingTemp{File: file, syncErr: errors.New("sync")} }, want: ErrSync},
		{name: "close", file: func(file *os.File) tempFile { return failingTemp{File: file, closeErr: errors.New("close")} }, want: ErrClose},
		{name: "rename", file: func(file *os.File) tempFile { return failingTemp{File: file} }, rename: func(string, string) error { return errors.New("rename") }, want: ErrReplace},
	}

	for _, test := range failures {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			target := filepath.Join(directory, ".env.example")
			if err := os.WriteFile(target, []byte("original"), 0o640); err != nil {
				t.Fatal(err)
			}
			fileSystem := &faultFilesystem{file: test.file, rename: test.rename}
			writer := &Writer{fs: fileSystem, syncDir: func(string) error { return nil }}
			err := writer.WriteEncrypted(target, []byte("replacement"))
			if !errors.Is(err, test.want) {
				t.Fatalf("WriteEncrypted() error = %v, want %v", err, test.want)
			}
			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "original" {
				t.Fatalf("target = %q, want original bytes", got)
			}
			assertNoTempFiles(t, directory)
		})
	}
}

func TestWriteEncryptedPreservesModeAndUsesTargetDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions are ACL controlled")
	}
	directory := filepath.Join(t.TempDir(), "nested")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, ".env.example")
	if err := os.WriteFile(target, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o640); err != nil {
		t.Fatal(err)
	}
	fileSystem := &faultFilesystem{file: func(file *os.File) tempFile { return failingTemp{File: file} }}
	writer := &Writer{fs: fileSystem, syncDir: func(string) error { return nil }}
	if err := writer.WriteEncrypted(target, []byte("new")); err != nil {
		t.Fatalf("WriteEncrypted() error = %v", err)
	}
	if got := fileSystem.createDirectory; got != directory {
		t.Fatalf("CreateTemp directory = %q, want %q", got, directory)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), fs.FileMode(0o640); got != want {
		t.Fatalf("target mode = %#o, want %#o", got, want)
	}
}

func TestWritePlaintextProtectionChecks(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, ".env.example")
	if err := os.WriteFile(source, []byte("encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	writer := &Writer{fs: osFilesystem{}, syncDir: func(string) error { return nil }}

	if err := writer.WritePlaintext(source, source, []byte("plaintext"), false); !errors.Is(err, ErrSamePath) {
		t.Fatalf("same path error = %v, want %v", err, ErrSamePath)
	}

	target := filepath.Join(directory, ".env.local")
	if err := os.WriteFile(target, []byte("old plaintext"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writer.WritePlaintext(source, target, []byte("new plaintext"), false); !errors.Is(err, ErrTargetExists) {
		t.Fatalf("existing target error = %v, want %v", err, ErrTargetExists)
	}
	if got, _ := os.ReadFile(target); string(got) != "old plaintext" {
		t.Fatalf("target was changed without force: %q", got)
	}
	if err := writer.WritePlaintext(source, target, []byte("new plaintext"), true); err != nil {
		t.Fatalf("forced WritePlaintext() error = %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "new plaintext" {
		t.Fatalf("forced target = %q", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := info.Mode().Perm(), fs.FileMode(0o600); got != want {
			t.Fatalf("plaintext mode = %#o, want %#o", got, want)
		}
	}
}

func TestRejectsSymlinkSourceAndTarget(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writer := &Writer{fs: osFilesystem{}, syncDir: func(string) error { return nil }}
	if err := writer.WriteEncrypted(link, []byte("replacement")); !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("symlink source error = %v, want %v", err, ErrInvalidSource)
	}
	if err := writer.WritePlaintext(source, link, []byte("plaintext"), true); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("symlink target error = %v, want %v", err, ErrInvalidTarget)
	}
}

func TestDirectorySyncFailureReportsAfterReplacement(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, ".env.example")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	writer := &Writer{fs: osFilesystem{}, syncDir: func(string) error { return errors.New("directory sync") }}
	err := writer.WriteEncrypted(target, []byte("new"))
	if !errors.Is(err, ErrDurability) {
		t.Fatalf("WriteEncrypted() error = %v, want %v", err, ErrDurability)
	}
	if got, _ := os.ReadFile(target); string(got) != "new" {
		t.Fatalf("target = %q, want new replacement", got)
	}
}

type faultFilesystem struct {
	file            func(*os.File) tempFile
	rename          func(string, string) error
	createDirectory string
}

func (f *faultFilesystem) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }
func (f *faultFilesystem) CreateTemp(directory, pattern string) (tempFile, error) {
	f.createDirectory = directory
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return nil, err
	}
	return f.file(file), nil
}
func (f *faultFilesystem) Rename(oldpath, newpath string) error {
	if f.rename != nil {
		return f.rename(oldpath, newpath)
	}
	return os.Rename(oldpath, newpath)
}
func (f *faultFilesystem) Remove(path string) error { return os.Remove(path) }

type failingTemp struct {
	*os.File
	writeErr error
	syncErr  error
	closeErr error
}

func (f failingTemp) Write(data []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.File.Write(data)
}
func (f failingTemp) Sync() error {
	if f.syncErr != nil {
		return f.syncErr
	}
	return f.File.Sync()
}
func (f failingTemp) Close() error {
	err := f.File.Close()
	if f.closeErr != nil {
		return f.closeErr
	}
	return err
}

func assertNoTempFiles(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".envseal-") {
			t.Fatalf("temporary file remains: %s", entry.Name())
		}
	}
}
