# Security

Please report vulnerabilities privately through GitHub's
[private vulnerability reporting](https://github.com/Gitguy268/LvW-bench/security/advisories/new),
not in a public issue.

Things worth knowing:
- `lvwb` starts model servers on `127.0.0.1` only.
- Custom model code from a Hugging Face repo runs only with `--trust-remote-code`.
- `HF_TOKEN` is only sent to the Hugging Face Hub (or `HF_ENDPOINT`).
