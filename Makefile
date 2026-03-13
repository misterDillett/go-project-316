.PHONY: build test run clean lint deps tidy install-deps

BINARY_NAME=hexlet-go-crawler
CMD_PATH=./cmd/hexlet-go-crawler

deps:
	go mod download
	go mod verify

tidy:
	go mod tidy

install-deps: tidy
	go get github.com/PuerkitoBio/goquery@v1.8.1
	go get golang.org/x/net@v0.20.0
	go mod tidy

build: install-deps
	go build -o $(BINARY_NAME) $(CMD_PATH)

test: install-deps
	go test -v -cover ./...

run: install-deps
ifndef URL
	@echo "Error: URL is required. Usage: make run URL=<url>"
	@exit 1
endif
	go run $(CMD_PATH) $(URL)

clean:
	go clean
	rm -f $(BINARY_NAME)

lint:
	golangci-lint run

install-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.55.2