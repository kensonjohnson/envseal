# envseal

`envseal` generates high-entropy credentials and encrypts selected simple dotenv values in a committed file, decrypting them to an explicit local output. It is a small Go command for repositories that need to share one password-manager-held secret without putting plaintext values in Git.

> **Scope:** Envseal encrypts dotenv value bodies; it is not a secrets manager, access-control system, or a substitute for production secret storage.

## Requirements and support

- Go **1.24+** for the `go tool` and global `go install` workflows.
- macOS, Linux, or Windows; `encrypt`, `decrypt`, and `rotate` require an interactive controlling terminal for password prompts.
- `golang.org/x/term` is Envseal's only non-standard runtime dependency; cryptography uses Go's standard library.

Passwords are read hidden from the controlling terminal only. Envseal never accepts a password in an argument, environment variable, stdin, or a file.

## Install in a repository

From the root of the repository that will use Envseal, pin a release tag:

```sh
go get -tool github.com/kensonjohnson/envseal@v1.20260803.1
go tool envseal --version
```

`go get -tool` records the tool and its exact version in that repository's `go.mod` and `go.sum`; commit both files. Invoke it from within that Go module:

```sh
go tool envseal check .env.example
```

To update, choose a reviewed release tag rather than a floating version, then review and commit the resulting module-file changes:

```sh
go get -tool github.com/kensonjohnson/envseal@v1.YYYYMMDD.N
go tool envseal --version
```

To roll back, install a previously reviewed immutable tag and commit the resulting `go.mod` and `go.sum` diff:

```sh
go get -tool github.com/kensonjohnson/envseal@v1.20260803.1
go tool envseal --version
```

Envseal is a public module, so the normal Go module proxy and checksum database apply; it needs no `GOPRIVATE` configuration. Release tags use Go-compatible CalVer: `v1.YYYYMMDD.N`, where `N` increments for multiple releases on a day. Source builds report `envseal devel`.

## Install globally

The repository-pinned `go tool` workflow above is preferred. To install a direct global binary instead, pin a reviewed release tag:

```sh
go install github.com/kensonjohnson/envseal@v1.20260814.1
```

An exact release tag is immutable and reproducible. `go install github.com/kensonjohnson/envseal@v1` instead resolves the latest v1 release at install time, so it floats.

This installs `envseal` to `GOBIN`, or `GOPATH/bin` when `GOBIN` is unset. Ensure that directory is on `PATH`, then invoke the binary directly:

```sh
envseal --version
envseal check .env.example
```

GitHub Releases also provide optional archives for macOS, Linux, and Windows on amd64 and arm64, with `SHA256SUMS` and GitHub build-provenance attestations. `go tool` is the primary distribution path; v1 does not publish package-manager packages.

After a release, maintainers should smoke-test the published module from a clean external module:

```sh
mkdir envseal-smoke && cd envseal-smoke
go mod init example.invalid/envseal-smoke
go get -tool github.com/kensonjohnson/envseal@v1.YYYYMMDD.N
go tool envseal --version
```

Verify an optional downloaded archive before use:

```sh
shasum -a 256 -c SHA256SUMS
gh attestation verify envseal_v1.YYYYMMDD.N_linux_amd64.tar.gz --repo kensonjohnson/envseal
```

## Shell completion

Envseal renders static completion for Bash, Zsh, Fish, and PowerShell. The rendered scripts complete `envseal` directly when that binary is on `PATH`; otherwise they define an `envseal` wrapper that runs the repository-pinned `go tool envseal`. They do not alter `go` completion.

The commands below use the repository-pinned workflow. For a global `go install` binary (or a release archive binary), replace every `go tool envseal` with `envseal`.

Render a script without changing the filesystem:

```sh
go tool envseal completion bash
go tool envseal completion zsh
go tool envseal completion fish
go tool envseal completion powershell
```

Install only writes to a shell-verified autoload location. It never changes a startup or profile file by default:

```sh
go tool envseal completion install bash
go tool envseal completion install zsh
go tool envseal completion install fish
go tool envseal completion install powershell
```

