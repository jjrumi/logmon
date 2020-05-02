.ONESHELL:

SHELL := /bin/bash
.SHELLFLAGS := -ec

BIN=$(CURDIR)/bin
ACCESS_LOG=/tmp/access.log

###
## Targets to build and run a Go-ready environment:
###

build: ## Build the log monitor binary
	docker build -t jjrumi/logmon .

bash:
	docker run --name logmon --rm -it jjrumi/logmon bash

attach:
	docker exec -it logmon bash

###
## Targets for a Go-ready environment:
###

go-install: ## Install dependencies
	go mod vendor

go-test: ## Run the tests
	go test -race -v -count=1 ./...

go-build: ## Build the log monitor binary
	go build \
		-mod=vendor \
		-o $(BIN)/logmon \
		./cmd

sim-slow-traffic: ## Write 1 req/s into the $ACCESS_LOG file
	flog -n 1 -l -d 1 >> $(ACCESS_LOG)
