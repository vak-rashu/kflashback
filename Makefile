IMG ?= ghcr.io/prashanthjos/kflashback:latest
BINARY ?= kflashback

.PHONY: all build run test lint docker-build docker-push install deploy clean ui-build ui-dev help

all: build

##@ Development

build: ## Build the controller binary.
	go build -o bin/$(BINARY) ./cmd/kflashback/

run: ## Run the controller locally.
	go run ./cmd/kflashback/ --config-name="" --storage-backend=sqlite --storage-dsn=./kflashback.db --ui-dir=./ui/dist

test: ## Run tests.
	go test ./... -coverprofile cover.out

lint: ## Run linters.
	golangci-lint run ./...

generate: ## Generate code (deepcopy, CRD manifests).
	controller-gen object paths="./api/..."
	controller-gen crd paths="./api/..." output:crd:dir=config/crd

fmt: ## Run go fmt.
	go fmt ./...

vet: ## Run go vet.
	go vet ./...

##@ UI

ui-install: ## Install UI dependencies.
	cd ui && npm ci

ui-dev: ## Run UI dev server.
	cd ui && npm run dev

ui-build: ## Build UI for production.
	cd ui && npm run build

##@ Deployment

install: ## Install CRDs into the cluster.
	kubectl apply -f config/crd/

uninstall: ## Uninstall CRDs from the cluster.
	kubectl delete -f config/crd/

deploy: install ## Deploy controller to the cluster.
	kubectl apply -f config/rbac/
	kubectl apply -f config/manager/

undeploy: ## Undeploy controller from the cluster.
	kubectl delete -f config/manager/
	kubectl delete -f config/rbac/
	kubectl delete -f config/crd/

##@ Docker

docker-build: ## Build docker image.
	docker build -t $(IMG) .

docker-push: ## Push docker image.
	docker push $(IMG)

##@ Cleanup

clean: ## Clean build artifacts.
	rm -rf bin/ cover.out ui/dist ui/node_modules

##@ Help

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