- **Bash:** Install [bash-completion](https://github.com/scop/bash-completion) and ensure it is loaded by your existing Bash setup. Envseal then writes `envseal.bash` to its configured `BASH_COMPLETION_USER_DIR`, or its default user completion directory. Start a new shell or reload your existing bash-completion setup. If no framework is detected, Envseal writes nothing; activate the current shell with `source <(go tool envseal completion bash)`.
- **Zsh:** Enable `autoload -Uz compinit && compinit` in your existing Zsh setup and ensure an existing writable entry is present in `$fpath`. Envseal writes `_envseal` there; restart Zsh or run `compinit` again. If no such entry is found, no files are written; activate the current shell with `source <(go tool envseal completion zsh)`.
- **Fish:** Envseal writes `envseal.fish` to `$XDG_CONFIG_HOME/fish/completions` (normally `~/.config/fish/completions`). Fish loads it automatically in a new shell; run `source ~/.config/fish/completions/envseal.fish` to load the default-location file now.
- **PowerShell:** PowerShell has no arbitrary completion autoload directory, so its default install intentionally writes nothing. Activate the current session with `go tool envseal completion powershell | Invoke-Expression`.

When the safe default reports no verified autoload location, persist completion only by explicitly consenting to a small idempotent startup/profile block:

```sh
go tool envseal completion install bash --configure-shell       # ~/.bashrc
go tool envseal completion install zsh --configure-shell        # ~/.zshrc
go tool envseal completion install powershell --configure-shell # current-user PowerShell profile
```

Before writing, `--configure-shell` prints the exact rendered-script target and profile block it will add. It then writes the static rendered script to `$XDG_CONFIG_HOME/envseal/completions` (normally `~/.config/envseal/completions`) and adds that marked, idempotent profile block sourcing the absolute script path. The block preserves existing content, is not duplicated on later runs, and contains no `go tool` command, so configured completion works equally with a direct release binary and the repository-pinned workflow. Restart the relevant shell after configuring it.

## Development and releases

[Just](https://github.com/casey/just) is the repository command runner. Run `just check` before a change; it performs formatting, vet, tests, module verification, and a development build. `just release-build v1.YYYYMMDD.N` produces the six local release archives and `SHA256SUMS` in `dist/`.

CI runs validation on pull requests; it never creates a release. A maintainer starts the **Release** GitHub Actions workflow with `workflow_dispatch` from the protected `main` branch. The workflow validates the commit, computes the next UTC `v1.YYYYMMDD.N` tag, builds and attests all six archives, and creates one GitHub Release. The release binary reports that exact tag.

## Quick start

Generate a high-entropy shared credential, store it in a password manager, and distribute it outside Git. Every collaborator who needs to decrypt or rotate the file needs that credential:

```sh
go tool envseal generate passphrase
go tool envseal generate secret
```

`generate passphrase` prints an eight-word hyphenated passphrase by default. `generate secret` prints a standard padded Base64 encoding of 32 random bytes by default. Each command writes exactly one credential plus a newline to stdout; normal shell redirection, pipes, terminal scrollback, and logs can persist or disclose it.

Create a committed template with placeholders only:

```dotenv
# .env.example (committed)
API_URL=https://api.example.invalid
API_TOKEN=replace-me
```

Ignore the local plaintext output:

```gitignore
.env
.env.*
!.env.example
!.env.sample
```

Encrypt only the intended value, review the diff, then commit it:

```sh
go tool envseal encrypt .env.example API_TOKEN
go tool envseal check .env.example
git add .env.example go.mod go.sum
```

The selected body becomes an `ENVSEAL[...]` value. Comments, line endings, keys, and unselected value bodies are preserved byte-for-byte.

A collaborator validates the committed file and writes plaintext only to their explicit ignored output:

```sh
go tool envseal check .env.example
go tool envseal decrypt .env.example .env
```

Use `decrypt --dry-run .env.example` to authenticate all sealed values without creating an output. Use `decrypt --force .env.example .env` only when deliberately replacing an existing local plaintext output.

## Commands

```text
envseal encrypt [--quiet] <source> <key> [<key> ...]
envseal decrypt [--quiet] [--force] <source> <plaintext-output>
envseal decrypt [--quiet] --dry-run <source>
envseal rotate  [--quiet] <source>
envseal check   [--quiet] <source>
envseal generate passphrase [--words <count>]
envseal generate secret [--bytes <count>]
```

- `encrypt` seals only named plaintext keys in place and skips selected values already sealed.
- `decrypt` authenticates every envelope and writes all restored values to a distinct output path.
- `rotate` authenticates and re-encrypts every envelope in place with a confirmed replacement password.
- `check` validates the entire file's grammar, duplicate keys, and envelope structure without prompting or writing.
- `generate passphrase` creates an EFF Large Wordlist passphrase; `--words` accepts 6–64 words and defaults to 8.
- `generate secret` creates a standard padded Base64 machine secret; `--bytes` accepts 16–4,096 random bytes and defaults to 32.
- `--quiet` suppresses the one success summary, never diagnostics; generation has no summary and does not accept `--quiet`.
- `--force` is decrypt-only and permits replacing the explicit plaintext output.
- `--dry-run` is decrypt-only; it prompts and authenticates but writes nothing.
- `--` stops option parsing for a path or key beginning with `-`.

Each successful dotenv command prints one summary to stdout. Each successful generation command writes only the credential and trailing newline to stdout. Operational, validation, authentication, and generation failures return exit code 1; malformed invocations return exit code 2. Diagnostics go to stderr and identify a path, optional line number, and category—not dotenv values, ciphertext, password material, or generated credentials.

## Dotenv subset

Envseal deliberately handles a narrow, lossless subset:

- Blank lines, full-line `#` comments, and LF/CRLF endings are preserved.
- Transformable assignments are one line of `KEY=VALUE`.
- `KEY` must match `^[A-Za-z_][A-Za-z0-9_]*$`; key matching is case-sensitive.
- `VALUE` is unquoted raw text. Leading/trailing spaces or tabs, inline comments, quotes, backticks, multiline values, `export`, whitespace around the key or `=`, interpolation, command substitution, and trailing backslashes are unsupported.
- Duplicate supported keys anywhere in a file are rejected by every command.

For `encrypt`, an unsupported line fails only when it is selected; unselected unsupported text remains unchanged. `check` reports every unsupported line. Do not use Envseal on dotenv files that rely on advanced dotenv syntax.

## Passwords, encryption, and compatibility

Use `generate secret` for a password-manager-held 256-bit secret, or `generate passphrase` when a human-typable shared credential is preferred. Do not place generated credentials in Git, shell history, CI, environment variables, or a dotenv file. Envseal asks for confirmation when encrypting or rotating, asks once to decrypt, and reads no password for `check` or `generate`.

Each v1 envelope uses PBKDF2-HMAC-SHA-256 with 1,000,000 iterations, a fresh 16-byte salt, and AES-256-GCM. The exact dotenv key is authenticated associated data, so moving an envelope to another key fails authentication. Plaintext values are limited to 1 MiB.

The envelope's `v1` format marker is a compatibility boundary, not the Go module major. Envseal will continue to decrypt every released envelope format. A future version may add a newer envelope format; `rotate` is the explicit migration path and decrypt never silently migrates a value.

## Filesystem behavior and recovery

Envseal rejects source and existing target symlinks, non-regular files, and decrypt source/output collisions. It completes parsing, transformation, and authentication in memory before creating a temporary file. Writes use a temporary file in the target directory, sync and close it, then replace the target.

On Unix, encrypt and rotate preserve ordinary source permission bits; decrypt output is created or replaced with mode `0600`. Same-directory replacement is atomic on Unix, followed by a best-effort directory sync. On Windows, the same workflow is used, but directory ACLs—not Unix mode bits—control plaintext confidentiality, and replacement is not guaranteed atomic.

A malformed envelope, wrong password, missing selected key, or failed authentication leaves the committed source and any existing plaintext target unchanged. If Envseal reports `replacement-not-durable`, the rename already succeeded but directory durability could not be confirmed; it does not roll back. Treat the file as potentially updated, validate it, then use `decrypt --dry-run` with the appropriate password before committing or retrying. Restore from version control or a verified backup if validation fails.

## Security reports

See [SECURITY.md](SECURITY.md). Please use private GitHub vulnerability reporting and never include real passwords or dotenv values in a report.

## License

Envseal is licensed under [0BSD](LICENSE).
