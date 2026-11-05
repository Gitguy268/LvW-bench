package resolve

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Gitguy268/LvW-bench/internal/hub"
)

func TestChoose(t *testing.T) {
	d1 := []hub.File{{Path: "config.json"}, {Path: "model-00001-of-00004.safetensors"}, {Path: "gguf/mmproj-f16.gguf", Size: 3},
		{Path: "gguf/model-f16.gguf", Size: 30}, {Path: "gguf/model-Q4_K_M.gguf", Size: 10}}
	s, err := choose(d1, Options{})
	if err != nil || s.Kind != KindGGUF || s.File != "gguf/model-Q4_K_M.gguf" {
		t.Fatalf("default gguf: %+v %v", s, err)
	}
	s, _ = choose(d1, Options{Quant: "f16"})
	if s.File != "gguf/model-f16.gguf" {
		t.Fatalf("quant f16: %+v", s)
	}
	s, _ = choose(d1, Options{Format: KindSafetensors})
	if s.Kind != KindSafetensors {
		t.Fatalf("forced safetensors: %+v", s)
	}
	hip := []hub.File{{Path: "M-Q4_K_M-hip-optimized.gguf"}, {Path: "M-Q4_K_M.gguf"}}
	if s, _ = choose(hip, Options{}); s.File != "M-Q4_K_M.gguf" {
		t.Fatalf("plain quant: %+v", s)
	}
	split := []hub.File{{Path: "m-Q8_0-00001-of-00002.gguf", Size: 5}, {Path: "m-Q8_0-00002-of-00002.gguf", Size: 5}}
	s, _ = choose(split, Options{})
	if len(s.Parts) != 2 || s.SizeBytes != 10 {
		t.Fatalf("split gguf: %+v", s)
	}
	epochs := []hub.File{{Path: "README.md"}, {Path: "ep1/model.safetensors"}, {Path: "ep1/config.json"}, {Path: "ep3/model.safetensors"}, {Path: "ep3/config.json"}}
	if _, err := choose(epochs, Options{}); err == nil {
		t.Fatal("subfolder-only weights should ask for --subfolder")
	}
	s, err = choose(epochs, Options{Subfolder: "ep3"})
	if err != nil || s.Kind != KindSafetensors || s.Subfolder != "ep3" {
		t.Fatalf("subfolder: %+v %v", s, err)
	}
	adapter := []hub.File{{Path: "adapter_config.json"}, {Path: "adapter_model.safetensors"}}
	s, _ = choose(adapter, Options{})
	if !s.Adapter {
		t.Fatalf("adapter: %+v", s)
	}
}

func TestResolveForms(t *testing.T) {
	dir := t.TempDir()
	g := filepath.Join(dir, "tiny-Q4_K_M.gguf")
	os.WriteFile(g, []byte("GGUF"), 0o644)
	st := filepath.Join(dir, "st")
	os.Mkdir(st, 0o755)
	os.WriteFile(filepath.Join(st, "config.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(st, "model.safetensors"), []byte("x"), 0o644)
	ctx := context.Background()
	if s, err := Resolve(ctx, g, Options{}); err != nil || s.Kind != KindGGUF || s.Quant != "Q4_K_M" {
		t.Fatalf("local gguf: %+v %v", s, err)
	}
	if s, err := Resolve(ctx, st, Options{}); err != nil || s.Kind != KindSafetensors || s.Local != st {
		t.Fatalf("local folder: %+v %v", s, err)
	}
	if s, err := Resolve(ctx, filepath.Join(st, "model.safetensors"), Options{}); err != nil || s.Kind != KindSafetensors {
		t.Fatalf("local safetensors file: %+v %v", s, err)
	}
	if s, err := Resolve(ctx, "http://127.0.0.1:8080/v1", Options{}); err != nil || s.Kind != KindServer {
		t.Fatalf("server: %+v %v", s, err)
	}
	for _, in := range []string{"https://huggingface.co/a/b/blob/main/x-Q4_K_M.gguf", "hf://a/b", "a/b@dev:x.gguf"} {
		if reHFURL.MatchString(in) || reRepoID.MatchString(in[len("hf://"):]) || reRepoID.MatchString(in) {
			continue
		}
		t.Fatalf("form not recognised: %s", in)
	}
}
