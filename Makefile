PACKAGE  = insulatr
IMAGE    = nicholasdille/$(PACKAGE)
STATIC   = insulatr-$(shell uname -m)
SOURCE   = $(shell echo *.go)
GOPATH   = $(CURDIR)/.gopath
BIN      = $(GOPATH)/bin
BASE     = $(GOPATH)/src/$(PACKAGE)
GO       = go
GOLINT   = $(BIN)/golint
DEPTH    = $(BIN)/depth
GOFMT    = gofmt
GLIDE    = glide
SEMVER   = $(BIN)/semver
BUILDDEF = insulatr.yaml

GIT_COMMIT = $(shell git rev-list -1 HEAD)
BUILD_TIME = $(shell date +%Y%m%d-%H%M%S)
GIT_TAG = $(shell git describe --tags 2>/dev/null)

M = $(shell printf "\033[34;1m▶\033[0m")

.DEFAULT_GOAL := $(PACKAGE)

.PHONY: clean deps format linter check static check-docker docker test run

clean: ; $(info $(M) Cleaning...)
	@rm bin/*

$(BASE): ; $(info $(M) Creating link...)
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@

$(GOLINT): $(BASE) ; $(info $(M) Installing linter...)
	@$(GO) get github.com/golang/lint/golint

$(GLIDE): ; $(info $(M) Installing glide...)
	@curl https://glide.sh/get | sh

$(DEPTH): $(BASE) ; $(info $(M) Installing depth...)
	@$(GO) get github.com/KyleBanks/depth/cmd/depth

$(SEMVER): $(BASE); $(info $(M) Installing semver...)
	@curl -sLf https://github.com/fsaintjacques/semver-tool/raw/2.1.0/src/semver > $@
	@chmod +x $@

semver: $(SEMVER)

depupdate: $(BASE) $(GLIDE) ; $(info $(M) Updating dependencies...)
	@$(GLIDE) update

deps: $(BASE) $(GLIDE) ; $(info $(M) Updating dependencies...)
	@$(GLIDE) install

format: $(BASE) ; $(info $(M) Running formatter...)
	@$(GOFMT) -l -w $(SOURCE)

lint: $(GOLINT) ; $(info $(M) Running linter...)
	@$(GOLINT) $(PACKAGE)

deptree: $(DEPTH) ; $(info $(M) Creating dependency tree...)
	@$(DEPTH) .

check: format lint

%.sha256: % ; $(info $(M) Creating SHA256 for $*...)
	@echo sha256sum $* > $@

%.asc: % ; $(info $(M) Creating signature for $*...)
	@gpg --local-user $$(git config --get user.signingKey) --sign --armor --detach-sig --yes $*

binary: $(PACKAGE)

$(PACKAGE): bin/$(PACKAGE) bin/$(PACKAGE).sha256 bin/$(PACKAGE).asc

bin/$(PACKAGE): $(BASE) $(SOURCE) ; $(info $(M) Building $(PACKAGE)...)
	@cd $(BASE) && $(GO) build -ldflags "-s -w -X main.GitCommit=$(GIT_COMMIT) -X main.BuildTime=$(BUILD_TIME) -X main.Version=$(GIT_TAG)" -o $@ $(SOURCE)

static: bin/$(STATIC) bin/$(STATIC).sha256 bin/$(STATIC).asc

bin/$(STATIC): $(BASE) $(SOURCE) ; $(info $(M) Building static $(PACKAGE)...)
	@cd $(BASE) && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -a -tags netgo -ldflags "-s -w -X main.GitCommit=$(GIT_COMMIT) -X main.BuildTime=$(BUILD_TIME) -X main.Version=$(GIT_TAG)" -o $@ $(SOURCE)

check-docker:
	@docker version >/dev/null

docker: $(IMAGE)-master

$(IMAGE)-%: check-docker ; $(info $(M) Building container image $(IMAGE):$*...)
	@docker build --build-arg REF=$* --tag $(IMAGE):$* .

test: docker ; $(info $(M) Building container image for testing...)
	@docker build --file Dockerfile.tests --tag $(IMAGE):tests .
	@for FILE in $$(cd tests && ls *.yaml); do \
		echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"; \
		echo "Running test with $${FILE}"; \
		docker run -it --rm --volume /var/run/docker.sock:/var/run/docker.sock $(IMAGE):tests --remove-volume --remove-network --file /tmp/$${FILE}; \
	done; \
	exit 0

run: docker ; $(info $(M) Running $(PACKAGE)...)
	@docker run -it --rm --volume /var/run/docker.sock:/var/run/docker.sock $(IMAGE) --remove-volume --remove-network

scp-%: binary ; $(info $(M) Copying to $*)
	@tar -cz bin/$(PACKAGE) $(BUILDDEF) | ssh $* tar -xvz

ssh-%: scp-% ; $(info $(M) Running remotely on $*)
	@ssh $* ./bin/$(PACKAGE) --file $(BUILDDEF) $(PARAMS)

tag-%: ; $(info $(M) Tagging as $*)
	@hub tag --annotate --sign $* --message "Version $*"
	@hub push origin $*

changelog-%: ; $(info $(M) Creating changelog for milestone $* on $(GIT_TAG))
	@( \
	    echo Version $(GIT_TAG); \
	    echo; \
	    hub issue -M $* -s closed -f "[%t](%U)%n" | while read LINE; do echo "- $$LINE"; done; \
	) > $(GIT_TAG).txt

extract-%: ; $(info $(M) Extracting static binary from $(IMAGE):$*...)
	@docker create --name $(PACKAGE)-$* $(IMAGE):$*
	@docker cp $(PACKAGE)-$*:/insulatr bin/$(STATIC)
	@docker rm $(PACKAGE)-$*

release-%: changelog-% ; $(info $(M) Releasing milestone $* as $(GIT_TAG))
	@hub release create -F $(GIT_TAG).txt -a bin/$(STATIC) -a bin/$(STATIC).sha256 -a bin/$(STATIC).asc $(GIT_TAG)

push-%: $(SEMVER) ; $(info $(M) Pushing semver tags for image $(IMAGE):$*...)
	@MAJOR=$$($(SEMVER) get major $*) && \
	MINOR=$$($(SEMVER) get minor $*) && \
	docker tag $(IMAGE):$* $(IMAGE):$$MAJOR.$$MINOR && \
	docker tag $(IMAGE):$* $(IMAGE):$$MAJOR && \
	docker push $(IMAGE):$* && \
	docker push $(IMAGE):$$MAJOR.$$MINOR && \
	docker push $(IMAGE):$$MAJOR

latest-%: ; $(info $(M) Pushing latest tag for image $(IMAGE):$*...)
	@docker tag $(IMAGE):$* $(IMAGE):latest && \
	docker push $(IMAGE):latest
