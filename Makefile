.PHONY: generate build test lint manifests clean

# Generate code (deepcopy, CRD manifests)
generate:
	go generate ./...
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

# Generate CRD manifests only
manifests:
	controller-gen crd paths="./api/..." output:crd:artifacts:config=config/crd/bases

# Clean build artifacts
clean:
	rm -rf bin/ dist/ coverage.out
