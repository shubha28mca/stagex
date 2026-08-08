#!/usr/bin/env bash
# One-shot setup for IIG StageX on Linux: installs Docker Engine + the Compose
# plugin (official get.docker.com) if missing, then builds and starts the stack.
#
#   chmod +x setup/setup-linux.sh && ./setup/setup-linux.sh
#
# Note: installing Docker and starting its service needs sudo.

set -euo pipefail

# Move to the repository root (this script lives in <repo>/setup).
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

echo "== IIG StageX setup (Linux) =="

# 1) Docker Engine + Compose plugin via the official convenience script.
if ! command -v docker >/dev/null 2>&1; then
  echo "Installing Docker Engine (get.docker.com)..."
  curl -fsSL https://get.docker.com | sudo sh
  sudo usermod -aG docker "$USER" || true
  echo "Added '$USER' to the 'docker' group (takes effect on next login)."
fi

# 2) Ensure the Docker service is enabled and running.
sudo systemctl enable --now docker 2>/dev/null || true

# 3) Confirm the Compose plugin is present.
DOCKER="docker"
docker info >/dev/null 2>&1 || DOCKER="sudo docker"   # group change may not be active in this shell yet
if ! $DOCKER compose version >/dev/null 2>&1; then
  echo "Docker Compose plugin is missing. Install 'docker-compose-plugin' for your distro, then re-run." >&2
  exit 1
fi
echo "Docker is ready."

# 4) Build and start the whole stack.
echo "Building and starting containers (first run downloads images, be patient)..."
$DOCKER compose up --build -d

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
(If 'docker' needs sudo, log out/in once so the 'docker' group applies.)
EOF
