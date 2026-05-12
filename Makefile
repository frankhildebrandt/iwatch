APP := iwatch
MAIN := ./cmd/iwatch
DIST := dist

.PHONY: run build build-linux-arm build-linux-x64 build-mac build-windows test vet install

run:
	go run $(MAIN)

build:
	mkdir -p $(DIST)
	go build -o $(DIST)/$(APP) $(MAIN)

build-linux-arm:
	mkdir -p $(DIST)
	GOOS=linux GOARCH=arm64 go build -o $(DIST)/$(APP)-linux-arm64 $(MAIN)

build-linux-x64:
	mkdir -p $(DIST)
	GOOS=linux GOARCH=amd64 go build -o $(DIST)/$(APP)-linux-amd64 $(MAIN)

build-mac:
	mkdir -p $(DIST)
	GOOS=darwin GOARCH=arm64 go build -o $(DIST)/$(APP)-macos-arm64 $(MAIN)

build-windows:
	mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 go build -o $(DIST)/$(APP)-windows-amd64.exe $(MAIN)

test:
	go test ./...

vet:
	go vet ./...

install: build
	install -m 0755 $(DIST)/$(APP) /usr/local/bin/$(APP)
