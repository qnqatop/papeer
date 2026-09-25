#!/usr/bin/env bash
# test-update.sh — End-to-end test for the auto-updater.
#
# Starts a mock GitHub API server that advertises v99.0.0 as the latest
# release, builds papeer with version v0.0.1, and runs it — ready for
# manual clicking through the update flow.
#
# Usage:
#   ./scripts/test-update.sh
#
# Then in the app: Settings → About → Check for Updates.
# The mock will report v99.0.0 as available.
#
# Press Ctrl+C to stop the mock server when done.

set -euo pipefail

MOCK_PORT="${MOCK_PORT:-19999}"
NEW_VERSION="v99.0.0"
CURRENT_VERSION="v0.0.1"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SERVE_DIR="${SERVE_DIR:-/tmp/papeer-update-test}"   # holds the NEW app, outside build/bin
LDPKG="github.com/qnqatop/papeer/internal/app.Version"
KEYPKG="github.com/qnqatop/papeer/internal/updater.PublicKey"

echo "============================================"
echo " Papeer Auto-Updater Test"
echo "============================================"
echo ""
echo "Platform:  $(go env GOOS)/$(go env GOARCH)"
echo "Current:   ${CURRENT_VERSION} (running)"
echo "Latest:    ${NEW_VERSION} (served by mock)"
echo ""

# ── Step 1: Build mock server ──────────────────────────

echo "==> Building mock update server..."
cd "$ROOT"
go build -o /tmp/mock-update-server ./cmd/mock-update-server/

# Throwaway signing key pair: the mock signs SHA256SUMS with the private key
# and the app under test is built with the public key, so the full signature
# path is exercised. The app is built with -tags mockupdate — only such builds
# honour PAPEER_UPDATE_API (release builds always talk to api.github.com).
echo "==> Generating throwaway update signing key..."
eval "$(go run ./cmd/release-sign keygen)"
export MOCK_SIGNING_KEY="${UPDATE_SIGNING_KEY}"

# ── Step 1b: On macOS, build two REAL apps ──────────────
#
# In `make dev` the binary is not inside a .app bundle, so the macOS install
# step (which swaps the .app) cannot run. So we build two real, fully iconned
# bundles:
#   • NEW  (${NEW_VERSION})     → copied to ${SERVE_DIR}, served by the mock
#   • CURR (${CURRENT_VERSION}) → left in build/bin, this is what you run
# The served app lives OUTSIDE build/bin so the updater (which overwrites
# build/bin/Papeer.app) never corrupts the source archive.
GOOS="$(go env GOOS)"
if [ "$GOOS" = "darwin" ]; then
    if [ -s "$HOME/.nvm/nvm.sh" ]; then
        # shellcheck disable=SC1091
        . "$HOME/.nvm/nvm.sh" && nvm use 22 >/dev/null
    fi

    echo "==> Building NEW app ${NEW_VERSION} (served)..."
    wails build -clean -tags mockupdate -ldflags "-X ${LDPKG}=${NEW_VERSION} -X ${KEYPKG}=${UPDATE_PUBLIC_KEY}"
    rm -rf "$SERVE_DIR"
    mkdir -p "$SERVE_DIR"
    cp -R build/bin/Papeer.app "$SERVE_DIR/Papeer.app"

    echo "==> Building CURRENT app ${CURRENT_VERSION} (run)..."
    wails build -clean -tags mockupdate -ldflags "-X ${LDPKG}=${CURRENT_VERSION} -X ${KEYPKG}=${UPDATE_PUBLIC_KEY}"

    export PAPEER_REAL_APP="$SERVE_DIR/Papeer.app"
    echo "==> Mock will serve real app from ${PAPEER_REAL_APP}"
fi

# ── Step 2: Start mock server ──────────────────────────

# Free the port first: a stale mock from a previous run would keep serving its
# OLD payload (e.g. a dummy), our new server would fail to bind, and the app
# would silently talk to the wrong server. Kill any leftover mock + port holder.
echo "==> Freeing port :${MOCK_PORT} (killing any stale mock)..."
pkill -f '/mock-update-server' 2>/dev/null || true
lsof -ti "tcp:${MOCK_PORT}" 2>/dev/null | xargs kill 2>/dev/null || true
sleep 0.5

