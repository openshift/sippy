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

.PHONY: verify apm verify-apm verify-migrations
verify: verify-apm verify-migrations

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
	LANG=en_US.utf-8 LC_ALL=en_US.utf-8 cd sippy-ng; CI=true npm test -- --coverage
	@which python3 > /dev/null 2>&1 || { echo "ERROR: python3 not found in PATH"; exit 1; }
	cd mcp && \
		if [ ! -x .venv/bin/python ]; then \
			python3 -m venv .venv && .venv/bin/pip install --upgrade pip -q; \
		fi && \
		.venv/bin/pip install -r requirements.txt -q && \
		.venv/bin/pytest test_server.py -v $(if $(ARTIFACT_DIR),--junitxml=$(ARTIFACT_DIR)/mcp-junit.xml)

lint: builddir npm
	./hack/go-lint.sh run ./...
	cd sippy-ng; npx eslint .
	# See https://github.com/facebook/create-react-app/issues/11174 about
	# why we only audit production deps:
	cd sippy-ng; npm audit --omit=dev --audit-level=high

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

_uvx_env = $(if $(filter true,$(CI)),UV_CACHE_DIR=/tmp/uv-cache UV_TOOL_DIR=/tmp/uv-tools)
apm:
	$(_uvx_env) uvx --from apm-cli@0.13.0 apm install
	$(_uvx_env) uvx --from apm-cli@0.13.0 apm compile

verify-migrations:
	./hack/verify-migrations.sh

verify-apm: apm
	@if [ -n "$$(git status --porcelain -- .claude .cursor .gemini .opencode AGENTS.md CLAUDE.md GEMINI.md sippy-ng/AGENTS.md sippy-ng/CLAUDE.md mcp/AGENTS.md mcp/CLAUDE.md)" ]; then \
		echo "ERROR: Generated APM files are out of date. Run 'make apm' and commit the results."; \
		git status --short -- .claude .cursor .gemini .opencode AGENTS.md CLAUDE.md GEMINI.md sippy-ng/AGENTS.md sippy-ng/CLAUDE.md mcp/AGENTS.md mcp/CLAUDE.md; \
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
