packages := $(shell go list ./...)

LINTER_BIN ?= golangci-lint

GO111MODULE ?= on
export GO111MODULE

.PHONY: test
test:
	@go test -v $(packages)

.PHONY: vet
vet:
	@go vet $(packages)

.PHONY: lint
lint:
	@$(LINTER_BIN) run --new-from-rev "HEAD~$(git rev-list master.. --count)" ./...

.PHONY: mod
mod:
	@go mod tidy
