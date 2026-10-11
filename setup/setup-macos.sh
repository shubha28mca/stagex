#!/usr/bin/env bash
# =============================================================================
# One-shot setup for IIG StageX on macOS.
#
#   chmod +x setup/setup-macos.sh && ./setup/setup-macos.sh
#
# Picks a container runtime based on what the OS can actually run:
#
#   macOS 14+  ->  Docker Desktop (brew cask), the upstream default.
#   macOS 12/13 ->  Colima, a Lima-backed Linux VM that exposes a real Docker
#                   daemon. Current Docker Desktop declares a minimum OS of
#                   macOS 14, so on Monterey/Ventura it installs but can never
#                   launch. Colima + the `docker` CLI + the Compose plugin give
#                   byte-identical `docker compose` behaviour, so
#                   docker-compose.yml is used unchanged.
#
# Either way the result is the same stack on the same ports.
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# Colima VM sizing. Two Go builds plus two Vite production builds run inside the
# VM, so memory is the binding constraint. Override from the environment on a
# roomier machine, e.g. COLIMA_MEMORY=8 ./setup/setup-macos.sh
COLIMA_CPU="${COLIMA_CPU:-2}"
COLIMA_MEMORY="${COLIMA_MEMORY:-4}"
COLIMA_DISK="${COLIMA_DISK:-40}"

echo "== IIG StageX setup (macOS) =="

macos_major="$(sw_vers -productVersion | cut -d. -f1)"
echo "Detected macOS $(sw_vers -productVersion) on $(uname -m)"

# -----------------------------------------------------------------------------
# 1) Homebrew — the macOS package manager everything else comes from.
# -----------------------------------------------------------------------------
if ! command -v brew >/dev/null 2>&1; then
  echo "Installing Homebrew..."
  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
  # Put brew on PATH for the rest of this script (prefix differs by arch).
  for candidate in /opt/homebrew/bin/brew /usr/local/bin/brew; do
    [ -x "$candidate" ] && eval "$("$candidate" shellenv)" && break
  done
fi

# -----------------------------------------------------------------------------
# 2) Container runtime.
# -----------------------------------------------------------------------------
if [ "$macos_major" -ge 14 ]; then
  # --- Docker Desktop path -----------------------------------------------
  echo "macOS $macos_major supports Docker Desktop."
  if ! command -v docker >/dev/null 2>&1; then
    echo "Installing Docker Desktop via Homebrew..."
    brew install --cask docker
  fi
  if ! docker info >/dev/null 2>&1; then
    echo "Starting Docker Desktop..."
    open -a Docker || true
    printf "Waiting for the Docker engine to start"
    for _ in $(seq 1 60); do
      docker info >/dev/null 2>&1 && break
      printf "."; sleep 3
    done
    printf "\n"
  fi
  if ! docker info >/dev/null 2>&1; then
    echo "Docker engine is not running. Open Docker Desktop, wait until it says 'Running', then re-run." >&2
    exit 1
  fi
else
  # --- Colima path (macOS 12 / 13) ---------------------------------------
  echo "macOS $macos_major cannot run current Docker Desktop (it requires macOS 14+)."
  echo "Using Colima as the Docker runtime instead."

  # The `docker` CLI, the Compose plugin, and the Colima VM manager. These are
  # three separate pieces: Docker Desktop normally bundles all of them.
  runtime_formulae=(colima docker docker-compose)

  # Colima needs a hypervisor. Apple's own Virtualization.framework backend
  # ("vz") requires macOS 13+, so on Monterey the VM must run under QEMU and
  # Colima refuses to start without qemu-img on PATH.
  if [ "$macos_major" -lt 13 ]; then
    runtime_formulae+=(qemu)
  fi

  for formula in "${runtime_formulae[@]}"; do
    if ! brew list --formula "$formula" >/dev/null 2>&1; then
      echo "Installing $formula..."
      brew install "$formula"
    fi
  done

  # Compose ships as a docker CLI plugin. Recent docker CLIs already search
  # Homebrew's plugin directory, so check before intervening — on this setup
  # `docker compose` works straight after `brew install`. If it doesn't, link
  # the plugin into the per-user plugin dir, which every version searches.
  if ! docker compose version >/dev/null 2>&1; then
    echo "Registering the Compose plugin with the docker CLI..."
    mkdir -p "$HOME/.docker/cli-plugins"
    ln -sfn "$(brew --prefix)/opt/docker-compose/bin/docker-compose" \
            "$HOME/.docker/cli-plugins/docker-compose"
  fi

  if ! colima status >/dev/null 2>&1; then
    echo "Starting the Colima VM (${COLIMA_CPU} CPU / ${COLIMA_MEMORY} GB RAM / ${COLIMA_DISK} GB disk)..."
    echo "The first start downloads a Linux image — this takes a few minutes."
    colima start --cpu "$COLIMA_CPU" --memory "$COLIMA_MEMORY" --disk "$COLIMA_DISK"
  else
    echo "Colima is already running."
  fi

  if ! docker info >/dev/null 2>&1; then
    echo "The Docker daemon is not reachable. Try: colima delete && ./setup/setup-macos.sh" >&2
    exit 1
  fi
fi

echo "Docker engine is running."
docker compose version

# -----------------------------------------------------------------------------
# 3) Build and start the stack.
#
# On 2-core / 8 GB hardware a single parallel build of all four images can be
# OOM-killed inside the VM, so build them one at a time before starting.
# -----------------------------------------------------------------------------
echo "Building images one at a time (first run compiles Go and Vite — be patient)..."
for service in api web admin-api admin-web; do
  echo "--- building $service ---"
  docker compose build "$service"
done

echo "Starting the stack..."
docker compose up -d

# Wait for both APIs to report healthy before declaring success.
printf "Waiting for the APIs to become healthy"
for _ in $(seq 1 60); do
  if curl -fsS http://localhost:8080/api/health >/dev/null 2>&1 &&
     curl -fsS http://localhost:8081/admin/health >/dev/null 2>&1; then
    printf " ok\n"; break
  fi
  printf "."; sleep 3
done
printf "\n"

cat <<'EOF'

StageX is up:
  Participant app : http://localhost:5173
  Admin console   : http://localhost:5174
  Participant API : http://localhost:8080/api/health
  Admin API       : http://localhost:8081/admin/health

Admin demo logins:
  Operational Admin : ops@stagex.test / Ops@12345
  Event Admin       : event@stagex.test / Event@12345

Stop the stack with :  docker compose down
Release the VM's RAM :  colima stop        (macOS 12/13 only)
EOF
