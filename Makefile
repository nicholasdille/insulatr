OWNER    = nicholasdille
PACKAGE  = insulatr
IMAGE    = $(OWNER)/$(PACKAGE)
STATIC   = insulatr-$(shell uname -m)
SOURCE   = $(shell echo *.go)
PWD      = $(shell pwd)
BIN      = $(PWD)/bin
TOOLS    = $(PWD)/tools
GOMOD    = $(PWD)/go.mod
GOFMT    = gofmt
SEMVER   = $(TOOLS)/semver
BUILDDEF = insulatr.yaml

GIT_COMMIT = $(shell git rev-list -1 HEAD)
BUILD_TIME = $(shell date +%Y%m%d-%H%M%S)
GIT_TAG = $(shell git describe --tags 2>/dev/null)

M = $(shell printf "\033[34;1m▶\033[0m")

.DEFAULT_GOAL := $(PACKAGE)

.PHONY: clean prepare deps deppatch depupdate deptidy format linter check static binary check-docker docker test run check-changes $(PACKAGE) bump-% build-% release-% tag-% changelog changelog-% release $(IMAGE)-% check-tag push-% latest-%

.SECONDARY:

clean: clean-docker; $(info $(M) Cleaning...)
	@rm -rf $(BIN)
	@rm -rf $(TOOLS)

prepare: | $(BIN) $(TOOLS) $(SEMVER)

$(BIN): ; $(info $(M) Preparing binary...)
	@mkdir -p $(BIN)

$(TOOLS): ; $(info $(M) Preparing tools...)
	@mkdir -p $(TOOLS)

##################################################
# TOOLS
##################################################

deps: $(GOMOD)

deppatch: ; $(info $(M) Updating dependencies to the latest patch...)
	@go get -u=patch

depupdate: ; $(info $(M) Updating dependencies to the latest version...)
	@go get -u

deptidy: ; $(info $(M) Updating dependencies to the latest version...)
	@go mod tidy

$(GOMOD): ; $(info $(M) Initializing dependencies...)
	@test -f go.mod || go mod init

format: ; $(info $(M) Running formatter...)
	@gofmt -l -w $(SOURCE)

# go get github.com/golang/lint/golint
lint: ; $(info $(M) Running linter...)
	@golint $(PACKAGE)

# go get github.com/KyleBanks/depth/cmd/depth
deptree: ; $(info $(M) Creating dependency tree...)
	@depth .

semver: $(SEMVER)

$(SEMVER): $(TOOLS) ; $(info $(M) Installing semver...)
	@test -f $@ && test -x $@ || ( \
		curl -sLf https://github.com/fsaintjacques/semver-tool/raw/2.1.0/src/semver > $@; \
		chmod +x $@; \
	)

##################################################
# BUILD
##################################################

check: format lint

%.sha256: % ; $(info $(M) Creating SHA256 for $*...)
	@echo sha256sum $* > $@

%.asc: % ; $(info $(M) Creating signature for $*...)
	@gpg --local-user $$(git config --get user.signingKey) --sign --armor --detach-sig --yes $*

binary $(PACKAGE): $(BIN)/$(PACKAGE)

$(BIN)/$(PACKAGE): $(SOURCE) | prepare ; $(info $(M) Building $(PACKAGE)...)
	@go build -ldflags "-s -w -X main.GitCommit=$(GIT_COMMIT) -X main.BuildTime=$(BUILD_TIME) -X main.Version=$(GIT_TAG)" -o $@ $(SOURCE)

static: $(BIN)/$(STATIC)

$(BIN)/$(STATIC): $(SOURCE) | prepare ; $(info $(M) Building static $(PACKAGE)...)
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags "-s -w -X main.GitCommit=$(GIT_COMMIT) -X main.BuildTime=$(BUILD_TIME) -X main.Version=$(GIT_TAG)" -o $@ $(SOURCE)

##################################################
# TEST
##################################################

scp-%: $(BIN)/$(PACKAGE) ; $(info $(M) Copying to $*)
	@tar -cz bin/$(PACKAGE) $(BUILDDEF) | ssh $* tar -xvz

ssh-%: scp-% ; $(info $(M) Running remotely on $*)
	@ssh $* ./bin/$(PACKAGE) --file $(BUILDDEF) $(PARAMS)

