// Package presets lists models LvW-bench knows how to run out of the box.
package presets

// Preset is a named model input.
type Preset struct {
	Name    string   `json:"name"`
	Input   string   `json:"input"`
	Kind    string   `json:"kind"` // decision, generative
	Tracks  []string `json:"tracks"`
	Args    []string `json:"args,omitempty"`
	Comment string   `json:"comment"`
	Mode    string   `json:"mode,omitempty"` // specialist: trained on the same task as a workflow
}

// All presets: the Spot Snake / Mermaid models and small baselines.
var All = []Preset{
	{"spot-snake-d1-2b", "Lucasgitguy-co/spot-snake-d1-2B-v0.1", "decision", []string{"decide"}, nil, "Spot Snake d1-2B, GGUF Q4_K_M through llama.cpp /v1/systemone", "specialist"},
	{"spot-snake-d1-3b", "Lucasgitguy-co/spot-snake-d1-3B-v0.1", "decision", []string{"decide"}, nil, "Spot Snake d1-3B, GGUF Q4_K_M through llama.cpp /v1/systemone", "specialist"},
	{"spot-snake-minimind-jev", "Lucasgitguy-co/spot-snake-minimind-jev-v0.1", "decision", []string{"decide"}, nil, "MiniMind-JEV ranking head: answers Spot's move questions only, through the Python helper", "specialist"},
	{"minimind-mermaid-run2", "Lucasgitguy-co/minimind-mermaid-run2", "generative", []string{"decide", "answer", "reason"}, []string{"--subfolder", "ep3"}, "MiniMind Mermaid run 2, epoch 3, safetensors through the helper", ""},
	{"minicpm-mermaid", "Lucasgitguy-co/minicpm-mermaid-gguf", "generative", []string{"decide", "answer", "reason"}, nil, "MiniCPM-V Mermaid fine-tune, GGUF, text only", ""},
	{"lfm3-vl-2b-heretic", "Lucasgitguy-co/lfm3-vl-2b-heretic-gguf", "generative", []string{"decide", "answer", "reason"}, nil, "LFM3-VL 2B heretic, text only", ""},
	{"d1-2b", "LiquidAI/d1-2B-GGUF", "decision", []string{"decide"}, nil, "Liquid AI d1-2B base, the Spot Snake starting point", ""},
	{"qwen3-0.6b", "unsloth/Qwen3-0.6B-GGUF", "generative", []string{"decide", "answer", "reason"}, nil, "Qwen3 0.6B baseline (native thinking)", ""},
	{"smollm2-135m", "bartowski/SmolLM2-135M-Instruct-GGUF", "generative", []string{"decide", "answer", "reason"}, nil, "SmolLM2 135M, the smallest smoke-test model", ""},
	{"smollm2-360m", "bartowski/SmolLM2-360M-Instruct-GGUF", "generative", []string{"decide", "answer", "reason"}, nil, "SmolLM2 360M baseline", ""},
	{"smollm2-1.7b", "bartowski/SmolLM2-1.7B-Instruct-GGUF", "generative", []string{"decide", "answer", "reason"}, nil, "SmolLM2 1.7B baseline", ""},
	{"lfm2-350m", "LiquidAI/LFM2-350M-GGUF", "generative", []string{"decide", "answer", "reason"}, nil, "LFM2 350M baseline", ""},
	{"lfm2-1.2b", "LiquidAI/LFM2-1.2B-GGUF", "generative", []string{"decide", "answer", "reason"}, nil, "LFM2 1.2B baseline", ""},
}

// Find returns a preset by name.
func Find(name string) (Preset, bool) {
	for _, p := range All {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}
