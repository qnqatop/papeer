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
DUMMY_VERSION="v99.0.0"
CURRENT_VERSION="v0.0.1"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "============================================"
echo " Papeer Auto-Updater Test"
echo "============================================"
echo ""
echo "Platform:  $(go env GOOS)/$(go env GOARCH)"
echo "Current:   ${CURRENT_VERSION}"
echo "Latest:    ${DUMMY_VERSION} (mock)"
echo ""

# ── Step 1: Build mock server ──────────────────────────

echo "==> Building mock update server..."
cd "$ROOT"
go build -o /tmp/mock-update-server ./cmd/mock-update-server/

# ── Step 2: Start mock server ──────────────────────────

echo "==> Starting mock server on :${MOCK_PORT}..."
/tmp/mock-update-server &
MOCK_PID=$!
trap "echo ''; echo '==> Stopping mock server (pid ${MOCK_PID})...'; kill ${MOCK_PID} 2>/dev/null; wait ${MOCK_PID} 2>/dev/null; exit 0" INT TERM

# Wait for server to be ready.
for i in $(seq 1 30); do
    if curl -s "http://localhost:${MOCK_PORT}/repos/qnqatop/papeer/releases/latest" > /dev/null 2>&1; then
        break
    fi
    sleep 0.3
done

# Quick validation.
echo -n "==> Checking mock API... "
RELEASE_JSON=$(curl -s "http://localhost:${MOCK_PORT}/repos/qnqatop/papeer/releases/latest")
TAG=$(echo "$RELEASE_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin)['tag_name'])" 2>/dev/null || echo "unknown")
echo "tag=${TAG}"

if [ "$TAG" != "${DUMMY_VERSION}" ]; then
    echo "ERROR: Expected tag ${DUMMY_VERSION}, got ${TAG}"
    kill $MOCK_PID 2>/dev/null
    exit 1
fi

echo ""
echo "============================================"
echo " Mock server is running."
echo ""
echo " Now starting papeer in dev mode..."
echo ""
echo " TEST FLOW:"
echo "   1. Settings → About → Check for Updates"
echo "   2. Should show: New version ${DUMMY_VERSION} available"
echo "   3. Click Download Update → watch progress"
echo "   4. Click Restart to Update"
echo ""
echo " The dummy binary will print 'Updated!' and exit."
echo " That's expected — it means the install worked."
echo ""
echo " Press Ctrl+C when done."
echo "============================================"
echo ""

# ── Step 3: Run papeer ─────────────────────────────────

export PAPEER_UPDATE_API="http://localhost:${MOCK_PORT}"
cd "$ROOT"
make dev
