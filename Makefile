# Makefile for building Chaos Scheduler
# Reference Guide - https://www.gnu.org/software/make/manual/make.html

IS_DOCKER_INSTALLED = $(shell which docker >> /dev/null 2>&1; echo $$?)

# list only our namespaced directories
PACKAGES = $(shell go list ./...)

# docker info
DOCKER_REPO ?= litmuschaos
DOCKER_IMAGE ?= chaos-scheduler
DOCKER_TAG ?= ci

.PHONY: help
help:
	@echo ""
	@echo "Usage:-"
	@echo "\tmake godeps                  -- sets up dependencies for image build"
	@echo "\tmake build-chaos-scheduler   -- builds the chaos scheduler image"
	@echo "\tmake push-chaos-scheduler    -- pushes the chaos scheduler image"
	@echo "\tmake build-amd64             -- builds the chaos scheduler amd64 image"
	@echo ""

.PHONY: all
all: godeps build-chaos-scheduler test push-chaos-scheduler

.PHONY: godeps
godeps:
	@echo ""
	@echo "INFO:\tverifying dependencies for chaos scheduler build ..."
	@go get  -v golang.org/x/lint/golint
	@go get  -v golang.org/x/tools/cmd/goimports

_build_check_docker:
	@if [ $(IS_DOCKER_INSTALLED) -eq 1 ]; \
		then echo "" \
		&& echo "ERROR:\tdocker is not installed. Please install it before build." \
		&& echo "" \
		&& exit 1; \
		fi;

.PHONY: codegen
codegen:
	@echo "------------------"
	@echo "--> Updating Codegen"
	@echo "------------------"
	${GOPATH}/src/k8s.io/code-generator/generate-groups.sh all \
	github.com/litmuschaos/chaos-scheduler/pkg/client github.com/litmuschaos/chaos-scheduler/pkg/apis \
	litmuschaos:v1alpha1

.PHONY: test
test:
	@echo "------------------"
	@echo "--> Run Go Test"
	@echo "------------------"
	@go test ./... -v 

.PHONY: unused-package-check
unused-package-check:
	@echo "------------------"
	@echo "--> Check unused packages for the chaos-operator"
	@echo "------------------"
	@tidy=$$(go mod tidy); \
	if [ -n "$${tidy}" ]; then \
		echo "go mod tidy checking failed!"; echo "$${tidy}"; echo; \
	fi

.PHONY: build-chaos-scheduler 
build-chaos-scheduler: 
	@echo "------------------"
	@echo "--> Build chaos-scheduler docker image" 
	@echo "------------------"
	@docker buildx build --file build/Dockerfile --progress plane --no-cache --platform linux/arm64,linux/amd64 --tag $(DOCKER_REPO)/$(DOCKER_IMAGE):$(DOCKER_TAG) .

.PHONY: push-chaos-scheduler
push-chaos-scheduler:
	@echo "------------------------------"
	@echo "--> Pushing image" 
	@echo "------------------------------"
	@docker buildx build --file build/Dockerfile --progress plane --no-cache --push --platform linux/arm64,linux/amd64 --tag $(DOCKER_REPO)/$(DOCKER_IMAGE):$(DOCKER_TAG) .

.PHONY: build-amd64
build-amd64:
	@echo "--------------------------------------------"
	@echo "--> Build chaos-scheduler amd-64 docker image"
	@echo "--------------------------------------------"
	sudo docker build --file build/Dockerfile --tag $(DOCKER_REPO)/$(DOCKER_IMAGE):$(DOCKER_TAG) . --build-arg TARGETARCH=amd64