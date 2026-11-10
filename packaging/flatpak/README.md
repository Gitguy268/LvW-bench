# Flatpak

A self-hosted Flatpak of the LvW-bench desktop app. It isn't on Flathub. Releases carry an
`io.github.gitguy268.lvw_bench.flatpak` file, built by `.github/workflows/flatpak.yml`.

```sh
flatpak remote-add --user --if-not-exists flathub https://dl.flathub.org/repo/flathub.flatpakrepo
flatpak install --user ./io.github.gitguy268.lvw_bench.flatpak
flatpak run io.github.gitguy268.lvw_bench
```

It includes llama.cpp's `llama-server` (CPU build) for GGUF models. Safetensors models need
uv, which the Flatpak doesn't have; use the `.deb` or the `lvwb` command for those.

Build it yourself on Linux (x86_64):

```sh
gh release download v0.1.0 -R Gitguy268/LvW-bench -p 'LvW-bench-*-linux-amd64.tar.gz' -O packaging/flatpak/lvw-bench-linux.tar.gz
cd packaging/flatpak
flatpak-builder --user --install-deps-from=flathub --force-clean --repo=repo build io.github.gitguy268.lvw_bench.yml
flatpak build-bundle --runtime-repo=https://dl.flathub.org/repo/flathub.flatpakrepo repo lvw-bench.flatpak io.github.gitguy268.lvw_bench
```
