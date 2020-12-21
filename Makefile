#!/usr/bin/make
# Makefile readme (ru): <http://linux.yaroslavl.ru/docs/prog/gnu_make_3-79_russian_manual.html>
# Makefile readme (en): <https://www.gnu.org/software/make/manual/html_node/index.html#SEC_Contents>

SHELL = /bin/sh

.DEFAULT_GOAL := build

# This will output the help for each task. thanks to https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help: ## Show this help
	@printf "\033[33m%s:\033[0m\n" 'Available commands'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[32m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build RR binary file for local os/arch
	CGO_ENABLED=0 go build -trimpath -ldflags "-s" -o ./rr ./cmd/rr/main.go

clean: ## Make some clean
	rm ./rr

install: build ## Build and install RR locally
	cp rr /usr/local/bin/rr

uninstall: ## Uninstall locally installed RR
	rm -f /usr/local/bin/rr

test: ## Run application tests
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/checker
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/config
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/gzip
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/headers
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/http
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/informer
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/logger
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/metrics
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/redis
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/reload
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/resetter
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/rpc
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/server
	go test -v -race -cover -tags=debug -covermode=atomic ./a_plugin_tests/static

lint: ## Run application linters
	go fmt ./...
	golint ./...
