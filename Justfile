# Zitadel Operator development commands

# Generate deepcopy methods, CRD manifests, and sync to Helm chart
generate:
    controller-gen object paths="./api/..."
    controller-gen crd paths="./api/..." output:crd:artifacts:config=config/crd/bases
    cp config/crd/bases/*.yaml charts/zitadel-operator-crds/templates/

# Build the operator binary (depends on generate to ensure CRDs are up to date)
build: generate
    go build -o bin/zitadel-operator ./cmd/operator/

# Run tests
test:
    go test ./... -coverprofile=coverage.out

# Run integration tests (requires real Zitadel + devbox shell)
test-integration:
    go test -tags=integration -v ./tests/integration/... -count=1 -timeout=120s

# Run linters
lint:
    golangci-lint run ./...

# Run Go vulnerability check
vuln:
    govulncheck ./...

# Verify generated files are committed (fails if generate produces uncommitted changes)
verify-generate: generate
    @echo "Checking for uncommitted generated files..."
    @git diff --exit-code -- config/crd/ charts/zitadel-operator-crds/templates/ api/v1alpha1/zz_generated.deepcopy.go || (echo "ERROR: Generated files are out of date. Run 'just generate' and commit." && exit 1)
    @echo "✅ Generated files are up to date."

# Run go mod tidy
tidy:
    go mod tidy

# Clean build artifacts
clean:
    rm -rf bin/ dist/ coverage.out

# Run all checks (build + test + lint + vuln)
check: build test lint vuln

# Build a snapshot release locally (no push, no tag)
snapshot:
    goreleaser release --snapshot --clean

# Package Helm charts locally (for testing)
helm-package:
    helm package charts/zitadel-operator --destination dist/
    helm package charts/zitadel-operator-crds --destination dist/
