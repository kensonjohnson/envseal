package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/kensonjohnson/envseal/internal/dotenv"
	"github.com/kensonjohnson/envseal/internal/envelope"
	"github.com/kensonjohnson/envseal/internal/filewrite"
	"github.com/kensonjohnson/envseal/internal/password"
)

type executor interface {
	Execute(Request) (string, error)
}

type passwordPrompter interface {
	Encrypt() ([]byte, error)
	Decrypt() ([]byte, error)
	Rotate() (current, replacement []byte, err error)
}

type sourceWriter interface {
	ValidateSource(string) error
	ValidatePlaintextTarget(source, target string, force bool) error
	WriteEncrypted(string, []byte) error
	WritePlaintext(source, target string, data []byte, force bool) error
}

type service struct {
	readFile  func(string) ([]byte, error)
	passwords passwordPrompter
	writer    sourceWriter
}

func newService() *service {
	return &service{
		readFile:  os.ReadFile,
		passwords: password.New(),
		writer:    filewrite.New(),
	}
}

func run(args []string, version string, stdout, stderr io.Writer, execute executor) int {
	req, err := Parse(args)
	if err != nil {
		var usage *UsageError
		if errors.As(err, &usage) {
			fmt.Fprintf(stderr, "envseal: %s\nRun 'envseal --help' for usage.\n", usage.Error())
			return 2
		}
		fmt.Fprintln(stderr, "envseal: command parsing failed")
		return 1
	}

	switch req.Command {
	case CommandHelp:
		fmt.Fprint(stdout, Usage(req.HelpFor))
		return 0
	case CommandVersion:
		fmt.Fprintf(stdout, "envseal %s\n", version)
		return 0
	default:
		if execute == nil {
			fmt.Fprintln(stderr, "envseal: command execution failed")
			return 1
		}
		summary, err := execute.Execute(req)
		if err != nil {
			writeExecutionError(stderr, req, err)
			return 1
		}
		if !req.Quiet {
			fmt.Fprintf(stdout, "envseal: %s\n", summary)
		}
		return 0
	}
}

func (s *service) Execute(req Request) (string, error) {
	switch req.Command {
	case CommandEncrypt:
		return s.encrypt(req)
	case CommandDecrypt:
		return s.decrypt(req)
	case CommandRotate:
		return s.rotate(req)
	case CommandCheck:
		return s.check(req)
	default:
		return "", &operationError{path: req.Source, category: "command-failed"}
	}
}

func (s *service) encrypt(req Request) (string, error) {
	document, err := s.load(req.Source)
	if err != nil {
		return "", err
	}
	preview, err := document.Encrypt(req.Keys, func(_ []byte, value []byte) ([]byte, error) {
		return append([]byte(nil), value...), nil
	})
	if err != nil {
		return "", err
	}
	password.Wipe(preview.Data)
	if preview.Changed == 0 {
		return fmt.Sprintf("encrypted 0 keys in %s", req.Source), nil
	}

	secret, err := s.passwords.Encrypt()
	if err != nil {
		return "", passwordError(req.Source)
	}
	defer password.Wipe(secret)
	result, err := document.Encrypt(req.Keys, func(key, value []byte) ([]byte, error) {
		sealed, err := envelope.Seal(secret, key, value)
		if err != nil {
			return nil, err
		}
		return []byte(sealed), nil
	})
	if err != nil {
		return "", err
	}
	if err := s.writer.WriteEncrypted(req.Source, result.Data); err != nil {
		return "", writeError(req.Source, err)
	}
	return fmt.Sprintf("encrypted %d keys in %s", result.Changed, req.Source), nil
}

