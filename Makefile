PACKAGE  = insulatr
STATIC   = insulatr-$(shell uname -m)
SOURCE   = $(shell echo *.go)
GOPATH   = $(CURDIR)/.gopath
BIN      = $(GOPATH)/bin
BASE     = $(GOPATH)/src/$(PACKAGE)
GO       = go
GOLINT   = $(BIN)/golint
GOFMT    = gofmt
GLIDE    = glide
BUILDDEF = insulatr.yaml

GIT_COMMIT = $(shell git rev-list -1 HEAD)
BUILD_TIME = $(shell date +%Y%m%d-%H%M%S)
GIT_TAG = $(shell git describe --tags 2>/dev/null)

M = $(shell printf "\033[34;1m▶\033[0m")

.DEFAULT_GOAL := $(PACKAGE)

.PHONY: clean deps format linter check static check-docker docker test run

clean: ; $(info $(M) Cleaning...)
	@rm -rf $(GOPATH)
	@rm bin/*

$(BASE): ; $(info $(M) Creating link...)
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@

$(GOLINT): $(BASE) ; $(info $(M) Installing linter...)
	@$(GO) get github.com/golang/lint/golint

deps: $(BASE) ; $(info $(M) Updating dependencies...)
	@$(GLIDE) update

format: $(BASE) ; $(info $(M) Running formatter...)
	@$(GOFMT) -l -w $(SOURCE)

lint: $(BASE) $(GOLINT) ; $(info $(M) Running linter...)
	@$(GOLINT) $(PACKAGE)

check: format lint

%.sha256: ; $(info $(M) Creating SHA256 for $*...)
	@echo sha256sum $* > $@

binary: $(PACKAGE)

$(PACKAGE): bin/$(PACKAGE) bin/$(PACKAGE).sha256

bin/$(PACKAGE): $(BASE) $(SOURCE) ; $(info $(M) Building $(PACKAGE)...)
	@cd $(BASE) && $(GO) build -ldflags "-X main.GitCommit=$(GIT_COMMIT) -X main.BuildTime=$(BUILD_TIME) -X main.Version=$(GIT_TAG)" -o $@ $(SOURCE)

static: bin/$(STATIC) bin/$(STATIC).sha256

bin/$(STATIC): $(BASE) $(SOURCE) ; $(info $(M) Building static $(PACKAGE)...)
	@cd $(BASE) && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -a -tags netgo -ldflags "-w -X main.GitCommit=$(GIT_COMMIT) -X main.BuildTime=$(BUILD_TIME) -X main.Version=$(GIT_TAG)" -o $@ $(SOURCE)

check-docker:
	@docker version >/dev/null

docker: check-docker ; $(info $(M) Building container image...)
	@docker build --tag $(PACKAGE) .

test: docker ; $(info $(M) Building container image for testing...)
	@docker build --file Dockerfile.tests --tag $(PACKAGE):tests .
	@for FILE in $$(cd tests && ls *.yaml); do \
		echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"; \
		echo "Running test with $${FILE}"; \
		docker run -it --rm --volume /var/run/docker.sock:/var/run/docker.sock $(PACKAGE):tests --remove-volume --remove-network --file /tmp/$${FILE}; \
	done; \
	exit 0

run: docker ; $(info $(M) Running $(PACKAGE)...)
	@docker run -it --rm --volume /var/run/docker.sock:/var/run/docker.sock $(PACKAGE) --remove-volume --remove-network

scp-%: binary ; $(info $(M) Copying to $*)
	@cat bin/$(PACKAGE) | gzip | ssh $* sh -c 'gunzip > $(PACKAGE)'

ssh-%: scp-% ; $(info $(M) Running remotely on $*)
	@ssh $* ./$(PACKAGE) --file $(BUILDDEF)

tag-%: ; $(info $(M) Tagging as $*)
	@hub tag $*

changelog-%: ; $(info $(M) Creating changelog for milestone $* on $(GIT_TAG))
	@( \
	    echo Version $(GIT_TAG); \
	    echo; \
	    hub issue -M $* -s closed -f "[%t](%U)%n" | while read LINE; do echo "- $$LINE"; done; \
	) > $(GIT_TAG).txt

release-%: static changelog-% ; $(info $(M) Releasing milestone $* as $(GIT_TAG))
	@hub release create -F $(GIT_TAG).txt -a bin/$(STATIC) -a bin/$(STATIC).sha256 $(GIT_TAG)
