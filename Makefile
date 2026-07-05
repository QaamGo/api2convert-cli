BINARY := api2convert

.PHONY: build test vet fmt lint tidy snapshot clean

build:
	go build -o dist/$(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

lint:
	golangci-lint run

tidy:
	go mod tidy

snapshot:
	goreleaser release --snapshot --clean --skip=publish

clean:
	rm -rf dist
