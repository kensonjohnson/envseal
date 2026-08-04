// Package password provides Envseal's terminal-only password prompts.
package password

import (
	"crypto/subtle"
	"errors"
	"io"
	"syscall"

	"golang.org/x/term"
)

var (
	// ErrTerminalUnavailable means Envseal could not use a controlling terminal.
	ErrTerminalUnavailable = errors.New("controlling terminal unavailable")
	// ErrBlankPassword means a prompt received no password bytes.
	ErrBlankPassword = errors.New("blank password")
	// ErrPasswordMismatch means a new-password confirmation differed.
	ErrPasswordMismatch = errors.New("password confirmation mismatch")
	// ErrPasswordCancelled means input ended or was interrupted before completion.
	ErrPasswordCancelled = errors.New("password input cancelled")
	// ErrPasswordRead means password input failed without exposing its cause.
	ErrPasswordRead = errors.New("password input failed")
)

type terminalSession interface {
	InputFD() int
	OutputFD() int
	Write([]byte) (int, error)
	Close() error
}

type terminalReader interface {
	IsTerminal(fd int) bool
	ReadPassword(fd int) ([]byte, error)
}

type xtermReader struct{}

func (xtermReader) IsTerminal(fd int) bool              { return term.IsTerminal(fd) }
func (xtermReader) ReadPassword(fd int) ([]byte, error) { return term.ReadPassword(fd) }

// Provider reads passwords only from the process's controlling terminal.
type Provider struct {
	open   func() (terminalSession, error)
	reader terminalReader
}

// New returns the production terminal-only password provider.
func New() *Provider {
	return &Provider{open: openControllingTerminal, reader: xtermReader{}}
}

// Encrypt prompts for a new password and its confirmation.
func (p *Provider) Encrypt() ([]byte, error) {
	return p.withTerminal(func(session terminalSession) ([]byte, error) {
		return p.confirm(session, "Envseal password: ", "Confirm Envseal password: ")
	})
}

// Decrypt prompts once for an existing password. It is also used by dry-run.
func (p *Provider) Decrypt() ([]byte, error) {
	return p.withTerminal(func(session terminalSession) ([]byte, error) {
		return p.read(session, "Envseal password: ")
	})
}

// Rotate prompts for the current password and a confirmed replacement.
func (p *Provider) Rotate() (current, replacement []byte, err error) {
	session, err := p.openValidated()
	if err != nil {
		return nil, nil, err
	}
	defer session.Close()

	current, err = p.read(session, "Current Envseal password: ")
	if err != nil {
		return nil, nil, err
	}
	replacement, err = p.confirm(session, "New Envseal password: ", "Confirm new Envseal password: ")
	if err != nil {
		Wipe(current)
		return nil, nil, err
	}
	return current, replacement, nil
}

// Wipe clears a password byte slice after its final use.
func Wipe(password []byte) {
	for i := range password {
		password[i] = 0
	}
}

func (p *Provider) withTerminal(prompt func(terminalSession) ([]byte, error)) ([]byte, error) {
	session, err := p.openValidated()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return prompt(session)
}

func (p *Provider) openValidated() (terminalSession, error) {
	if p == nil || p.open == nil || p.reader == nil {
		return nil, ErrTerminalUnavailable
	}
	session, err := p.open()
	if err != nil || session == nil {
		return nil, ErrTerminalUnavailable
	}
	if !p.reader.IsTerminal(session.InputFD()) || !p.reader.IsTerminal(session.OutputFD()) {
		_ = session.Close()
		return nil, ErrTerminalUnavailable
	}
	return session, nil
}

func (p *Provider) confirm(session terminalSession, firstPrompt, confirmationPrompt string) ([]byte, error) {
	password, err := p.read(session, firstPrompt)
	if err != nil {
		return nil, err
	}
	confirmation, err := p.read(session, confirmationPrompt)
	if err != nil {
		Wipe(password)
		return nil, err
	}
	matches := subtle.ConstantTimeCompare(password, confirmation) == 1
	Wipe(confirmation)
	if !matches {
		Wipe(password)
		return nil, ErrPasswordMismatch
	}
	return password, nil
}

func (p *Provider) read(session terminalSession, prompt string) ([]byte, error) {
	if _, err := io.WriteString(session, prompt); err != nil {
		return nil, ErrTerminalUnavailable
	}
	password, readErr := p.reader.ReadPassword(session.InputFD())
	// ReadPassword does not print the newline produced by Enter. Always make
	// the next prompt/error begin on a fresh controlling-terminal line.
	if _, err := io.WriteString(session, "\n"); err != nil && readErr == nil {
		Wipe(password)
		return nil, ErrTerminalUnavailable
	}
	if readErr != nil {
		Wipe(password)
		if errors.Is(readErr, io.EOF) || errors.Is(readErr, syscall.EINTR) {
			return nil, ErrPasswordCancelled
		}
		return nil, ErrPasswordRead
	}
	if len(password) == 0 {
		return nil, ErrBlankPassword
	}
	return password, nil
}
