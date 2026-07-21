# Zitadel Operator development commands

# Disable go.work (parent workspace interferes with controller-gen)
export GOWORK := "off"

# crd-ref-docs version for the generated API reference (docs/reference/api.md)
crd_ref_docs_version := "v0.3.0"

# Generate deepcopy methods, CRD manifests (synced to the Helm chart), and the CRD API reference
generate:
    controller-gen object paths="./api/..."
    controller-gen crd paths="./api/..." output:crd:artifacts:config=config/crd/bases
    cp config/crd/bases/*.yaml charts/zitadel-operator-crds/templates/
    go run github.com/elastic/crd-ref-docs@{{crd_ref_docs_version}} \
        --source-path=./api/v1alpha2 \
        --config=docs/reference/crd-ref-docs.yaml \
        --renderer=markdown \
        --output-path=docs/reference/api.md

# Format all Go files (gofmt + goimports via golangci-lint)
fmt:
    golangci-lint fmt ./...

# Build the operator binary (depends on generate + fmt)
build: generate fmt
    go build -o bin/zitadel-operator ./cmd/operator/

# Run tests
test:
    go test ./... -coverprofile=coverage.out

# Run integration tests (requires real Zitadel + devbox shell)
test-integration:
    go test -tags=integration -v ./tests/integration/... -count=1 -timeout=30m

# Run linters
lint:
    golangci-lint run ./...

# Run Go vulnerability check
vuln:
    govulncheck ./...

# Verify generated files are committed (fails if generate produces uncommitted changes)
verify-generate: generate
    @echo "Checking for uncommitted generated files..."
    @git diff --exit-code -- config/crd/ charts/zitadel-operator-crds/templates/ api/v1alpha2/zz_generated.deepcopy.go docs/reference/api.md || (echo "ERROR: Generated files are out of date. Run 'just generate' and commit." && exit 1)
    @echo "✅ Generated files are up to date."

# Run go mod tidy
tidy:
    go mod tidy

# Clean build artifacts
clean:
    rm -rf bin/ dist/ coverage.out

# Run all checks (build + test + lint + vuln + verify generated files)
check: build test lint vuln verify-generate

# Build a snapshot release locally (no push, no tag)
snapshot:
    goreleaser release --snapshot --clean

# Package Helm charts locally (for testing)
helm-package:
    helm package charts/zitadel-operator --destination dist/
    helm package charts/zitadel-operator-crds --destination dist/
