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
MILESTONE = $(shell curl -s https://api.github.com/repos/nicholasdille/insulatr/milestones?state=all | jq ".[] | select(.title == \"Version $(GIT_TAG)\").number")
MAJOR_VERSION = $(shell $(SEMVER) get major $(GIT_TAG))
MINOR_VERSION = $(shell $(SEMVER) get minor $(GIT_TAG))

M = $(shell printf "\033[34;1m▶\033[0m")

.DEFAULT_GOAL := $(PACKAGE)

.PHONY: clean deps format linter check static check-docker docker test run

clean: ; $(info $(M) Cleaning...)
	@rm bin/*

##################################################
# TOOLS
##################################################

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

##################################################
# BUILD
##################################################

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

##################################################
# TEST
##################################################

scp-%: binary ; $(info $(M) Copying to $*)
	@tar -cz bin/$(PACKAGE) $(BUILDDEF) | ssh $* tar -xvz

ssh-%: scp-% ; $(info $(M) Running remotely on $*)
	@ssh $* ./bin/$(PACKAGE) --file $(BUILDDEF) $(PARAMS)

##################################################
# PACKAGE
##################################################

check-docker:
	@docker version >/dev/null

docker: $(IMAGE)-master

$(IMAGE)-%: check-docker ; $(info $(M) Building container image $(IMAGE):$*...)
	@docker build --build-arg REF=$* --tag $(IMAGE):$* .

##################################################
# RELEASE
##################################################

extract-%: ; $(info $(M) Extracting static binary from $(IMAGE):$*...)
	@docker create --name $(PACKAGE)-$* $(IMAGE):$*
	@docker cp $(PACKAGE)-$*:/insulatr bin/$(STATIC)
	@docker rm $(PACKAGE)-$*

tag-%: binary $(IMAGE)-% extract-%; $(info $(M) Tagging as $*)
	@hub tag --annotate --sign $* --message "Version $*"
	@hub push origin $*

changelog-%: ; $(info $(M) Creating changelog for $(GIT_TAG) using milestone $*...)
	@( \
	    echo Version $(GIT_TAG); \
	    echo; \
	    hub issue -M $* -s closed -f "[%t](%U)%n" | while read LINE; do echo "- $$LINE"; done; \
	) > $(GIT_TAG).txt

release-%: static; $(info $(M) Uploading release for $(GIT_TAG)...)
	@hub release create -F $(GIT_TAG).txt -a bin/$(STATIC) -a bin/$(STATIC).sha256 -a bin/$(STATIC).asc $(GIT_TAG)

release: changelog-$(MILESTONE) release-$(MILESTONE) ; $(info $(M) Releasing version $(GIT_TAG)...)

push-%: $(SEMVER) ; $(info $(M) Pushing semver tags for image $(IMAGE):$*...)
	@docker tag $(IMAGE):$* $(IMAGE):$(MAJOR_VERSION).$(MINOR_VERSION)
	@docker tag $(IMAGE):$* $(IMAGE):$(MAJOR_VERSION)
	@docker push $(IMAGE):$*
	@docker push $(IMAGE):$(MAJOR_VERSION).$(MINOR_VERSION)
	@docker push $(IMAGE):$(MAJOR_VERSION)

latest-%: ; $(info $(M) Pushing latest tag for image $(IMAGE):$*...)
	@docker tag $(IMAGE):$* $(IMAGE):latest
	@docker push $(IMAGE):latest
