class LvwBench < Formula
  desc "Local benchmark for tiny System 1 (decision) and System 2 (generative) AI models"
  homepage "https://github.com/Gitguy268/LvW-bench"
  url "https://github.com/Gitguy268/LvW-bench/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "SHA256_SOURCE"
  license "MIT"
  head "https://github.com/Gitguy268/LvW-bench.git", branch: "main"

  depends_on "go" => :build
  depends_on "llama.cpp"

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}", output: bin/"lvwb"), "./cmd/lvwb"
  end

  def caveats
    <<~EOS
      GGUF models run on llama.cpp's llama-server, installed with this formula.
      Safetensors models need uv:
        brew install uv
      Start with:
        lvwb run smollm2-135m --profile quick
      The desktop app is a separate cask: brew install --cask gitguy268/tap/lvw-bench-app
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/lvwb version")
    assert_match "scorer checks passed", shell_output("#{bin}/lvwb selftest")
    assert_match "spot_snake", shell_output("#{bin}/lvwb tasks")
  end
end
