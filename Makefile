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

test: builddir npm mcp-venv
ifeq ($(ARTIFACT_DIR),)
	@echo "ARTIFACT_DIR is not defined. Using default JUnit file location."
	gotestsum --junitfile ./junit.xml ./pkg/...
else
	@echo "ARTIFACT_DIR is defined. Using $(ARTIFACT_DIR)/junit.xml."
	gotestsum --junitfile $(ARTIFACT_DIR)/junit.xml ./pkg/...
endif
	LANG=en_US.utf-8 LC_ALL=en_US.utf-8 cd sippy-ng; CI=true npm test -- --coverage --passWithNoTests
	cd mcp; .venv/bin/pytest test_server.py -v

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

PYTHON := $(shell command -v python3.12 2>/dev/null || command -v python3 2>/dev/null)

mcp-venv: mcp/.venv/.installed

mcp/.venv/.installed: mcp/requirements-test.txt mcp/requirements.txt
	@test -n "$(PYTHON)" || { echo "ERROR: python3.12 or python3 not found in PATH"; exit 1; }
	@test -x mcp/.venv/bin/python || (rm -rf mcp/.venv && $(PYTHON) -m venv mcp/.venv)
	mcp/.venv/bin/pip install --upgrade pip -q
	mcp/.venv/bin/pip install -r mcp/requirements-test.txt -q
	@touch $@

clean:
	rm -f sippy
	rm -f sippy-daemon
	rm -rf sippy-ng/build
	rm -rf sippy-ng/node_modules

apm:
	uvx --from apm-cli@0.13.0 apm install
	uvx --from apm-cli@0.13.0 apm compile

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

update-variants: sippy
	./sippy variants snapshot --config ./config/openshift.yaml --views ./config/views.yaml