func (s *service) decrypt(req Request) (string, error) {
	if !req.DryRun {
		if err := s.writer.ValidatePlaintextTarget(req.Source, req.Output, req.Force); err != nil {
			return "", writeError(req.Output, err)
		}
	}
	document, err := s.load(req.Source)
	if err != nil {
		return "", err
	}
	secret, err := s.passwords.Decrypt()
	if err != nil {
		return "", passwordError(req.Source)
	}
	defer password.Wipe(secret)
	result, err := document.Decrypt(func(key, value []byte) ([]byte, error) {
		return envelope.Open(secret, key, string(value))
	})
	if err != nil {
		return "", err
	}
	defer password.Wipe(result.Data)
	if req.DryRun {
		return fmt.Sprintf("verified %d envelopes in %s", result.Changed, req.Source), nil
	}
	if err := s.writer.WritePlaintext(req.Source, req.Output, result.Data, req.Force); err != nil {
		return "", writeError(req.Output, err)
	}
	return fmt.Sprintf("decrypted %d envelopes from %s", result.Changed, req.Source), nil
}

func (s *service) rotate(req Request) (string, error) {
	document, err := s.load(req.Source)
	if err != nil {
		return "", err
	}
	current, replacement, err := s.passwords.Rotate()
	if err != nil {
		return "", passwordError(req.Source)
	}
	defer password.Wipe(current)
	defer password.Wipe(replacement)
	result, err := document.Decrypt(func(key, value []byte) ([]byte, error) {
		plaintext, err := envelope.Open(current, key, string(value))
		if err != nil {
			return nil, err
		}
		defer password.Wipe(plaintext)
		sealed, err := envelope.Seal(replacement, key, plaintext)
		if err != nil {
			return nil, err
		}
		return []byte(sealed), nil
	})
	if err != nil {
		return "", err
	}
	if err := s.writer.WriteEncrypted(req.Source, result.Data); err != nil {
		return "", writeError(req.Source, err)
	}
	return fmt.Sprintf("rotated %d envelopes in %s", result.Changed, req.Source), nil
}

func (s *service) check(req Request) (string, error) {
	document, err := s.load(req.Source)
	if err != nil {
		return "", err
	}
	if err := document.Check(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s is valid", req.Source), nil
}

func (s *service) load(path string) (*dotenv.Document, error) {
	if s == nil || s.readFile == nil || s.writer == nil {
		return nil, &operationError{path: path, category: "source-read-failed"}
	}
	if err := s.writer.ValidateSource(path); err != nil {
		return nil, writeError(path, err)
	}
	source, err := s.readFile(path)
	if err != nil {
		return nil, &operationError{path: path, category: "source-read-failed"}
	}
	defer password.Wipe(source)
	return dotenv.Parse(source)
}

type operationError struct {
	path     string
	category string
}

func (e *operationError) Error() string { return "envseal operation failed" }

func passwordError(path string) error {
	return &operationError{path: path, category: "password-input-failed"}
}

func writeError(path string, err error) error {
	category := "file-write-failed"
	switch {
	case errors.Is(err, filewrite.ErrInvalidSource):
		category = "invalid-source-file"
	case errors.Is(err, filewrite.ErrInvalidTarget):
		category = "invalid-target-file"
	case errors.Is(err, filewrite.ErrSamePath):
		category = "source-target-collision"
	case errors.Is(err, filewrite.ErrTargetExists):
		category = "target-exists"
	case errors.Is(err, filewrite.ErrDurability):
		category = "replacement-not-durable"
	}
	return &operationError{path: path, category: category}
}

func writeExecutionError(stderr io.Writer, req Request, err error) {
	var dotenvError *dotenv.Error
	if errors.As(err, &dotenvError) {
		for _, issue := range dotenvError.Issues {
			if issue.Line > 0 {
				fmt.Fprintf(stderr, "envseal: %s:%d: %s\n", req.Source, issue.Line, issue.Category)
			} else {
				fmt.Fprintf(stderr, "envseal: %s: %s\n", req.Source, issue.Category)
			}
		}
		return
	}
	var operation *operationError
	if errors.As(err, &operation) {
		fmt.Fprintf(stderr, "envseal: %s: %s\n", operation.path, operation.category)
		return
	}
	fmt.Fprintf(stderr, "envseal: %s: operation-failed\n", req.Source)
}
