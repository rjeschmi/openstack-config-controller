# Makefile for openstack-config-controller

.DEFAULT_GOAL := all

BINARY := bin/manager
GO := go

.PHONY: all build run clean fmt generate test

all: build

build:
	$(GO) build -o $(BINARY) main.go

run:
	$(GO) run .

clean:
	rm -f $(BINARY)

fmt:
	gofmt -w .

generate:
	@echo "Generating CRDs..."
	$(GO) run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.11.0 crd:crdVersions=v1 paths=./api/... output:artifacts:config=config/crd/bases

test:
	$(GO) test ./...
