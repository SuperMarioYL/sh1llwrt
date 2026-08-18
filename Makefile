BINARY   := sh1llwrt
PKG      := ./cmd/sh1llwrt
VERSION  := $(shell cat VERSION 2>/dev/null | tr -d ' \n')
LDFLAGS  := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build test vet demo install clean

build: ## build the binary into ./bin/sh1llwrt
	@mkdir -p bin
	go build $(LDFLAGS) -o bin/$(BINARY) $(PKG)

test: ## run the test suite
	go test ./...

vet: ## go vet
	go vet ./...

demo: ## render the demo gif via vhs (requires vhs + a tty)
	@mkdir -p assets
	vhs docs/demo.tape

install: build ## install the binary into ~/.local/bin
	@mkdir -p ~/.local/bin
	cp bin/$(BINARY) ~/.local/bin/$(BINARY)

clean:
	rm -rf bin
