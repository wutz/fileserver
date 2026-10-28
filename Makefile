# Image URL to use all building/pushing image targets
IMG ?= ghcr.io/wutz/fileserver:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= docker

# CONFIG_NAMESPACE is where `make run` reads the operator config ConfigMap.
CONFIG_NAMESPACE ?= fileserver-system

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate the CRD and RBAC from the markers.
	"$(CONTROLLER_GEN)" rbac:roleName=manager-role crd paths="./..." output:crd:artifacts:config=config/crd/bases output:rbac:artifacts:config=config/rbac

.PHONY: generate
generate: controller-gen ## Generate the DeepCopy methods.
	"$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter.
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes.
	"$(GOLANGCI_LINT)" run --fix

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run the tests against envtest.
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test ./... -coverprofile cover.out

.PHONY: build
build: manifests generate fmt vet ## Build the manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run the operator from your host against the current kubeconfig context.
	go run ./cmd/main.go --config-namespace "$(CONFIG_NAMESPACE)"

.PHONY: docker-build
docker-build: ## Build the manager image.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push the manager image.
	$(CONTAINER_TOOL) push ${IMG}

##@ Helm

HELM ?= helm
CHART_DIR ?= charts/fileserver
CHART_VERSION ?= 0.3.0
CHART_REPO ?= oci://ghcr.io/wutz/charts

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart.
	$(HELM) lint $(CHART_DIR)

.PHONY: helm-template
helm-template: ## Render the Helm chart with its default values.
	$(HELM) template fileserver $(CHART_DIR) --namespace fileserver-system

.PHONY: helm-package
helm-package: ## Package the Helm chart into dist/.
	mkdir -p dist
	$(HELM) package $(CHART_DIR) --version $(CHART_VERSION) --app-version $(CHART_VERSION) --destination dist

.PHONY: helm-push
helm-push: ## Push the packaged Helm chart to the OCI registry.
	$(HELM) push dist/fileserver-$(CHART_VERSION).tgz $(CHART_REPO)

##@ Build dependencies

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint

CONTROLLER_TOOLS_VERSION ?= v0.22.0
ENVTEST_VERSION ?= v0.25.2
ENVTEST_K8S_VERSION ?= 1.37
GOLANGCI_LINT_VERSION ?= v2.13.1

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" || true ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(1)" "$(1)-$(3)" ;\
} ;\
ln -sf "$(1)-$(3)" "$(1)"
endef
