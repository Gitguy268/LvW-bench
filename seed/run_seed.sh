#!/bin/sh
# Seed leaderboard: the standard profile on every preset, one after another.
# Run from the repo root: seed/run_seed.sh [profile] [lvwb binary]
# Results go to results/, logs to seed/logs/. LVWB_LLAMA_SERVER defaults to the pinned build
# used for the Spot Snake models (llama.cpp 1b6935b); brew's llama.cpp works as well.
set -u
cd "$(dirname "$0")/.." || exit 1
PROFILE=${1:-standard}
LVWB=${2:-./.bin/lvwb}
export LVWB_LLAMA_SERVER=${LVWB_LLAMA_SERVER:-$HOME/.cache/spot-snake/llama.cpp/build/bin/llama-server}
OUT=${OUT:-results}
MODELS=${MODELS:-"smollm2-135m spot-snake-d1-2b spot-snake-d1-3b spot-snake-minimind-jev d1-2b lfm2-350m smollm2-360m qwen3-0.6b minicpm-mermaid minimind-mermaid-run2 lfm2-1.2b lfm3-vl-2b-heretic smollm2-1.7b"}
mkdir -p seed/logs "$OUT"
for m in $MODELS; do
  echo "=== $m $(date '+%H:%M:%S')"
  if "$LVWB" run "$m" --profile "$PROFILE" --out "$OUT" >"seed/logs/$m.log" 2>&1; then
    grep -E "^  (decide|answer|reason)" "seed/logs/$m.log"
  else
    echo "FAILED $m:"; tail -n 5 "seed/logs/$m.log"
  fi
  df -h / | tail -1
done
echo "=== done $(date '+%H:%M:%S')"
