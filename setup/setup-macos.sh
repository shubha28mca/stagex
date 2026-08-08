#!/usr/bin/env bash
# One-shot setup for IIG StageX on macOS: installs Homebrew + Docker Desktop if
# missing, waits for the Docker engine, then builds and starts the stack.
#
#   chmod +x setup/setup-macos.sh && ./setup/setup-macos.sh

set -euo pipefail

# Move to the repository root (this script lives in <repo>/setup).
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

echo "== IIG StageX setup (macOS) =="

# 1) Homebrew (the standard macOS package manager).
if ! command -v brew >/dev/null 2>&1; then
  echo "Installing Homebrew..."
  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
  # Make brew available on Apple Silicon in this session.
  [ -x /opt/homebrew/bin/brew ] && eval "$(/opt/homebrew/bin/brew shellenv)"
fi

# 2) Docker Desktop.
if ! command -v docker >/dev/null 2>&1; then
  echo "Installing Docker Desktop via Homebrew..."
  brew install --cask docker
fi

# 3) Start Docker and wait for the engine.
if ! docker info >/dev/null 2>&1; then
  echo "Starting Docker Desktop..."
  open -a Docker || true
  printf "Waiting for the Docker engine to start"
  for _ in $(seq 1 60); do
    if docker info >/dev/null 2>&1; then break; fi
    printf "."; sleep 3
  done
  printf "\n"
fi
if ! docker info >/dev/null 2>&1; then
  echo "Docker engine is not running. Open Docker Desktop, wait until it says 'Running', then re-run." >&2
  exit 1
fi
echo "Docker engine is running."

# 4) Build and start the whole stack.
echo "Building and starting containers (first run downloads images, be patient)..."
docker compose up --build -d

cat <<'EOF'

StageX is up:
  Participant app : http://localhost:5173
  Admin console   : http://localhost:5174
  Participant API : http://localhost:8080/api/health
  Admin API       : http://localhost:8081/admin/health

Admin demo logins:
  Operational Admin : ops@stagex.test / Ops@12345
  Event Admin       : event@stagex.test / Event@12345

Stop it later with:  docker compose down
EOF
