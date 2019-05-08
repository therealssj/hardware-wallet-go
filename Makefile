.DEFAULT_GOAL := help
.PHONY: all build
.PHONY: test_unit test_integration test
.PHONY: dep mocks
.PHONY: clean lint check format

GOPATH  ?= $(HOME)/go

all: build

build:
	cd cmd/cli && ./install.sh

dep: ## Ensure package dependencies are up to date
	dep ensure -v

mocks: ## Create all mock files for unit tests
	echo "Generating mock files"
	mockery -name Devicer -dir ./src/skywallet -case underscore -inpkg -testonly
	mockery -name DeviceDriver -dir ./src/skywallet -case underscore -inpkg -testonly

test_unit: ## Run unit tests
	go test -v github.com/skycoin/hardware-wallet-go/src/skywallet

integration-test-emulator: ## Run emulator integration tests
	./ci-scripts/integration-test.sh -a -m EMULATOR -n emulator-integration

integration-test-wallet: ## Run usb integration tests
	./ci-scripts/integration-test.sh -m USB -n wallet-integration

test: test_unit integration-test-emulator ## Run all tests

install-linters: ## Install linters
	go get -u github.com/FiloSottile/vendorcheck
	# For some reason this install method is not recommended, see https://github.com/golangci/golangci-lint#install
	# However, they suggest `curl ... | bash` which we should not do
	go get -u github.com/golangci/golangci-lint/cmd/golangci-lint

check: lint test ## Perform self-tests

lint: ## Run linters. Use make install-linters first.
	vendorcheck ./...
	golangci-lint run -c .golangci.yml ./...

format: ## Formats the code. Must have goimports installed (use make install-linters).
	goimports -w -local github.com/skycoin/hardware-wallet-go ./cmd
	goimports -w -local github.com/skycoin/hardware-wallet-go ./src

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
