export PATH := ${HOME}/go/bin:/go/bin:${PATH}

DOCKER := $(or $(DOCKER),podman)
DEPS = npm go
CHECK := $(foreach dep,$(DEPS),\
        $(if $(shell which $(dep)),"$(dep) found",$(error "Missing $(dep) in PATH")))

GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_TREE_STATE := $(if $(shell git status --porcelain),dirty,clean)
LDFLAGS := -ldflags "-X github.com/openshift/sippy/pkg/version.commitFromGit=$(GIT_COMMIT) -X github.com/openshift/sippy/pkg/version.buildDate=$(BUILD_DATE) -X github.com/openshift/sippy/pkg/version.gitTreeState=$(GIT_TREE_STATE)"

all: test build

build: builddir clean npm frontend sippy sippy-daemon

verify: lint

builddir:
	mkdir -p sippy-ng/build
	touch sippy-ng/build/index.js

frontend:
	cd sippy-ng; npm run build

sippy: builddir
	go build $(LDFLAGS) -mod=vendor ./cmd/sippy/...

sippy-daemon: builddir
	go build $(LDFLAGS) -mod=vendor ./cmd/sippy-daemon/...

test: builddir npm
ifeq ($(ARTIFACT_DIR),)
	@echo "ARTIFACT_DIR is not defined. Using default JUnit file location."
	gotestsum --junitfile ./junit.xml ./pkg/...
else
	@echo "ARTIFACT_DIR is defined. Using $(ARTIFACT_DIR)/junit.xml."
	gotestsum --junitfile $(ARTIFACT_DIR)/junit.xml ./pkg/...
endif
	LANG=en_US.utf-8 LC_ALL=en_US.utf-8 cd sippy-ng; CI=true npm test -- --coverage --passWithNoTests

lint: builddir npm
	./hack/go-lint.sh run ./...
	cd sippy-ng; npx eslint .
	# See https://github.com/facebook/create-react-app/issues/11174 about
	# why we only audit production deps:
	cd sippy-ng; npm audit --production

npm:
	# For debugging
	npm --version
	npm config set fetch-retry-mintimeout 20000
	npm config set fetch-retry-maxtimeout 120000
	cd sippy-ng; npm install --no-audit

clean:
	rm -f sippy
	rm -f sippy-daemon
	rm -rf sippy-ng/build
	rm -rf sippy-ng/node_modules

e2e:
	./scripts/e2e.sh

images:
	$(DOCKER) build .

update-variants: sippy
	./sippy variants snapshot --config ./config/openshift.yaml
