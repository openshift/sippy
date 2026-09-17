#!/usr/bin/env bash
set -euo pipefail

if [ ! -d "${HOME}" ]; then
    mkdir -p "${HOME}"
fi

mkdir -p "${HOME}/.config/containers"

if [ ! -f "${HOME}/.config/containers/registries.conf" ]; then
    cat > "${HOME}/.config/containers/registries.conf" <<'REGISTRIES'
unqualified-search-registries = [
  "registry.access.redhat.com",
  "registry.redhat.io",
  "docker.io"
]
short-name-mode = "permissive"
REGISTRIES
fi

if [ ! -f "${HOME}/.config/containers/storage.conf" ]; then
    if [ -c "/dev/fuse" ] && [ -f "/usr/bin/fuse-overlayfs" ]; then
        cat > "${HOME}/.config/containers/storage.conf" <<'STORAGE'
[storage]
driver = "overlay"
graphroot = "/tmp/graphroot"
[storage.options.overlay]
mount_program = "/usr/bin/fuse-overlayfs"
STORAGE
    else
        cat > "${HOME}/.config/containers/storage.conf" <<'STORAGE'
[storage]
driver = "vfs"
STORAGE
    fi
fi

# Handle OpenShift's random UID (add /etc/passwd entry if unmapped)
if ! whoami &> /dev/null; then
    if [ -w /etc/passwd ]; then
        echo "${USER_NAME:-user}:x:$(id -u):0:${USER_NAME:-user} user:${HOME}:/bin/bash" >> /etc/passwd
        echo "${USER_NAME:-user}:x:$(id -u):" >> /etc/group
    fi
fi

# Configure subuid/subgid for rootless podman user namespaces.
# ci-operator pods with hostUsers:false map only 65536 UIDs (0-65535).
# The default rootless range (100000:65536) falls outside that, so
# newuidmap/newgidmap fail with EPERM. Compute a range that fits.
USER_NAME="$(whoami)"
SUBID_START_DEFAULT=$(( $(id -u) + 1 ))
SUBID_COUNT_DEFAULT=$(( 65536 - SUBID_START_DEFAULT ))
if (( SUBID_COUNT_DEFAULT <= 0 )); then
    SUBID_START_DEFAULT=100000
    SUBID_COUNT_DEFAULT=65536
fi
SUBID_START="${SUBID_START:-$SUBID_START_DEFAULT}"
SUBID_COUNT="${SUBID_COUNT:-$SUBID_COUNT_DEFAULT}"
echo "${USER_NAME}:${SUBID_START}:${SUBID_COUNT}" > /etc/subuid
echo "${USER_NAME}:${SUBID_START}:${SUBID_COUNT}" > /etc/subgid

# Start the podman API socket service. testcontainers-go communicates
# via the Docker HTTP API over a unix socket. Rootless podman does not
# listen on a socket by default.
PODMAN_SOCKET_DIR="${XDG_RUNTIME_DIR:-/tmp}/podman"
mkdir -p "${PODMAN_SOCKET_DIR}"
PODMAN_SOCKET="${PODMAN_SOCKET_DIR}/podman.sock"
podman system service "unix://${PODMAN_SOCKET}" --time=0 &

for _ in $(seq 1 30); do
    if [ -S "${PODMAN_SOCKET}" ]; then
        break
    fi
    sleep 0.1
done

if [ ! -S "${PODMAN_SOCKET}" ]; then
    echo "ERROR: podman socket did not appear at ${PODMAN_SOCKET}" >&2
    exit 1
fi

export DOCKER_HOST="unix://${PODMAN_SOCKET}"
export TESTCONTAINERS_RYUK_DISABLED=true

exec "$@"
