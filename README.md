# envseal

`envseal` encrypts selected simple dotenv values in a committed file and decrypts them to an explicit local output. It is a small Go command for repositories that need to share one password-manager-held secret without putting plaintext values in Git.

> **Scope:** Envseal encrypts dotenv value bodies; it is not a secrets manager, access-control system, or a substitute for production secret storage.

## Requirements and support

- Go **1.24+** for the primary `go tool` workflow.
- macOS, Linux, or Windows with an interactive controlling terminal.
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

Release tags use Go-compatible CalVer: `v1.YYYYMMDD.N`, where `N` increments for multiple releases on a day. Source builds report `envseal devel`.

GitHub Releases also provide optional archives for macOS, Linux, and Windows on amd64 and arm64, with `SHA256SUMS` and GitHub build-provenance attestations. `go tool` is the primary distribution path; v1 does not publish package-manager packages.

## Quick start

Generate one high-entropy shared password in your password manager and distribute it outside Git. Every collaborator who needs to decrypt or rotate the file needs that password.

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
```

- `encrypt` seals only named plaintext keys in place and skips selected values already sealed.
- `decrypt` authenticates every envelope and writes all restored values to a distinct output path.
- `rotate` authenticates and re-encrypts every envelope in place with a confirmed replacement password.
- `check` validates the entire file's grammar, duplicate keys, and envelope structure without prompting or writing.
- `--quiet` suppresses the one success summary, never diagnostics.
- `--force` is decrypt-only and permits replacing the explicit plaintext output.
- `--dry-run` is decrypt-only; it prompts and authenticates but writes nothing.
- `--` stops option parsing for a path or key beginning with `-`.

Each successful command prints one summary to stdout. Operational, validation, and authentication failures return exit code 1; malformed invocations return exit code 2. Diagnostics go to stderr and identify a path, optional line number, and category—not dotenv values, ciphertext, or password material.

## Dotenv subset

Envseal deliberately handles a narrow, lossless subset:

- Blank lines, full-line `#` comments, and LF/CRLF endings are preserved.
- Transformable assignments are one line of `KEY=VALUE`.
- `KEY` must match `^[A-Za-z_][A-Za-z0-9_]*$`; key matching is case-sensitive.
- `VALUE` is unquoted raw text. Leading/trailing spaces or tabs, inline comments, quotes, backticks, multiline values, `export`, whitespace around the key or `=`, interpolation, command substitution, and trailing backslashes are unsupported.
- Duplicate supported keys anywhere in a file are rejected by every command.

For `encrypt`, an unsupported line fails only when it is selected; unselected unsupported text remains unchanged. `check` reports every unsupported line. Do not use Envseal on dotenv files that rely on advanced dotenv syntax.

## Passwords, encryption, and compatibility

Use a password-manager-generated 256-bit secret. Do not place it in Git, shell history, CI, environment variables, or a dotenv file. Envseal asks for confirmation when encrypting or rotating, asks once to decrypt, and reads no password for `check`.

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
