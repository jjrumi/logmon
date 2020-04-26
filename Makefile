.ONESHELL:

SHELL := /bin/bash
.SHELLFLAGS := -ec

BIN=$(CURDIR)/bin

go-install-vendor: ## Install dependencies
	go mod vendor

go-test: ## Run the tests
	go test -race -v -count=1 -p 1 ./...

go-build: ## Build the log monitor binary
	go build \
		-mod=vendor \
		-o $(BIN)/logmon \
		./cmd
