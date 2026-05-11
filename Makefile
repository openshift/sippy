export PATH := ${HOME}/go/bin:/go/bin:${PATH}

DOCKER := $(or $(DOCKER),podman)
NO_DEP_CHECK_TARGETS := devcontainer-up devcontainer-claude
ifeq ($(filter $(NO_DEP_CHECK_TARGETS),$(MAKECMDGOALS)),)
DEPS = npm go
CHECK := $(foreach dep,$(DEPS),\
        $(if $(shell which $(dep)),"$(dep) found",$(error "Missing $(dep) in PATH")))
endif

GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_TREE_STATE := $(if $(shell git status --porcelain),dirty,clean)
LDFLAGS := -ldflags "-X github.com/openshift/sippy/pkg/version.commitFromGit=$(GIT_COMMIT) -X github.com/openshift/sippy/pkg/version.buildDate=$(BUILD_DATE) -X github.com/openshift/sippy/pkg/version.gitTreeState=$(GIT_TREE_STATE)"

all: test build

build: builddir clean npm frontend sippy sippy-daemon

.PHONY: verify apm verify-apm
verify: lint verify-apm

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
	cd sippy-ng; npm audit --omit=dev

npm: sippy-ng/node_modules/.package-lock.json

sippy-ng/node_modules/.package-lock.json: sippy-ng/package-lock.json
	npm --version
	npm config set fetch-retry-mintimeout 20000
	npm config set fetch-retry-maxtimeout 120000
	cd sippy-ng; npm ci --no-audit --ignore-scripts

clean:
	rm -f sippy
	rm -f sippy-daemon
	rm -rf sippy-ng/build
	rm -rf sippy-ng/node_modules

apm:
	uvx --from apm-cli@0.11.0 apm install
	uvx --from apm-cli@0.11.0 apm compile

verify-apm: apm
	@if ! git diff --quiet HEAD -- .claude .cursor .gemini .opencode AGENTS.md CLAUDE.md GEMINI.md sippy-ng/AGENTS.md sippy-ng/CLAUDE.md mcp/AGENTS.md mcp/CLAUDE.md; then \
		echo "ERROR: Generated APM files are out of date. Run 'make apm' and commit the results."; \
		git diff --stat HEAD -- .claude .cursor .gemini .opencode AGENTS.md CLAUDE.md GEMINI.md sippy-ng/AGENTS.md sippy-ng/CLAUDE.md mcp/AGENTS.md mcp/CLAUDE.md; \
		exit 1; \
	fi

devcontainer-up:
	devcontainer up --workspace-folder . --docker-path podman

devcontainer-claude:
	podman exec -it -w /workspace sippy-dev claude $(CLAUDE_ARGS)

e2e:
	./scripts/e2e.sh

images:
	$(DOCKER) build .

# Requires BigQuery credentials to determine which releases are synthetic (see
# the Synthetic column on the Releases table). Set GOOGLE_APPLICATION_CREDENTIALS
# to a service account key file, e.g.:
#   GOOGLE_APPLICATION_CREDENTIALS=path/to/key.json make update-variants
BIGQUERY_PROJECT ?= openshift-ci-data-analysis
update-variants: sippy
	./sippy variants snapshot --config ./config/openshift.yaml --views ./config/views.yaml --bigquery-project $(BIGQUERY_PROJECT)
