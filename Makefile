export PATH := ${HOME}/go/bin:/go/bin:${PATH}

DEPS = npm go
CHECK := $(foreach dep,$(DEPS),\
        $(if $(shell which $(dep)),"$(dep) found",$(error "Missing $(exec) in PATH")))

all: test build

build: clean npm
	# Needed to embed static contents until go 1.17, which can do it natively.
	# This however modifies go.mod, so we need to tidy and vendor after to clean things up.
	go get -u github.com/GeertJohan/go.rice/rice
	go mod tidy
	go mod vendor
	cd sippy-ng; npm run build
	go build -mod=vendor .
	rice append -i . --exec sippy

test: npm
	go test -v ./...
	cd sippy-ng; CI=true npm test -- --coverage

lint: npm
	golangci-lint run ./...
	cd sippy-ng; npx eslint .

npm:
	# For debugging
	npm --version
	# See https://github.com/facebook/create-react-app/issues/11174 about
	# why we only audit production deps:
	cd sippy-ng; [ -d node_modules ] || npm install --no-audit && npm audit --production

clean:
	rm -f sippy
