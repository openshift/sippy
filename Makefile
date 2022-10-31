export PATH := ${HOME}/go/bin:/go/bin:${PATH}

DEPS = npm go
CHECK := $(foreach dep,$(DEPS),\
        $(if $(shell which $(dep)),"$(dep) found",$(error "Missing $(exec) in PATH")))

all: test build

builddir:
	mkdir -p sippy-ng/build
	touch sippy-ng/build/index.html

build: builddir clean npm
	cd sippy-ng; npm run build
	go build -mod=vendor .

test: builddir npm
	go test -v ./pkg/...
	LANG=en_US.utf-8 LC_ALL=en_US.utf-8 cd sippy-ng; CI=true npm test -- --coverage

lint: builddir npm
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
	rm -rf sippy-ng/build

e2e:
	./scripts/e2e.sh
