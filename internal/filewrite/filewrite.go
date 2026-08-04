// Package filewrite performs Envseal's checked, same-directory replacement writes.
package filewrite

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

var (
	ErrInvalidSource = errors.New("invalid source file")
	ErrInvalidTarget = errors.New("invalid target file")
	ErrSamePath      = errors.New("source and target are the same file")
	ErrTargetExists  = errors.New("plaintext target already exists")
	ErrCreateTemp    = errors.New("temporary file creation failed")
	ErrWrite         = errors.New("temporary file write failed")
	ErrMode          = errors.New("temporary file mode update failed")
	ErrSync          = errors.New("temporary file sync failed")
	ErrClose         = errors.New("temporary file close failed")
	ErrReplace       = errors.New("file replacement failed")
	ErrDurability    = errors.New("replacement completed but directory sync failed")
)

type tempFile interface {
	io.Writer
	Name() string
	Chmod(fs.FileMode) error
	Sync() error
	Close() error
}

type filesystem interface {
	Lstat(string) (fs.FileInfo, error)
	CreateTemp(string, string) (tempFile, error)
	Rename(string, string) error
	Remove(string) error
}

type osFilesystem struct{}

func (osFilesystem) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }
func (osFilesystem) CreateTemp(dir, pattern string) (tempFile, error) {
	return os.CreateTemp(dir, pattern)
}
func (osFilesystem) Rename(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }
func (osFilesystem) Remove(path string) error             { return os.Remove(path) }

// Writer is a testable checked-write boundary. Its zero value is not ready for
// use; callers should use New.
type Writer struct {
	fs      filesystem
	syncDir func(string) error
}

// New returns the production writer.
func New() *Writer {
	return &Writer{fs: osFilesystem{}, syncDir: syncDirectory}
}

// ValidateSource requires a regular, non-symlink source before it is read.
func (w *Writer) ValidateSource(source string) error {
	_, err := w.regularSource(source)
	return err
}

// WriteEncrypted replaces a validated regular source with encrypted content,
// preserving its ordinary permission bits.
func (w *Writer) WriteEncrypted(source string, data []byte) error {
	info, err := w.regularSource(source)
	if err != nil {
		return err
	}
	return w.replace(source, data, info.Mode().Perm())
}

// ValidatePlaintextTarget rejects source/output collisions and disallowed
// existing output targets before a password is requested or plaintext is made.
func (w *Writer) ValidatePlaintextTarget(source, target string, force bool) error {
	sourceInfo, err := w.regularSource(source)
	if err != nil {
		return err
	}
	if sameCleanPath(source, target) {
		return ErrSamePath
	}

	targetInfo, err := w.fs.Lstat(target)
	switch {
	case err == nil:
		if !regular(targetInfo) {
			return ErrInvalidTarget
		}
		if os.SameFile(sourceInfo, targetInfo) {
			return ErrSamePath
		}
		if !force {
			return ErrTargetExists
		}
	case errors.Is(err, os.ErrNotExist):
		// A new output is allowed after the source-path equivalence check above.
	default:
		return ErrInvalidTarget
	}

	return nil
}

// WritePlaintext writes a decrypted output after validating a separate source.
// Existing outputs require force and are always replaced with mode 0600.
func (w *Writer) WritePlaintext(source, target string, data []byte, force bool) error {
	if err := w.ValidatePlaintextTarget(source, target, force); err != nil {
		return err
	}
	return w.replace(target, data, 0o600)
}

func (w *Writer) regularSource(path string) (fs.FileInfo, error) {
	if w == nil || w.fs == nil {
		return nil, ErrInvalidSource
	}
	info, err := w.fs.Lstat(path)
	if err != nil || !regular(info) {
		return nil, ErrInvalidSource
	}
	return info, nil
}

func regular(info fs.FileInfo) bool {
	if info == nil {
		return false
	}
	mode := info.Mode()
	return mode&fs.ModeSymlink == 0 && mode.IsRegular()
}

func sameCleanPath(first, second string) bool {
	firstAbs, firstErr := filepath.Abs(first)
	secondAbs, secondErr := filepath.Abs(second)
	if firstErr != nil || secondErr != nil {
		return filepath.Clean(first) == filepath.Clean(second)
	}
	return filepath.Clean(firstAbs) == filepath.Clean(secondAbs)
}

func (w *Writer) replace(target string, data []byte, mode fs.FileMode) error {
	if w == nil || w.fs == nil {
		return ErrCreateTemp
	}
	temporary, err := w.fs.CreateTemp(filepath.Dir(target), ".envseal-*")
	if err != nil {
		return ErrCreateTemp
	}

	closed := false
	replaced := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		if !replaced {
			_ = w.fs.Remove(temporary.Name())
		}
	}()

	written, err := temporary.Write(data)
	if err != nil || written != len(data) {
		return ErrWrite
	}
	if err := temporary.Chmod(mode); err != nil {
		return ErrMode
	}
	if err := temporary.Sync(); err != nil {
		return ErrSync
	}
	if err := temporary.Close(); err != nil {
		closed = true
		return ErrClose
	}
	closed = true
	if err := w.fs.Rename(temporary.Name(), target); err != nil {
		return ErrReplace
	}
	replaced = true
	if w.syncDir != nil {
		if err := w.syncDir(filepath.Dir(target)); err != nil {
			return ErrDurability
		}
	}
	return nil
}
