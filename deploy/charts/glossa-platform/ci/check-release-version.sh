#!/usr/bin/env bash
# Checks that the places a release version lives agree:
#   - .kiln.yaml: every `VERSION` build arg (the images' baked-in version)
#   - Chart.yaml: appVersion (the chart's default image tag is v<appVersion>)
#   - the release tag, when given (vX.Y.Z or X.Y.Z)
#
#   ci/check-release-version.sh            # .kiln.yaml vs Chart.yaml
#   ci/check-release-version.sh v0.5.2     # ... and the tag about to be pushed
#
# Runs in CI (platform.yml's chart job) and belongs before tagging a release.
set -euo pipefail

root="$(cd "$(dirname "$0")/../../../.." && pwd)"
kiln="$root/.kiln.yaml"
chart="$root/deploy/charts/glossa-platform/Chart.yaml"

app=$(sed -nE 's/^appVersion:[[:space:]]*"?([^"[:space:]]+)"?.*/\1/p' "$chart" | head -1)
[ -n "$app" ] || { echo "no appVersion in $chart" >&2; exit 1; }

versions=$(sed -nE 's/.*VERSION:[[:space:]]*"?([^"},[:space:]]+)"?.*/\1/p' "$kiln" | sort -u)
[ -n "$versions" ] || { echo "no VERSION build arg in $kiln" >&2; exit 1; }

status=0
if [ "$(printf '%s\n' "$versions" | wc -l | tr -d ' ')" != 1 ]; then
  echo "error: .kiln.yaml carries different VERSION args: $(echo $versions)" >&2
  status=1
fi
kiln_version=$(printf '%s\n' "$versions" | head -1)
if [ "$kiln_version" != "$app" ]; then
  echo "error: .kiln.yaml VERSION is '$kiln_version' but Chart.yaml appVersion is '$app'; bump both in one commit" >&2
  status=1
fi
if [ $# -ge 1 ]; then
  tag_version="${1#v}"
  if [ "$tag_version" != "$app" ]; then
    echo "error: release tag '$1' does not match appVersion '$app'; bump the chart and .kiln.yaml first, then tag" >&2
    status=1
  fi
fi
if [ "$status" -eq 0 ]; then
  echo "release version $app: .kiln.yaml, Chart.yaml${1:+ and tag $1} agree"
fi
exit "$status"
