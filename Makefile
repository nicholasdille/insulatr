PACKAGE = insulatr
STATIC  = insulatr-$(shell uname -m)
SOURCE  = *.go
GOPATH  = $(CURDIR)/.gopath
BIN     = $(GOPATH)/bin
BASE    = $(GOPATH)/src/$(PACKAGE)
GO      = go
GOLINT  = $(BIN)/golint
GOFMT   = gofmt
GLIDE   = glide

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

$(PACKAGE): bin/$(PACKAGE)

bin/$(PACKAGE): $(SOURCE) ; $(info $(M) Building $(PACKAGE)...)
	@cd $(BASE) && $(GO) build -o bin/$(PACKAGE) $(SOURCE)

static: bin/$(STATIC)

bin/$(STATIC): $(SOURCE) ; $(info $(M) Building static $(PACKAGE)...)
	@cd $(BASE) && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -a -tags netgo -ldflags '-w' -o bin/$(STATIC) $(SOURCE)

check-docker:
	@docker version >/dev/null

docker: check-docker static ; $(info $(M) Building container image...)
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