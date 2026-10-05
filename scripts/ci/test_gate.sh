#!/usr/bin/env bash
# Self-test for check_forbidden.sh.
#
# Why this exists at all: the gate originally passed the whole pattern list to
# grep as positional arguments. Only the first entry was read as the regex,
# the rest became filenames, grep found nothing, and stderr was discarded. The
# gate reported success while enforcing one term out of twenty-five. Nothing
# about that was visible from its output.
#
# A gate that cannot report its own blindness is not a gate. Three checks:
#
#   1. every pattern survives into the matcher           (the exact bug above)
#   2. every literal pattern is caught end to end        (does it scan at all)
#   3. a clean tree passes                              (is it not just noisy)
#
# Check 1 is white-box on purpose. For a pattern like `uakino\.(biz|best|me|tv)`
# there is no single literal string that is guaranteed to match it, so an
# end-to-end probe alone cannot tell "the pattern is absent from the matcher"
# apart from "my probe text was wrong". Reading the matcher does tell them
# apart, and that distinction is the whole point of this file.
#
# Run: bash scripts/ci/test_gate.sh

set -uo pipefail

GATE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATE="$GATE_DIR/check_forbidden.sh"
PATTERN_FILE="$GATE_DIR/forbidden.txt"
PROBE_DIR="$(cd "$GATE_DIR/../.." && pwd)/contracts"

cleanup() { rm -f "$PROBE_DIR"/gate_probe_*.txt; }
trap cleanup EXIT

for f in "$GATE" "$PATTERN_FILE"; do
  [[ -f "$f" ]] || { echo "test_gate: missing $f" >&2; exit 1; }
done

mapfile -t PATTERNS < <(grep -v '^[[:space:]]*#' "$PATTERN_FILE" | grep -v '^[[:space:]]*$')
if [[ ${#PATTERNS[@]} -eq 0 ]]; then
  echo "test_gate: forbidden.txt has no patterns" >&2
  exit 1
fi

fail() { echo "test_gate: FAIL — $*" >&2; exit 1; }

# --- check 1: every pattern reaches the matcher -------------------------
REGEX="$(bash "$GATE" --print-regex)"
missing_regex=()
for p in "${PATTERNS[@]}"; do
  [[ "$REGEX" == *"$p"* ]] || missing_regex+=("$p")
done
if [[ ${#missing_regex[@]} -gt 0 ]]; then
  printf 'test_gate: FAIL — %d pattern(s) never reached the matcher:\n' "${#missing_regex[@]}" >&2
  printf '  %s\n' "${missing_regex[@]}" >&2
  fail "a pattern that is not in the matcher is a pattern that is never enforced"
fi

# --- check 2: literal patterns are caught end to end --------------------
# Only a pattern with no regex metacharacter can be probed with a string that
# is guaranteed to match it. In this list every regex pattern contains a
# backslash (isroot\.in, uakino\.(biz|best|me|tv), ...), so a backslash test is
# both sufficient and unambiguous -- unlike a bracket class of metacharacters,
# which bash does not escape the way it looks like it does.
is_regex_pattern() { [[ "$1" == *'\'* ]]; }

cleanup
probed=0
for i in "${!PATTERNS[@]}"; do
  p="${PATTERNS[$i]}"
  is_regex_pattern "$p" && continue
  printf 'match %s\n' "$p" > "$(printf '%s/gate_probe_%02d.txt' "$PROBE_DIR" "$i")"
  probed=$((probed + 1))
done

if [[ $probed -eq 0 ]]; then
  fail "no literal pattern was available to probe"
fi

# Run from / so a cwd-dependent gate fails here too.
out="$(cd / && bash "$GATE" 2>&1)"
missed=()
for i in "${!PATTERNS[@]}"; do
  is_regex_pattern "${PATTERNS[$i]}" && continue
  probe="$(printf 'gate_probe_%02d.txt' "$i")"
  [[ "$out" == *"$probe"* ]] || missed+=("${PATTERNS[$i]}")
done
if [[ ${#missed[@]} -gt 0 ]]; then
  printf 'test_gate: FAIL — %d literal pattern(s) not caught end to end:\n' "${#missed[@]}" >&2
  printf '  %s\n' "${missed[@]}" >&2
  echo "$out" | head -30 >&2
  fail "the matcher is correct but the scan is not"
fi

# --- check 3: a clean tree passes --------------------------------------
cleanup
out="$(cd / && bash "$GATE" 2>&1)"
[[ "$out" == *"clean"* ]] || { echo "$out" | head -30 >&2; fail "a clean tree did not pass"; }

echo "test_gate: PASS — ${#PATTERNS[@]} patterns reach the matcher, $probed probed end to end, clean tree passes"