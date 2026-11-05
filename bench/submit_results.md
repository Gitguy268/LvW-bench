# bench/

- `score.py`: rescores an `lvwb` predictions file with the same rules as the Go scorer.
  No model is needed: `python bench/score.py <result>.predictions.jsonl` (add `--data DIR`
  to use local parquet or JSONL files instead of downloading the dataset).

## Submitting a result

1. Run the model with `lvwb run <model>` on the `standard` profile.
2. Open a discussion on this dataset, titled `Result: <model>`. Attach the result `.json`
   and its `.predictions.jsonl`, and say whether the model is general or a specialist.
3. To show the score on the model's own page, open a PR on the model repo that adds the
   output of `lvwb export <result.json> --eval-results` as `.eval_results/lvw-bench.yaml`.
