// Package cli parses Envseal's command-line interface and renders its stable
// help, version, and usage-error boundary.
package cli

import (
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/kensonjohnson/envseal/internal/generator"
)

// Command identifies an Envseal operation.
type Command string

const (
	CommandHelp     Command = "help"
	CommandVersion  Command = "version"
	CommandEncrypt  Command = "encrypt"
	CommandDecrypt  Command = "decrypt"
	CommandRotate   Command = "rotate"
	CommandCheck    Command = "check"
	CommandGenerate Command = "generate"
)

// Request is a fully parsed command invocation. Command handlers consume this
// type instead of inspecting process arguments directly.
type Request struct {
	Command Command
	HelpFor Command
	Source  string
	Output  string
	Keys    []string
	Quiet   bool
	Force   bool
	DryRun  bool
	Mode    string
	Words   int
	Bytes   int
}

// UsageError is an invocation-shape error and maps to exit code 2.
type UsageError struct {
	message string
}

func (e *UsageError) Error() string { return e.message }

func usageError(message string) error {
	return &UsageError{message: message}
}

// Parse decodes an Envseal invocation without reading files, terminals, or
// environment variables. It deliberately does not echo user arguments in
// errors, because a path or key may contain sensitive context.
func Parse(args []string) (Request, error) {
	if len(args) == 0 {
		return Request{}, usageError("missing command")
	}

	switch args[0] {
	case "--help":
		if len(args) != 1 {
			return Request{}, usageError("--help accepts no arguments")
		}
		return Request{Command: CommandHelp}, nil
	case "--version":
		if len(args) != 1 {
			return Request{}, usageError("--version accepts no arguments")
		}
		return Request{Command: CommandVersion}, nil
	case string(CommandEncrypt):
		return parseOperation(CommandEncrypt, args[1:])
	case string(CommandDecrypt):
		return parseOperation(CommandDecrypt, args[1:])
	case string(CommandRotate):
		return parseOperation(CommandRotate, args[1:])
	case string(CommandCheck):
		return parseOperation(CommandCheck, args[1:])
	case string(CommandGenerate):
		return parseGenerate(args[1:])
	default:
		return Request{}, usageError("unknown command")
	}
}

func parseOperation(command Command, args []string) (Request, error) {
	req := Request{Command: command}
	var positional []string
	options := true
	help := false

	for _, arg := range args {
		if options {
			switch arg {
			case "--":
				options = false
				continue
			case "--help":
				help = true
				continue
			case "--quiet":
				if req.Quiet {
					return Request{}, usageError("--quiet may be specified once")
				}
				req.Quiet = true
				continue
			case "--force":
				if command != CommandDecrypt {
					return Request{}, usageError("--force is only valid with decrypt")
				}
				if req.Force {
					return Request{}, usageError("--force may be specified once")
				}
				req.Force = true
				continue
			case "--dry-run":
				if command != CommandDecrypt {
					return Request{}, usageError("--dry-run is only valid with decrypt")
				}
				if req.DryRun {
					return Request{}, usageError("--dry-run may be specified once")
				}
				req.DryRun = true
				continue
			default:
				if strings.HasPrefix(arg, "-") {
					return Request{}, usageError("unknown option")
				}
			}
		}
		positional = append(positional, arg)
	}

	if help {
		return Request{Command: CommandHelp, HelpFor: command}, nil
	}

	switch command {
	case CommandEncrypt:
		if len(positional) < 2 {
			return Request{}, usageError("encrypt requires a source and at least one key")
		}
		req.Source = positional[0]
		req.Keys = positional[1:]
	case CommandDecrypt:
		if req.DryRun {
			if req.Force {
				return Request{}, usageError("--force and --dry-run cannot be combined")
			}
			if len(positional) != 1 {
				return Request{}, usageError("decrypt --dry-run requires exactly one source")
			}
			req.Source = positional[0]
		} else {
			if len(positional) != 2 {
				return Request{}, usageError("decrypt requires a source and plaintext output")
			}
			req.Source = positional[0]
			req.Output = positional[1]
		}
	case CommandRotate, CommandCheck:
		if len(positional) != 1 {
			return Request{}, usageError(string(command) + " requires exactly one source")
		}
		req.Source = positional[0]
	default:
		return Request{}, errors.New("internal command parser error")
	}

	if err := validateRequest(req); err != nil {
		return Request{}, err
	}
	return req, nil
}

