# Makefile for building Chaos Exporter
# Reference Guide - https://www.gnu.org/software/make/manual/make.html

IS_DOCKER_INSTALLED = $(shell which docker >> /dev/null 2>&1; echo $$?)

# list only our namespaced directories
PACKAGES = $(shell go list ./...)

# docker info
DOCKER_REPO ?= litmuschaos
DOCKER_IMAGE ?= chaos-scheduler
DOCKER_TAG ?= latest

.PHONY: all
all: deps format lint build test dockerops

.PHONY: help
help:
	@echo ""
	@echo "Usage:-"
	@echo "\tmake deps      -- sets up dependencies for image build"
	@echo "\tmake gotasks   -- builds the chaos scheduler binary"
	@echo "\tmake dockerops -- builds & pushes the chaos scheduler image"
	@echo ""

.PHONY: deps
deps: _build_check_docker godeps 

.PHONY: godeps
godeps:
	@echo ""
	@echo "INFO:\tverifying dependencies for chaos scheduler build ..."
	@go get  -v golang.org/x/lint/golint
	@go get  -v golang.org/x/tools/cmd/goimports

.PHONY: _build_check_docker
_build_check_docker:
	@if [ $(IS_DOCKER_INSTALLED) -eq 1 ]; \
		then echo "" \
		&& echo "ERROR:\tdocker is not installed. Please install it before build." \
		&& echo "" \
		&& exit 1; \
		fi;

.PHONY: gotasks
gotasks: format lint build
 
.PHONY: format
format:
	@echo "------------------"
	@echo "--> Running go fmt"
	@echo "------------------"
	@go fmt $(PACKAGES)

.PHONY: lint
lint:
	@echo "------------------"
	@echo "--> Running golint"
	@echo "------------------"
	@golint $(PACKAGES)
	@echo "------------------"
	@echo "--> Running go vet"
	@echo "------------------"
	@go vet $(PACKAGES)

.PHONY: build  
build:
	@echo "------------------"
	@echo "--> Build Chaos Scheduler"
	@echo "------------------"
	@go build -o ${GOPATH}/src/github.com/litmuschaos/chaos-scheduler/build/_output/bin/chaos-scheduler -gcflags all=-trimpath=${GOPATH} -asmflags all=-trimpath=${GOPATH} github.com/litmuschaos/chaos-scheduler/cmd/manager 

.PHONY: test
test:
	@echo "------------------"
	@echo "--> Run Go Test"
	@echo "------------------"
	@go test ./... -v 

.PHONY: dockerops 
dockerops: 
	@echo "------------------"
	@echo "--> Build & Push chaos-scheduler docker image" 
	@echo "------------------"
	sudo docker build . -f build/Dockerfile -t $(DOCKER_REPO)/$(DOCKER_IMAGE):$(DOCKER_TAG)
