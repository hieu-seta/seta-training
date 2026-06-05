# seta-training — top-level Makefile
# Workspace has independent modules; loop over them for build/test/lint.

MODULES := pkg tools/migrator services/auth services/team services/asset services/audit-worker
GOLANGCI := $(shell go env GOPATH)/bin/golangci-lint

.PHONY: help up down logs ps lint test test-int build clean tidy e2e demo cover cover-html tools

help:
	@echo "targets: up down logs ps lint test test-int build tidy tools e2e demo cover cover-html clean"

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

ps:
	docker compose ps

tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

lint:
	@for m in $(MODULES); do echo "== lint $$m =="; (cd $$m && $(GOLANGCI) run ./...) || exit 1; done

test:
	@for m in $(MODULES); do echo "== test $$m =="; (cd $$m && go test ./... -race -short) || exit 1; done

test-int:
	@for m in $(MODULES); do echo "== test-int $$m =="; (cd $$m && go test ./... -race -tags=integration -timeout=5m) || exit 1; done

build:
	@for m in $(MODULES); do echo "== build $$m =="; (cd $$m && go build ./...) || exit 1; done

tidy:
	@for m in $(MODULES); do echo "== tidy $$m =="; (cd $$m && go mod tidy) || exit 1; done

e2e:
	@bash scripts/e2e/run_all.sh

demo:
	@bash scripts/demo.sh

cover:
	@for m in $(MODULES); do \
	  echo "== cover $$m =="; \
	  (cd $$m && go test -race -short -coverprofile=coverage.out -covermode=atomic ./... && go tool cover -func=coverage.out | tail -1) || exit 1; \
	done

cover-html:
	@for m in $(MODULES); do \
	  (cd $$m && go test -race -short -coverprofile=coverage.out -covermode=atomic ./... && go tool cover -html=coverage.out -o coverage.html); \
	done

clean:
	rm -rf bin/ tmp/
	@find . -name coverage.out -delete
	@find . -name coverage.html -delete
