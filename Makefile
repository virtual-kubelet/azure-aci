TOOLS_DIR := hack/tools
TOOLS_BIN_DIR := $(abspath $(TOOLS_DIR)/bin)

GOLANGCI_LINT_VER := v1.51.0
GOLANGCI_LINT_BIN := golangci-lint
GOLANGCI_LINT := $(abspath $(TOOLS_BIN_DIR)/$(GOLANGCI_LINT_BIN)-$(GOLANGCI_LINT_VER))

GOIMPORTS_VER := latest
GOIMPORTS_BIN := goimports
GOIMPORTS := $(abspath $(TOOLS_BIN_DIR)/$(GOIMPORTS_BIN)-$(GOIMPORTS_VER))

# Scripts
GO_INSTALL := ./hack/go-install.sh
AKS_E2E_SCRIPT := ./hack/e2e/aks.sh
AKS_ADDON_E2E_SCRIPT := ./hack/e2e/aks-addon.sh

GO111MODULE := on
export GO111MODULE

TEST_CREDENTIALS_DIR ?= $(abspath .azure)
TEST_AKS_CREDENTIALS_JSON ?= $(TEST_CREDENTIALS_DIR)/aks_credentials.json
TEST_CREDENTIALS_JSON ?= $(TEST_CREDENTIALS_DIR)/credentials.json
TEST_LOGANALYTICS_JSON ?= $(TEST_CREDENTIALS_DIR)/loganalytics.json
export TEST_CREDENTIALS_JSON TEST_LOGANALYTICS_JSON TEST_AKS_CREDENTIALS_JSON

VERSION ?= v1.6.1
REGISTRY ?= ghcr.io
IMG_NAME ?= virtual-kubelet
INIT_IMG_NAME ?= init-validation
IMAGE ?= $(REGISTRY)/$(IMG_NAME)
INIT_IMAGE ?= $(REGISTRY)/$(INIT_IMG_NAME)
LOCATION := $(E2E_REGION)
E2E_CLUSTER_NAME := $(CLUSTER_NAME)

OUTPUT_TYPE ?= type=docker
BUILDPLATFORM ?= linux/amd64
IMG_TAG ?= $(subst v,,$(VERSION))
INIT_IMG_TAG ?= 0.2.0

BUILD_DATE ?= $(shell date '+%Y-%m-%dT%H:%M:%S')
VERSION_FLAGS := "-ldflags=-X main.buildVersion=$(IMG_TAG) -X main.buildTime=$(BUILD_DATE)"

## --------------------------------------
## Tooling Binaries
## --------------------------------------

$(GOLANGCI_LINT):
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) github.com/golangci/golangci-lint/cmd/golangci-lint $(GOLANGCI_LINT_BIN) $(GOLANGCI_LINT_VER)

# GOIMPORTS
$(GOIMPORTS):
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) golang.org/x/tools/cmd/goimports $(GOIMPORTS_BIN) $(GOIMPORTS_VER)

BUILDX_BUILDER_NAME ?= img-builder
QEMU_VERSION ?= 5.2.0-2

.PHONY: docker-buildx-builder
docker-buildx-builder:
	@if ! docker buildx ls | grep $(BUILDX_BUILDER_NAME); then \
  		docker run --rm --privileged multiarch/qemu-user-static:$(QEMU_VERSION) --reset -p yes; \
		docker buildx create --name $(BUILDX_BUILDER_NAME) --use; \
		docker buildx inspect $(BUILDX_BUILDER_NAME) --bootstrap; \
	fi

.PHONY: docker-build-image
docker-build-image: docker-buildx-builder
	 docker buildx build \
		--file docker/virtual-kubelet/Dockerfile \
		--build-arg VERSION_FLAGS=$(VERSION_FLAGS) \
		--output=$(OUTPUT_TYPE) \
		--platform="$(BUILDPLATFORM)" \
		--pull \
		--tag $(IMAGE):$(IMG_TAG) .

.PHONY: docker-build-init-image
docker-build-init-image: docker-buildx-builder
	docker buildx build \
		--file docker/init-container/Dockerfile \
		--output=$(OUTPUT_TYPE) \
		--platform="$(BUILDPLATFORM)" \
		--pull \
		--tag $(INIT_IMAGE):$(INIT_IMG_TAG) .

