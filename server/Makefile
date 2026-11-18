.PHONY: all web build run demo test fakedevice darwin docker clean

all: build

# Build the Vue frontend into web/dist (embedded into the Go binary).
web:
	cd web/ui && npm ci && npm run build
	touch web/dist/.gitkeep

# Semantic version from ../VERSION (bumped by hand), plus the local commit.
VERSION := $(shell cat ../VERSION)
BUILD_VERSION := $(VERSION)-dev.g$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X main.version=$(BUILD_VERSION)

build: web
	go build -ldflags "$(LDFLAGS)" -o bin/homepaper ./cmd/homepaper

# Run with Home Assistant (HA_URL/HA_TOKEN from .env or the environment) and
# the login via HA.
run: build
	HOMEPAPER_DATA=data ./bin/homepaper

# Run with demo entities and without a login (development only).
demo: build
	HOMEPAPER_DATA=data HOMEPAPER_AUTH=off HA_URL= HA_TOKEN= ./bin/homepaper

test:
	go vet ./...
	go test ./...
	cd web/ui && npm run typecheck

fakedevice:
	go build -o bin/fakedevice ./cmd/fakedevice

# macOS binary (Apple silicon), e.g. to run the server outside a VM.
darwin: web
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/homepaper-darwin-arm64 ./cmd/homepaper

docker:
	docker build --build-arg VERSION=$(BUILD_VERSION) -t homepaper:$(VERSION) -t homepaper .

clean:
	rm -rf bin web/dist/assets web/dist/index.html
