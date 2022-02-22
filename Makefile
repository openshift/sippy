export PATH := ${HOME}/go/bin:/go/bin:${PATH}

DEPS = npm go
CHECK := $(foreach dep,$(DEPS),\
        $(if $(shell which $(dep)),"$(dep) found",$(error "Missing $(exec) in PATH")))

all: test build

build: clean npm
	cd sippy-ng; npm run build
	go build -mod=vendor .

test: npm
	go test -v ./...
	LANG=en_US.utf-8 LC_ALL=en_US.utf-8 cd sippy-ng; CI=true npm test -- --coverage

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
	find sippy-ng/build/ -mindepth 1 -not -name .gitignore -not -name index.html -delete
