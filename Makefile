# claude-teams-operator Makefile

IMG ?= ghcr.io/camlabs/claude-teams-operator:latest
CLAUDE_CODE_IMG ?= ghcr.io/camlabs/claude-code-runner:latest
KIND_CLUSTER_NAME ?= claude-teams

# Tool versions
CONTROLLER_GEN_VERSION ?= v0.17.0
KUSTOMIZE_VERSION ?= v5.5.0

##@ General

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate CRD manifests
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate deepcopy methods
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt
	go fmt ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run tests
	go test ./... -coverprofile cover.out

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build operator binary
	go build -o bin/manager cmd/manager/main.go

.PHONY: run
run: manifests generate fmt vet ## Run operator locally against cluster
	go run cmd/manager/main.go

.PHONY: docker-build
docker-build: ## Build operator Docker image
	docker build -t $(IMG) -f docker/Dockerfile.operator .

.PHONY: docker-build-runner
docker-build-runner: ## Build Claude Code runner Docker image
	docker build -t $(CLAUDE_CODE_IMG) -f docker/Dockerfile.claude-code .

.PHONY: docker-push
docker-push: ## Push operator image
	docker push $(IMG)

##@ Kind Development

.PHONY: kind-create
kind-create: ## Create Kind cluster for development
	bash hack/kind-setup.sh

.PHONY: kind-delete
kind-delete: ## Delete Kind cluster
	kind delete cluster --name $(KIND_CLUSTER_NAME)

.PHONY: kind-load
kind-load: docker-build docker-build-runner ## Load images into Kind
	kind load docker-image $(IMG) --name $(KIND_CLUSTER_NAME)
	kind load docker-image $(CLAUDE_CODE_IMG) --name $(KIND_CLUSTER_NAME)

##@ Deployment

.PHONY: install
install: manifests ## Install CRDs into cluster
	kubectl apply -f config/crd/bases/

.PHONY: uninstall
uninstall: ## Uninstall CRDs from cluster
	kubectl delete -f config/crd/bases/

.PHONY: deploy
deploy: manifests ## Deploy operator to cluster
	kubectl apply -f config/rbac/
	kubectl apply -f config/manager/

.PHONY: undeploy
undeploy: ## Remove operator from cluster
	kubectl delete -f config/manager/
	kubectl delete -f config/rbac/

.PHONY: sample
sample: ## Deploy sample AgentTeam
	kubectl apply -f config/samples/

##@ Helm

.PHONY: helm-install
helm-install: ## Install via Helm
	helm install claude-teams-operator ./charts/claude-teams-operator \
		--namespace claude-teams-system --create-namespace

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall Helm release
	helm uninstall claude-teams-operator --namespace claude-teams-system

##@ Tools

CONTROLLER_GEN = $(shell go env GOPATH)/bin/controller-gen
.PHONY: controller-gen
controller-gen: ## Install controller-gen
	@test -f $(CONTROLLER_GEN) || go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
