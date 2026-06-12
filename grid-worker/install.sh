#!/usr/bin/env bash
# Grid Worker Install Script
# Usage: curl -sSfL https://example.com/install.sh | bash
# Or:    ./install.sh [--version v1.0.0] [--install-dir /usr/local/bin]

set -euo pipefail

BINARY="grid-worker"
REPO="grid-computing/grid-worker"
DEFAULT_INSTALL_DIR="/usr/local/bin"
VERSION="${1:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log()  { echo -e "${GREEN}[grid-worker]${NC} $*"; }
warn() { echo -e "${YELLOW}[grid-worker]${NC} $*" >&2; }
err()  { echo -e "${RED}[grid-worker]${NC} ERROR: $*" >&2; exit 1; }

# Detect OS and arch
detect_platform() {
    local os arch

    case "$(uname -s)" in
        Linux*)  os="linux" ;;
        Darwin*) os="darwin" ;;
        *)       err "Unsupported OS: $(uname -s)" ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        arm64|aarch64) arch="arm64" ;;
        *) err "Unsupported architecture: $(uname -m)" ;;
    esac

    echo "${os}_${arch}"
}

# Check for required tools
check_deps() {
    for dep in curl tar; do
        if ! command -v "$dep" &>/dev/null; then
            err "Required tool not found: $dep"
        fi
    done
}

# Get the latest release version from GitHub
get_latest_version() {
    curl -sSfL "https://api.github.com/repos/${REPO}/releases/latest" \
        | grep '"tag_name"' \
        | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/'
}

main() {
    check_deps

    local platform
    platform=$(detect_platform)

    # Resolve version
    if [[ "$VERSION" == "latest" ]]; then
        log "Resolving latest version..."
        VERSION=$(get_latest_version) || err "Failed to get latest version"
    fi

    log "Installing ${BINARY} ${VERSION} for ${platform}..."

    local base_url="https://github.com/${REPO}/releases/download/${VERSION}"
    local archive="${BINARY}_${VERSION#v}_${platform}.tar.gz"
    local tmpdir
    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    # Download archive
    log "Downloading ${archive}..."
    curl -sSfL "${base_url}/${archive}" -o "${tmpdir}/${archive}" \
        || err "Download failed. Check that version ${VERSION} exists."

    # Verify checksum
    local checksums_file="${BINARY}_${VERSION#v}_checksums.txt"
    log "Verifying checksums..."
    curl -sSfL "${base_url}/${checksums_file}" -o "${tmpdir}/${checksums_file}" \
        || warn "Could not download checksums file; skipping verification"

    if [[ -f "${tmpdir}/${checksums_file}" ]]; then
        cd "$tmpdir"
        if command -v sha256sum &>/dev/null; then
            sha256sum --check --ignore-missing "${checksums_file}" \
                || err "Checksum verification failed!"
        elif command -v shasum &>/dev/null; then
            shasum -a 256 --check --ignore-missing "${checksums_file}" \
                || err "Checksum verification failed!"
        fi
        log "Checksum verified ✓"
        cd - >/dev/null
    fi

    # Extract
    tar -xzf "${tmpdir}/${archive}" -C "${tmpdir}" "${BINARY}" \
        || err "Failed to extract archive"

    # Install
    if [[ -w "$INSTALL_DIR" ]]; then
        mv "${tmpdir}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
    else
        log "Installing to ${INSTALL_DIR} (requires sudo)..."
        sudo mv "${tmpdir}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
    fi

    chmod +x "${INSTALL_DIR}/${BINARY}"

    log "Installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"
    log "Version: $("${INSTALL_DIR}/${BINARY}" version)"

    echo ""
    echo -e "${GREEN}Next steps:${NC}"
    echo "  1. Configure: ${BINARY} set-key <your-api-key>"
    echo "  2. Run preflight checks: ${BINARY} preflight"
    echo "  3. Install as system service: sudo ${BINARY} install"
    echo "  4. Check status: ${BINARY} status"
    echo ""
    echo "  Config file: ~/.grid-worker/config.yaml"
    echo "  Documentation: https://github.com/${REPO}"
}

main "$@"
