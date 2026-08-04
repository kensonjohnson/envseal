default: check

# Format all tracked Go source files.
fmt:
    gofmt -w $(git ls-files '*.go')

# Fail when tracked Go source is not gofmt-formatted.
fmt-check:
    test -z "$(gofmt -l $(git ls-files '*.go'))"

# Run the full unit and integration test suite with the supported Go baseline.
test:
    GOTOOLCHAIN=go1.24.0 go test ./...

# Run race-enabled tests on platforms supported by the Go race detector.
race:
    GOTOOLCHAIN=go1.24.0 go test -race ./...

# Run static analysis.
vet:
    GOTOOLCHAIN=go1.24.0 go vet ./...

# Ensure the pinned module graph is tidy and verifiable without modifying it.
verify:
    GOTOOLCHAIN=go1.24.0 go mod tidy -diff
    GOTOOLCHAIN=go1.24.0 go mod verify

# Build a local development binary.
build:
    mkdir -p build
    GOTOOLCHAIN=go1.24.0 go build -o build/envseal .

# Run the repository's local release-validation suite.
check: fmt-check vet test verify build

# Build six versioned release archives and SHA256SUMS into dist/.
release-build VERSION:
    ./scripts/release-build.sh {{VERSION}}
