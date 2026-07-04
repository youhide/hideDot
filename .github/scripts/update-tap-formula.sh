#!/usr/bin/env bash
# Render a Homebrew formula for a prebuilt-binary release and push it to the tap.
#
# Everything is parameterised via environment variables so the same script can
# be reused across projects (e.g. hidedot and hidetop). It reads the per-archive
# checksums produced by GoReleaser (dist/checksums.txt) and installs the
# prebuilt binary — no compilation on the user's machine.
#
# Required env:
#   TAP_TOKEN     GitHub token with write access to the tap repo
#   TAP_REPO      owner/name of the tap        (e.g. youhide/homebrew-youhide)
#   RELEASE_REPO  owner/name of the project    (e.g. youhide/hideDot)
#   PROJECT_NAME  GoReleaser ProjectName / archive prefix (e.g. hideDot)
#   FORMULA_NAME  formula file + binary name   (e.g. hidedot)
#   BINARY_NAME   binary name inside the archive (e.g. hidedot)
#   DESCRIPTION   formula desc
#   HOMEPAGE      formula homepage
#   LICENSE       SPDX license id
#   TAG           release tag (e.g. v0.0.7)
set -euo pipefail

: "${TAP_TOKEN:?}"; : "${TAP_REPO:?}"; : "${RELEASE_REPO:?}"
: "${PROJECT_NAME:?}"; : "${FORMULA_NAME:?}"; : "${BINARY_NAME:?}"
: "${DESCRIPTION:?}"; : "${HOMEPAGE:?}"; : "${LICENSE:?}"; : "${TAG:?}"

version="${TAG#v}"
checksums="dist/checksums.txt"
base_url="https://github.com/${RELEASE_REPO}/releases/download/${TAG}"
class_name="$(printf '%s' "${FORMULA_NAME:0:1}" | tr '[:lower:]' '[:upper:]')${FORMULA_NAME:1}"

darwin_arm="${PROJECT_NAME}_Darwin_arm64.tar.gz"
darwin_amd="${PROJECT_NAME}_Darwin_x86_64.tar.gz"
linux_arm="${PROJECT_NAME}_Linux_arm64.tar.gz"
linux_amd="${PROJECT_NAME}_Linux_x86_64.tar.gz"

sha_for() {
  local file="$1" sha
  sha="$(awk -v f="$file" '$2 == f {print $1}' "$checksums")"
  if [ -z "$sha" ]; then
    echo "::error::checksum not found for $file in $checksums" >&2
    exit 1
  fi
  printf '%s' "$sha"
}

work="$(mktemp -d)"
git clone --depth 1 "https://x-access-token:${TAP_TOKEN}@github.com/${TAP_REPO}.git" "$work"

mkdir -p "$work/Formula"
cat > "$work/Formula/${FORMULA_NAME}.rb" <<EOF
class ${class_name} < Formula
  desc "${DESCRIPTION}"
  homepage "${HOMEPAGE}"
  version "${version}"
  license "${LICENSE}"

  on_macos do
    on_arm do
      url "${base_url}/${darwin_arm}"
      sha256 "$(sha_for "$darwin_arm")"
    end
    on_intel do
      url "${base_url}/${darwin_amd}"
      sha256 "$(sha_for "$darwin_amd")"
    end
  end

  on_linux do
    on_arm do
      url "${base_url}/${linux_arm}"
      sha256 "$(sha_for "$linux_arm")"
    end
    on_intel do
      url "${base_url}/${linux_amd}"
      sha256 "$(sha_for "$linux_amd")"
    end
  end

  def install
    bin.install "${BINARY_NAME}"
  end

  test do
    system "#{bin}/${BINARY_NAME}", "--version"
  end
end
EOF

# Drop a stale cask for this project, if one was ever published.
rm -f "$work/Casks/${FORMULA_NAME}.rb"

cd "$work"
git config user.name "youhide-bot"
git config user.email "youri@youhide.com.br"
git add -A
if git diff --cached --quiet; then
  echo "No formula changes to commit."
else
  git commit -m "${FORMULA_NAME} ${version}"
  git push origin HEAD
fi
