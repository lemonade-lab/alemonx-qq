SHELL := /bin/bash

BINARIES := \
	dist/alemonx-qq-linux-amd64 \
	dist/alemonx-qq-windows-amd64.exe \
	dist/alemonx-qq-darwin-arm64 \
	dist/alemonx-qq-darwin-amd64

.PHONY: test vet validate web build dist check

test:
	go test ./runner/...

vet:
	go vet ./runner/...

validate:
	python3 scripts/validate-alx.py alx.json

# Build the plugin web UI (React + Tailwind, alx design tokens) into ../web.
web:
	cd frontend && yarn install --non-interactive && yarn build

dev-fe:
	cd frontend && yarn dev

build-fe:
	cd frontend && yarn build

build: $(BINARIES)

dist/alemonx-qq-linux-amd64:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o $@ ./runner

dist/alemonx-qq-windows-amd64.exe:
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o $@ ./runner

dist/alemonx-qq-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o $@ ./runner

dist/alemonx-qq-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o $@ ./runner

# Package each platform as a zip containing the full plugin directory
# (alx.json + dist/ + web/), matching the CI release artifact layout.
dist: build web
	mkdir -p release
	for target in linux-amd64 windows-amd64 darwin-arm64 darwin-amd64; do \
		case "$$target" in \
			windows-amd64) binary="dist/alemonx-qq-windows-amd64.exe" ;; \
			*) binary="dist/alemonx-qq-$$target" ;; \
		esac; \
		stage="release/alemonx-qq-$$target/alemonx-qq"; \
		rm -f "release/alemonx-qq-$$target.zip"; \
		mkdir -p "$$stage/dist"; \
		cp alx.json "$$stage/alx.json"; \
		cp -r web "$$stage/web"; \
		cp "$$binary" "$$stage/dist/"; \
		(cd "release/alemonx-qq-$$target" && zip -qr "../alemonx-qq-$$target.zip" alemonx-qq); \
		rm -rf "release/alemonx-qq-$$target"; \
	done
	@ls -la release/

check: test vet validate
