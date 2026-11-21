#!/usr/bin/env bash
# Installs the pinned Syft release for GitHub Actions Docker jobs.
#
# This script is intentionally Linux/amd64 only because it is used by the
# ubuntu-latest runners. It downloads only the official GitHub release asset,
# verifies the official SHA-256 before extraction, checks the reported version,
# and retries only transient download failures.
set -euo pipefail

readonly SYFT_VERSION="v1.56.0"
readonly SYFT_VERSION_NO_V="${SYFT_VERSION#v}"
readonly SYFT_ASSET="syft_${SYFT_VERSION_NO_V}_linux_amd64.tar.gz"
# Official SHA-256 from syft_1.56.0_checksums.txt for the linux_amd64 archive.
readonly SYFT_SHA256="1c2493cfcf52d3367d6cb25780d9190c111c2219c34c820574c43e8bae15c528"
readonly RELEASE_BASE="https://github.com/anchore/syft/releases/download/${SYFT_VERSION}"
readonly MAX_ATTEMPTS=5
readonly INITIAL_DELAY_SECONDS=2

install_dir="${SYFT_INSTALL_DIR:-${HOME}/.local/bin}"
mkdir -p "${install_dir}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

# download retries transient HTTP/connection failures with bounded exponential
# backoff. Any other failure is permanent and exits immediately.
download() {
  local url="$1"
  local destination="$2"
  local attempt=1
  local delay="${INITIAL_DELAY_SECONDS}"
  local http_code status

  while [ "${attempt}" -le "${MAX_ATTEMPTS}" ]; do
    set +e
    http_code="$(curl \
      --silent \
      --show-error \
      --location \
      --connect-timeout 10 \
      --max-time 180 \
      --output "${destination}" \
      --write-out '%{http_code}' \
      "${url}")"
    status=$?
    set -e

    if [ "${status}" -eq 0 ] && [ "${http_code}" = "200" ]; then
      echo "Downloaded ${url} on attempt ${attempt}/${MAX_ATTEMPTS}."
      return 0
    fi

    case "${status}:${http_code}" in
      0:502|0:503|0:504|6:*|7:*|18:*|28:*|35:*|52:*|56:*)
        ;;
      *)
        echo "::error::Permanent download failure for ${url} (curl ${status}, HTTP ${http_code})."
        return 1
        ;;
    esac

    if [ "${attempt}" -eq "${MAX_ATTEMPTS}" ]; then
      echo "::error::Transient download failure persisted after ${MAX_ATTEMPTS} attempts for ${url} (curl ${status}, HTTP ${http_code})."
      return 1
    fi

    echo "Transient download failure for ${url} (curl ${status}, HTTP ${http_code}); retrying in ${delay}s."
    sleep "${delay}"
    attempt=$((attempt + 1))
    delay=$((delay * 2))
  done
}

verify_sha256() {
  local file="$1"
  local line="${SYFT_SHA256}  ${file}"

  if command -v sha256sum >/dev/null 2>&1 && sha256sum --help 2>&1 | grep -q -- '--check'; then
    printf '%s\n' "${line}" | sha256sum --check --strict
  elif command -v shasum >/dev/null 2>&1; then
    printf '%s\n' "${line}" | shasum -a 256 -c
  else
    echo "::error::No SHA-256 verification tool is available; refusing to install Syft."
    return 1
  fi
}

archive="${tmp_dir}/${SYFT_ASSET}"
download "${RELEASE_BASE}/${SYFT_ASSET}" "${archive}"
verify_sha256 "${archive}"

tar --extract --gzip --file "${archive}" --directory "${tmp_dir}" syft

actual_version="$("${tmp_dir}/syft" version | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+' | head -n 1 || true)"
if [ "${actual_version}" != "${SYFT_VERSION_NO_V}" ]; then
  echo "::error::Unexpected Syft version: expected ${SYFT_VERSION_NO_V}, got '${actual_version:-unknown}'."
  exit 1
fi

install -m 0755 "${tmp_dir}/syft" "${install_dir}/syft"
"${install_dir}/syft" version >/dev/null

if [ -n "${GITHUB_PATH:-}" ]; then
  echo "${install_dir}" >> "${GITHUB_PATH}"
fi

echo "Installed Syft ${SYFT_VERSION} at ${install_dir}/syft."
