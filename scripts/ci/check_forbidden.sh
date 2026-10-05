#!/usr/bin/env bash
# Kadrbox: gate on forbidden terms.
#
# Fails the build when a pirate source name, CDN or hostname appears in
# shipped code. Both app stores read the code and the screenshots, so a name
# in a constant, a test fixture or a doc comment is enough to get the app
# rejected — it must never get there in the first place.
#
# Why a gate and not a review habit: the names keep coming back. They were in
# constants, in Drift migrations, in DAO tests and in the README at the same
# time, and each removal was a separate manual sweep that the next person undid.
# A test is the only thing that survives the next person.
#
# Scoped to SHIPPED code. plugins/ is excluded while it is being exported out
# of this repository; it must be deleted, not cleaned.

set -uo pipefail

PATTERN_FILE="scripts/ci/forbidden.txt"
SCAN_PATHS=(frontend/lib frontend/test backend contracts .github scripts/ci)

if [[ ! -f "$PATTERN_FILE" ]]; then
  echo "gate: $PATTERN_FILE is missing — refusing to pass silently" >&2
  exit 1
fi

mapfile -t PATTERNS < <(grep -v '^[[:space:]]*#' "$PATTERN_FILE" | grep -v '^[[:space:]]*$')
if [[ ${#PATTERNS[@]} -eq 0 ]]; then
  echo "gate: $PATTERN_FILE has no patterns — refusing to pass silently" >&2
  exit 1
fi

failed=0

for path in "${SCAN_PATHS[@]}"; do
  [[ -e "$path" ]] || continue
  # Exclude this script and the pattern list itself, and any build output.
  hits=$(grep -rInE -i \
    --exclude-dir=build \
    --exclude-dir=.dart_tool \
    --exclude-dir=ephemeral \
    --exclude-dir=ci \
    "${PATTERNS[@]}" "$path" 2>/dev/null || true)

  if [[ -n "$hits" ]]; then
    echo "gate: forbidden terms under $path" >&2
    echo "$hits" | cut -c1-200 >&2
    echo >&2
    failed=1
  fi
done

if [[ $failed -ne 0 ]]; then
  cat >&2 <<'MSG'
gate: the app must not name pirate sources anywhere in shipped code.

If this is a leftover from the migration, delete it. If the name is genuinely
required, it belongs in the separate community-server repository, not here —
this repository ships an app that connects to a catalog URL the user supplies.
MSG
  exit 1
fi

echo "gate: clean (${#PATTERNS[@]} patterns, ${#SCAN_PATHS[@]} paths)"