func parseGenerate(args []string) (Request, error) {
	req := Request{Command: CommandGenerate}
	var positional []string
	help := false
	options := true
	wordsSet := false
	bytesSet := false

	for index := 0; index < len(args); index++ {
		arg := args[index]
		if options {
			switch arg {
			case "--":
				options = false
				continue
			case "--help":
				help = true
				continue
			case "--words", "--bytes":
				if index+1 == len(args) {
					return Request{}, usageError(arg + " requires a decimal count")
				}
				index++
				count, err := parseDecimalCount(args[index])
				if err != nil {
					return Request{}, err
				}
				if arg == "--words" {
					if wordsSet {
						return Request{}, usageError("--words may be specified once")
					}
					wordsSet = true
					req.Words = count
				} else {
					if bytesSet {
						return Request{}, usageError("--bytes may be specified once")
					}
					bytesSet = true
					req.Bytes = count
				}
				continue
			default:
				if strings.HasPrefix(arg, "-") {
					return Request{}, usageError("unknown option")
				}
			}
		}
		positional = append(positional, arg)
	}

	if help {
		return Request{Command: CommandHelp, HelpFor: CommandGenerate}, nil
	}
	if len(positional) != 1 {
		return Request{}, usageError("generate requires exactly one mode")
	}
	req.Mode = positional[0]
	switch req.Mode {
	case "passphrase":
		if bytesSet {
			return Request{}, usageError("--bytes is only valid with generate secret")
		}
		if !wordsSet {
			req.Words = 8
		}
		if req.Words < generator.MinPassphraseWords || req.Words > generator.MaxPassphraseWords {
			return Request{}, usageError("--words must be between 6 and 64")
		}
	case "secret":
		if wordsSet {
			return Request{}, usageError("--words is only valid with generate passphrase")
		}
		if !bytesSet {
			req.Bytes = 32
		}
		if req.Bytes < generator.MinSecretBytes || req.Bytes > generator.MaxSecretBytes {
			return Request{}, usageError("--bytes must be between 16 and 4096")
		}
	default:
		return Request{}, usageError("generate mode must be passphrase or secret")
	}
	return req, nil
}

func parseDecimalCount(value string) (int, error) {
	if value == "" {
		return 0, usageError("count must be a decimal integer")
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, usageError("count must be a decimal integer")
		}
	}
	count, err := strconv.Atoi(value)
	if err != nil {
		return 0, usageError("count must be a decimal integer")
	}
	return count, nil
}

func validateRequest(req Request) error {
	if req.Source == "" {
		return usageError("source must not be empty")
	}
	if req.Command == CommandDecrypt && !req.DryRun && req.Output == "" {
		return usageError("plaintext output must not be empty")
	}
	for _, key := range req.Keys {
		if key == "" {
			return usageError("key must not be empty")
		}
	}
	return nil
}

// Run applies the standard output and exit-code boundary around Parse and the
// production command executor.
func Run(args []string, version string, stdout, stderr io.Writer) int {
	return run(args, version, stdout, stderr, newService())
}

// Usage returns the help text for the root command or an individual operation.
func Usage(command Command) string {
	switch command {
	case CommandEncrypt:
		return "Usage: envseal encrypt [--quiet] <source> <key> [<key> ...]\n\nEncrypt selected dotenv values in place.\n\nOptions:\n  --quiet  Suppress the success summary.\n  --help   Show this help.\n  --       Stop option parsing.\n"
	case CommandDecrypt:
		return "Usage: envseal decrypt [--quiet] [--force] <source> <plaintext-output>\n       envseal decrypt [--quiet] --dry-run <source>\n\nDecrypt sealed dotenv values to an explicit plaintext output, or authenticate them without writing output.\n\nOptions:\n  --quiet    Suppress the success summary.\n  --force    Replace an existing plaintext output.\n  --dry-run  Authenticate all envelopes without writing output.\n  --help     Show this help.\n  --         Stop option parsing.\n"
	case CommandRotate:
		return "Usage: envseal rotate [--quiet] <source>\n\nRe-encrypt all sealed values in place with a new password.\n\nOptions:\n  --quiet  Suppress the success summary.\n  --help   Show this help.\n  --       Stop option parsing.\n"
	case CommandCheck:
		return "Usage: envseal check [--quiet] <source>\n\nValidate dotenv syntax and sealed-envelope structure without prompting or writing.\n\nOptions:\n  --quiet  Suppress the success summary.\n  --help   Show this help.\n  --       Stop option parsing.\n"
	case CommandGenerate:
		return "Usage: envseal generate passphrase [--words <count>]\n       envseal generate secret [--bytes <count>]\n\nGenerate a credential and write it to stdout.\n\nOptions:\n  --words  Passphrase word count (6–64; default 8).\n  --bytes  Secret byte count (16–4096; default 32).\n  --help   Show this help.\n  --       Stop option parsing.\n"
	default:
		return "Usage:\n  envseal encrypt [--quiet] <source> <key> [<key> ...]\n  envseal decrypt [--quiet] [--force] <source> <plaintext-output>\n  envseal decrypt [--quiet] --dry-run <source>\n  envseal rotate [--quiet] <source>\n  envseal check [--quiet] <source>\n  envseal generate passphrase [--words <count>]\n  envseal generate secret [--bytes <count>]\n\nCommands:\n  encrypt   Encrypt selected dotenv values in place.\n  decrypt   Write plaintext values to an explicit output.\n  rotate    Re-encrypt all sealed values with a new password.\n  check     Validate source syntax and envelope structure.\n  generate  Generate a passphrase or machine secret.\n\nGlobal options:\n  --help     Show root help, or command help after a command.\n  --version  Print the build version.\n  --          Stop option parsing for paths or keys beginning with '-'.\n"
	}
}