##################################################
# PACKAGE
##################################################

$(IMAGE)-% push-%: MAJOR_VERSION = $(shell $(SEMVER) get major $(GIT_TAG))
$(IMAGE)-% push-%: MINOR_VERSION = $(shell $(SEMVER) get minor $(GIT_TAG))

check-docker: ; $(info $(M) Checking for Docker...)
	@docker version >/dev/null

clean-docker: ; $(info $(M) Removing Docker images called $(IMAGE)...)
	@docker image ls -q $(IMAGE) | uniq | xargs -r docker image rm -f
	@rm -rf $(PWD)/.docker

$(PWD)/.docker/$(IMAGE)/%.image: | check-docker ; $(info $(M) Building container image $(IMAGE):$*...)
	@mkdir -p $(PWD)/.docker/$(IMAGE)
	@if docker image ls $(IMAGE):$* | grep --invert-match --quiet "$(IMAGE):$*"; then \
		docker build --tag $(IMAGE):$* .; \
	fi
	@touch .docker/$(IMAGE)/$*.image

$(IMAGE)-%: | $(BIN)/$(STATIC) $(BIN)/$(STATIC).asc $(BIN)/$(STATIC).sha256 $(PWD)/.docker/$(IMAGE)/%.image ; $(info $(M) Tagging container image $(IMAGE):$*...)
	@if test "$*" != "master"; then \
		docker tag $(IMAGE):$* $(IMAGE):$(MAJOR_VERSION).$(MINOR_VERSION); \
		docker tag $(IMAGE):$* $(IMAGE):$(MAJOR_VERSION); \
	fi

docker: $(IMAGE)-master

##################################################
# RELEASE
##################################################

check-changes: ; $(info $(M) Checking for uncommitted changes...)
	@if test "$$(git status --short)"; then \
		git status --short; \
		false; \
	fi

bump-%: ; $(info $(M) Bumping $* for version $(GIT_TAG)...)
	@$(SEMVER) bump $* $(GIT_TAG)

check-tag: ; $(info $(M) Checking for untagged commits in $(GIT_TAG)...)
	@if ! $(SEMVER) get prerel $(GIT_TAG) | grep -v --quiet "^[0-9]*-g[0-9a-f]*$$"; then \
		PAGER= git log --oneline -n $$($(SEMVER) get prerel $(GIT_TAG) | cut -d- -f1); \
		false; \
	fi

tag-%: | check-changes ; $(info $(M) Tagging as $*...)
	@git tag | grep -q "$(GIT_TAG)" || git tag --annotate --sign $* --message "Version $*"
	@git push origin $*

changelog: MILESTONE = $(shell curl -s https://api.github.com/repos/$(IMAGE)/milestones?state=all | jq ".[] | select(.title == \"Version $(GIT_TAG)\").number")
changelog: changelog-$(MILESTONE)

changelog-%: ; $(info $(M) Creating changelog for $(GIT_TAG) using milestone $*...)
	@( \
	    echo Version $(GIT_TAG); \
	    echo; \
	    hub issue -M $* -s closed -f "[%t](%U)%n" | while read LINE; do echo "- $$LINE"; done; \
	) > $(GIT_TAG).txt

release-%: check-changes check-tag tag-% $(IMAGE)-% push-%; $(info $(M) Uploading release for $(GIT_TAG)...)
	@hub release create -F $(GIT_TAG).txt -a bin/$(STATIC) -a bin/$(STATIC).sha256 -a bin/$(STATIC).asc $(GIT_TAG)

release: changelog release-$(GIT_TAG) ; $(info $(M) Releasing version $(GIT_TAG)...)

push-%: ; $(info $(M) Pushing semver tags for image $(IMAGE):$*...)
	@docker push $(IMAGE):$*
	@docker push $(IMAGE):$(MAJOR_VERSION).$(MINOR_VERSION)
	@docker push $(IMAGE):$(MAJOR_VERSION)

latest-%: ; $(info $(M) Pushing latest tag for image $(IMAGE):$*...)
	@docker tag $(IMAGE):$* $(IMAGE):latest
	@docker push $(IMAGE):latest
