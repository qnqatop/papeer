.PHONY: all windows mac linux clean test dev dev-frontend

# Version stamp: short git sha + dirty marker. Fed into the binary via ldflags
# so the UI footer can show exactly which commit a tester is on.
VERSION ?= $(shell git describe --always --dirty --tags 2>/dev/null || echo dev)
LDFLAGS := -X github.com/qnqatop/papeer/internal/app.Version=$(VERSION)

# Build for every supported platform.
all: windows mac linux

# Cross-compile to Windows (64-bit) from macOS.
windows:
	@echo "==> Building for Windows (amd64) — version $(VERSION)..."
	wails build -platform windows/amd64 -clean -ldflags "$(LDFLAGS)"

# Build for macOS (Universal: Intel + Apple Silicon).
mac:
	@echo "==> Building for macOS (Universal) — version $(VERSION)..."
	wails build -platform darwin/universal -clean -ldflags "$(LDFLAGS)"

# Build for Linux (amd64). Needs libgtk-3-dev + libwebkit2gtk-4.0-dev; on a Mac
# host build inside the Docker image instead (see Dockerfile.linux).
linux:
	@echo "==> Building for Linux (amd64) — version $(VERSION)..."
	wails build -platform linux/amd64 -clean -ldflags "$(LDFLAGS)"

# Launch the app in dev mode with hot reload (Go rebuild + Vite HMR).
# Requires Node ≥22. If nvm is installed, the right version is selected
# automatically via .nvmrc (or run `nvm use 22` first).
# Press Ctrl+C to stop.
dev:
	@echo "==> Starting wails dev — version $(VERSION)..."
	@if [ -s "$$HOME/.nvm/nvm.sh" ]; then \
		. "$$HOME/.nvm/nvm.sh" && nvm use 22 >/dev/null && \
		wails dev -ldflags "$(LDFLAGS)"; \
	else \
		wails dev -ldflags "$(LDFLAGS)"; \
	fi

# Vite-only dev server (no Wails app). Useful when iterating on UI alone
# against mocked wailsjs bindings. Won't have backend RPC available.
dev-frontend:
	@echo "==> Starting Vite dev server (frontend only)..."
	@if [ -s "$$HOME/.nvm/nvm.sh" ]; then \
		. "$$HOME/.nvm/nvm.sh" && nvm use 22 >/dev/null && \
		cd frontend && npm run dev; \
	else \
		cd frontend && npm run dev; \
	fi

test:
	@echo "==> Running Go tests (with race detector)..."
	go test -race ./...
	@echo "==> Running frontend tests (requires Node ≥22)..."
	cd frontend && npm test

clean:
	@echo "==> Cleaning build/bin/..."
	rm -rf build/bin/*