echo "==> Starting mock server on :${MOCK_PORT}..."
PORT="${MOCK_PORT}" /tmp/mock-update-server &
MOCK_PID=$!
trap "echo ''; echo '==> Stopping mock server (pid ${MOCK_PID})...'; kill ${MOCK_PID} 2>/dev/null; wait ${MOCK_PID} 2>/dev/null; exit 0" INT TERM

# Wait for OUR server to be ready — and bail if it died (e.g. port still busy).
for _ in $(seq 1 30); do
    if ! kill -0 "$MOCK_PID" 2>/dev/null; then
        echo "ERROR: mock server exited (port :${MOCK_PORT} still in use?). Check the log above."
        exit 1
    fi
    if curl -s "http://localhost:${MOCK_PORT}/repos/qnqatop/papeer/releases/latest" > /dev/null 2>&1; then
        break
    fi
    sleep 0.3
done

# Quick validation.
echo -n "==> Checking mock API... "
RELEASE_JSON=$(curl -s "http://localhost:${MOCK_PORT}/repos/qnqatop/papeer/releases/latest")
TAG=$(echo "$RELEASE_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin)['tag_name'])" 2>/dev/null || echo "unknown")
ASSET_SIZE=$(echo "$RELEASE_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin)['assets'][0]['size'])" 2>/dev/null || echo 0)
echo "tag=${TAG} asset=${ASSET_SIZE} bytes"

if [ "$TAG" != "${NEW_VERSION}" ]; then
    echo "ERROR: Expected tag ${NEW_VERSION}, got ${TAG} — a different server is answering on :${MOCK_PORT}."
    kill $MOCK_PID 2>/dev/null
    exit 1
fi

# On macOS the served asset must be the real app (~MBs), not a tiny dummy. A
# small size means a stale/dummy server slipped through — abort loudly.
if [ "$GOOS" = "darwin" ] && [ "${ASSET_SIZE}" -lt 3000000 ]; then
    echo "ERROR: served asset is only ${ASSET_SIZE} bytes — looks like a dummy, not the real app."
    echo "       A stale mock server is likely holding :${MOCK_PORT}. Kill it and retry:"
    echo "         pkill -f mock-update-server"
    kill $MOCK_PID 2>/dev/null
    exit 1
fi

echo ""
echo "============================================"
echo " Mock server is running."
echo ""
echo " TEST FLOW:"
echo "   1. Settings → About → Check for Updates"
echo "   2. Should show: New version ${NEW_VERSION} available"
echo "   3. Click Download Update → watch progress"
echo "   4. Click Restart to Update"
echo ""

export PAPEER_UPDATE_API="http://localhost:${MOCK_PORT}"
cd "$ROOT"

# ── Step 3: Run papeer ─────────────────────────────────

if [ "$GOOS" = "darwin" ]; then
    echo " macOS: launching Papeer.app (${CURRENT_VERSION}). On 'Restart to"
    echo " Update' the .app is swapped for ${NEW_VERSION} and relaunches via"
    echo " LaunchServices (open). After relaunch, Settings → About should show"
    echo " ${NEW_VERSION} — a full, iconned, working app (not a stub)."
    echo ""
    echo " Press Ctrl+C here to stop the mock server when done."
    echo "============================================"
    echo ""
    # Launch the inner binary directly so it inherits PAPEER_UPDATE_API (`open`
    # does not reliably forward env vars to the app). The GUI window still shows.
    PAPEER_UPDATE_API="http://localhost:${MOCK_PORT}" \
        build/bin/Papeer.app/Contents/MacOS/papeer &
    # Keep the mock server in the foreground so Ctrl+C stops it.
    wait "$MOCK_PID"
else
    echo " The dummy binary will print 'Updated!' and exit — that means the"
    echo " install worked. Press Ctrl+C when done."
    echo "============================================"
    echo ""
    make dev VERSION="${CURRENT_VERSION}" WAILS_TAGS=mockupdate UPDATE_PUBLIC_KEY="${UPDATE_PUBLIC_KEY}"
fi
