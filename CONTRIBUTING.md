# Contributing

Thanks for helping. Issues and pull requests are welcome.

## Set up

```bash
go build -o .bin/lvwb ./cmd/lvwb
go test ./...
node web/test/golden.test.mjs     # JS scorer
python3 testdata/make_golden.py   # Python scorer; must not change internal/score/golden.json
```

## Rules for changes

- **Scoring changes** touch three files: `internal/score` (Go), `bench/score.py` and
  `web/js/score.js`. Add a case to `testdata/make_golden.py` and regenerate
  `internal/score/golden.json`.
- **Task changes** (new items, new workflows, other prompts) change the scores. Bump
  `results.Version` and say so in `CHANGELOG.md`. Results from different bench versions are
  never ranked together.
- **Data:** only add sources under MIT, Apache-2.0, CC-BY or CC0. No share-alike or
  unlicensed data. Credit every source in `NOTICE`.
- Keep commits small and focused. CI runs on Linux, macOS and Windows.

## Adding a result

Run `lvwb run <model>` on the `standard` profile. Then open a discussion on the
[dataset](https://huggingface.co/datasets/Lucasgitguy-co/LvW-bench/discussions), or a PR that
adds both the `.json` and the `.predictions.jsonl` to `results/`. Say whether the model was
trained on the same task (`--mode specialist`).