.PHONY: clean
clean: files := bin/virtual-kubelet bin/virtual-kubelet.tgz
clean:
	@rm -f $(files) &>/dev/null || exit 0

## --------------------------------------
## Tests
## --------------------------------------

.PHONY: unit-tests
unit-tests: testauth
	@echo running tests
	LOCATION=$(LOCATION) AKS_CREDENTIAL_LOCATION=$(TEST_AKS_CREDENTIALS_JSON) \
	AZURE_AUTH_LOCATION=$(TEST_CREDENTIALS_JSON) \
	LOG_ANALYTICS_AUTH_LOCATION=$(TEST_LOGANALYTICS_JSON) \
	go test -v $(shell go list ./... | grep -v /e2e) -race -coverprofile=coverage.txt -covermode=atomic fmt
	go tool cover -func=coverage.txt

.PHONY: e2e-test
e2e-test:
	PR_RAND=$(PR_COMMIT_SHA) E2E_TARGET=$(E2E_TARGET) \
 	IMG_URL=$(REGISTRY) IMG_REPO=$(IMG_NAME) IMG_TAG=$(IMG_TAG) \
 	INIT_IMG_REPO=$(INIT_IMG_NAME) INIT_IMG_TAG=$(INIT_IMG_TAG) \
 	LOCATION=$(LOCATION) RESOURCE_GROUP=$(E2E_CLUSTER_NAME) \
 	$(AKS_E2E_SCRIPT) go test -timeout 90m -v ./e2e

.PHONY: aks-addon-e2e-test
aks-addon-e2e-test:
	PR_RAND=$(PR_COMMIT_SHA) E2E_TARGET=$(E2E_TARGET) \
	IMG_URL=$(REGISTRY) IMG_REPO=$(IMG_NAME) IMG_TAG=$(IMG_TAG) \
	INIT_IMG_REPO=$(INIT_IMG_NAME) INIT_IMG_TAG=$(INIT_IMG_TAG) \
	LOCATION=$(LOCATION) RESOURCE_GROUP=$(E2E_CLUSTER_NAME) \
	$(AKS_ADDON_E2E_SCRIPT) go test -timeout 90m -v ./e2e

.PHONY: vet
vet:
	@go vet ./... #$(packages)

## --------------------------------------
## Linting
## --------------------------------------

.PHONY: lint
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run -v

.PHONY: lint-full
lint-full: $(GOLANGCI_LINT) ## Run slower linters to detect possible issues
	$(GOLANGCI_LINT) run -v --fast=false

.PHONY: mod
mod:
	@go mod tidy

.PHONY: fmt
fmt:  $(GOIMPORTS) ## Run go fmt against code.
	go fmt ./...
	$(GOIMPORTS) -w $$(go list -f {{.Dir}} ./...)

.PHONY: testauth
testauth: test-cred-json test-aks-cred-json test-loganalytics-json

test-cred-json:
	@echo Building test credentials
	chmod a+x hack/ci/create_credentials.sh
	hack/ci/create_credentials.sh

test-aks-cred-json:
	@echo Building test AKS credentials
	chmod a+x hack/ci/create_aks_credentials.sh
	hack/ci/create_aks_credentials.sh

test-loganalytics-json:
	@echo Building log analytics credentials
	chmod a+x hack/ci/create_loganalytics_auth.sh
	hack/ci/create_loganalytics_auth.sh

.PHONY: release-manifest
release-manifest:
	@sed -i -e 's/^VERSION ?= .*/VERSION ?= ${VERSION}/' ./Makefile
	@sed -i -e "s/version: .*/version: ${IMG_TAG}/" ./charts/virtual-kubelet/Chart.yaml
	@sed -i -e "s/tag: .*/tag: ${IMG_TAG}/" ./charts/virtual-kubelet/values.yaml
	@sed -i -e 's/RELEASE_TAG=.*/RELEASE_TAG=${IMG_TAG}/' ./charts/virtual-kubelet/README.md
	@sed -i -e 's/RELEASE_TAG=.*/RELEASE_TAG=${IMG_TAG}/' ./docs/UPGRADE-README.md
	@sed -i -e 's/RELEASE_TAG=.*/RELEASE_TAG=${IMG_TAG}/' README.md
