#!/bin/sh
set -e

TAR_FILE="/tmp/goreleaser.tar.gz"
RELEASES_URL="https://github.com/goreleaser/goreleaser/releases"
GH_REL_DIR="github-release-notes-linux-amd64-0.2.0"
GH_REL_URL="https://github.com/buchanae/github-release-notes/releases/download/0.2.0/${GH_REL_DIR}.tar.gz"
test -z "$TMPDIR" && TMPDIR="$(mktemp -d)"

last_version() {
  curl -sL -o /dev/null -w %{url_effective} "$RELEASES_URL/latest" |
    rev |
    cut -f1 -d'/'|
    rev
}

download() {
  test -z "$VERSION" && VERSION="$(last_version)"
  test -z "$VERSION" && {
    echo "Unable to get goreleaser version." >&2
    exit 1
  }
  rm -f "$TAR_FILE"
  curl -s -L -o "$TAR_FILE" \
    "$RELEASES_URL/download/$VERSION/goreleaser_$(uname -s)_$(uname -m).tar.gz"

  curl -sSL ${GH_REL_URL} | tar xz && sudo mv ${GH_REL_DIR}/github-release-notes /usr/local/bin/ && rm -rf ${GH_REL_DIR}
}

download
tar -xf "$TAR_FILE" -C "$TMPDIR"
"${TMPDIR}/goreleaser" --release-notes <(github-release-notes -org weaveworks -repo flagger -since-latest-release)
