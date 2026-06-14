BINARY=ec

build:
	go build -o $(BINARY) ./cmd/ec

install: build
	mkdir -p $(HOME)/bin
	install -m 0755 $(BINARY) $(HOME)/bin/$(BINARY)

test:
	go test ./...

vet:
	go vet ./...

.PHONY: build install test vet
