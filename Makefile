all: build test

build:
	go build -mod=vendor .

test:
	go test -v ./...

