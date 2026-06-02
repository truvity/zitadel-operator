# Zitadel Operator development commands

# Generate deepcopy methods and CRD manifests
generate:
    controller-gen object paths="./api/..."
    controller-gen crd paths="./api/..." output:crd:artifacts:config=config/crd/bases

# Build the operator binary
build:
    go build -o bin/zitadel-operator ./cmd/operator/

# Run tests
test:
    go test ./... -coverprofile=coverage.out

# Run linters
lint:
    golangci-lint run ./...

# Run Go vulnerability check
vuln:
    govulncheck ./...

# Generate CRD manifests only
manifests:
    controller-gen crd paths="./api/..." output:crd:artifacts:config=config/crd/bases

# Run go mod tidy
tidy:
    go mod tidy

# Clean build artifacts
clean:
    rm -rf bin/ dist/ coverage.out

# Run all checks (generate + build + test + lint + vuln)
check: generate build test lint vuln

# Create a release tag and push (triggers GitHub Actions release workflow)
release version:
    @echo "Tagging v{{version}} and pushing..."
    git tag -a "v{{version}}" -m "Release v{{version}}"
    git push origin "v{{version}}"
    @echo "Release v{{version}} triggered. Watch: https://github.com/truvity/zitadel-operator/actions"

# Build a snapshot release locally (no push, no tag)
snapshot:
    goreleaser release --snapshot --clean

# Package Helm charts locally (for testing)
helm-package:
    helm package charts/zitadel-operator --destination dist/
    helm package charts/zitadel-operator-crds --destination dist/

# Push Helm charts to GHCR (requires: helm registry login ghcr.io)
helm-push version:
    helm package charts/zitadel-operator --version {{version}} --app-version {{version}} --destination dist/
    helm package charts/zitadel-operator-crds --version {{version}} --app-version {{version}} --destination dist/
    helm push dist/zitadel-operator-{{version}}.tgz oci://ghcr.io/truvity/charts
    helm push dist/zitadel-operator-crds-{{version}}.tgz oci://ghcr.io/truvity/charts
