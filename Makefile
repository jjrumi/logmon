.ONESHELL:

SHELL := /bin/bash
.SHELLFLAGS := -ec

BIN=$(CURDIR)/bin
ACCESS_LOG=/tmp/access.log

go-install-vendor: ## Install dependencies
	go mod vendor

go-test: ## Run the tests
	go test -race -v -count=1 -p 1 ./...

go-build: ## Build the log monitor binary
	go build \
		-mod=vendor \
		-o $(BIN)/logmon \
		./cmd

sim-slow-traffic: ## Continuously write log entries into the $ACCESS_LOG file
	flog -n 10 -l -d 2 -s 1 >> $(ACCESS_LOG)