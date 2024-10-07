export PATH := ${HOME}/go/bin:/go/bin:${PATH}

DOCKER := $(or $(DOCKER),podman)
DEPS = npm go
CHECK := $(foreach dep,$(DEPS),\
        $(if $(shell which $(dep)),"$(dep) found",$(error "Missing $(dep) in PATH")))

all: test build

build: builddir clean npm frontend sippy sippy-daemon

verify: lint

builddir:
	mkdir -p sippy-ng/build
	touch sippy-ng/build/index.js

frontend:
	cd sippy-ng; npm run build

sippy: builddir
	go build -mod=vendor ./cmd/sippy/...

sippy-daemon: builddir
	go build -mod=vendor ./cmd/sippy-daemon/...

test: builddir npm
	go test -v ./pkg/...
	LANG=en_US.utf-8 LC_ALL=en_US.utf-8 cd sippy-ng; CI=true npm test -- --coverage --passWithNoTests

lint: builddir npm
	# See https://github.com/facebook/create-react-app/issues/11174 about
	# why we only audit production deps:
	./hack/go-lint.sh run ./...
	cd sippy-ng; npx eslint .
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
