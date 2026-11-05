#!/bin/sh
# Build the static HF Space (leaderboard + browser runner) into dist/space.
# Usage: dataset/build_space.sh [lvwb binary]
set -eu
cd "$(dirname "$0")/.."
LVWB=${1:-./.bin/lvwb}
rm -rf dist/space && mkdir -p dist/space
cp -R web/index.html web/run.html web/docs.html web/favicon.svg web/css web/js dist/space/
"$LVWB" webdata dist/space/data results
cat > dist/space/README.md <<'MD'
---
title: LvW-bench leaderboard
emoji: 🐕
colorFrom: red
colorTo: yellow
sdk: static
pinned: false
license: mit
short_description: Local benchmark for tiny System 1 and System 2 models
datasets:
- Lucasgitguy-co/LvW-bench
tags:
- leaderboard
- benchmark
- small-models
---

The LvW-bench leaderboard and browser runner. Data: [Lucasgitguy-co/LvW-bench](https://huggingface.co/datasets/Lucasgitguy-co/LvW-bench).
Code and the `lvwb` runner: [Gitguy268/LvW-bench](https://github.com/Gitguy268/LvW-bench).
MD
echo "built dist/space"
