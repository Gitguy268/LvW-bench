// Package backend runs and talks to model servers: llama-server for GGUF, the uv Python
// helper for safetensors, or any OpenAI-compatible / System One server the user points at.
package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Gitguy268/LvW-bench/internal/schema"
)

// Client talks to one server.
type Client struct {
	BaseURL string // ends in /v1
	Model   string // model name sent in requests (Ollama, LM Studio need it)
	APIKey  string
	http    *http.Client
}

// NewClient normalises the base URL to end in /v1.
func NewClient(base, model string) *Client {
	base = strings.TrimRight(base, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return &Client{BaseURL: base, Model: model, http: &http.Client{Timeout: 15 * time.Minute}}
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return fmt.Errorf("%s: HTTP %d: %s", path, resp.StatusCode, msg)
	}
	return json.Unmarshal(data, out)
}

// Modalities reports the output modalities of the served model ("text", "decisions").
func (c *Client) Modalities(ctx context.Context) []string {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/models", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var m struct {
		Data []struct {
			ID           string `json:"id"`
			Architecture struct {
				Output []string `json:"output_modalities"`
			} `json:"architecture"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&m) != nil || len(m.Data) == 0 {
		return nil
	}
	if c.Model == "" {
		c.Model = m.Data[0].ID
	}
	return m.Data[0].Architecture.Output
}

// Has reports whether a modality is in the list.
func Has(mods []string, m string) bool {
	for _, x := range mods {
		if x == m {
			return true
		}
	}
	return false
}

// Message is a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the subset of the OpenAI chat API LvW-bench uses.
type ChatRequest struct {
	Messages    []Message
	MaxTokens   int
	Temperature float64
	TopLogprobs int   // >0 asks for logprobs of the first tokens
	Thinking    *bool // enable_thinking for chat templates that support it
	Seed        int
}

// ChatResponse is what came back.
type ChatResponse struct {
	Content      string
	Reasoning    string
	FinishReason string
	PromptTokens int
	OutputTokens int
	FirstTop     map[string]float64 // token -> probability, first output token
	HasLogprobs  bool
	Elapsed      time.Duration
}

// Chat sends one chat completion.
func (c *Client) Chat(ctx context.Context, r ChatRequest) (*ChatResponse, error) {
	body := map[string]any{
		"model":       c.Model,
		"messages":    r.Messages,
		"max_tokens":  r.MaxTokens,
		"temperature": r.Temperature,
		"stream":      false,
	}
	if r.Seed != 0 {
		body["seed"] = r.Seed
	}
	if r.TopLogprobs > 0 {
		body["logprobs"] = true
		body["top_logprobs"] = r.TopLogprobs
	}
	if r.Thinking != nil {
		body["chat_template_kwargs"] = map[string]any{"enable_thinking": *r.Thinking}
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				Reasoning        string `json:"reasoning"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
			Logprobs     *struct {
				Content []struct {
					Token       string  `json:"token"`
					Logprob     float64 `json:"logprob"`
					TopLogprobs []struct {
						Token   string  `json:"token"`
						Logprob float64 `json:"logprob"`
					} `json:"top_logprobs"`
				} `json:"content"`
			} `json:"logprobs"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	start := time.Now()
	if err := c.post(ctx, "/chat/completions", body, &out); err != nil {
		return nil, err
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("chat: empty response")
	}
	ch := out.Choices[0]
	res := &ChatResponse{
		Content:      ch.Message.Content,
		Reasoning:    ch.Message.ReasoningContent + ch.Message.Reasoning,
		FinishReason: ch.FinishReason,
		PromptTokens: out.Usage.PromptTokens,
		OutputTokens: out.Usage.CompletionTokens,
		Elapsed:      time.Since(start),
	}
	if ch.Logprobs != nil && len(ch.Logprobs.Content) > 0 {
		res.HasLogprobs = true
		res.FirstTop = map[string]float64{}
		first := ch.Logprobs.Content[0]
		for _, t := range first.TopLogprobs {
			res.FirstTop[t.Token] += math.Exp(t.Logprob)
		}
		if len(first.TopLogprobs) == 0 {
			res.FirstTop[first.Token] += math.Exp(first.Logprob)
		}
	}
	return res, nil
}

// SystemOne sends one /v1/systemone request and returns a distribution per question.
func (c *Client) SystemOne(ctx context.Context, state json.RawMessage, qs map[string]schema.Question) (map[string]map[string]float64, int, error) {
	var out struct {
		Answers map[string]struct {
			Type          string             `json:"type"`
			Choice        string             `json:"choice"`
			Probabilities map[string]float64 `json:"probabilities"`
			Noul          *float64           `json:"noul"`
		} `json:"answers"`
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	}
	body := map[string]any{"state": state, "questions": qs}
	if c.Model != "" {
		body["model"] = c.Model
	}
	if err := c.post(ctx, "/systemone", body, &out); err != nil {
		return nil, 0, err
	}
	res := map[string]map[string]float64{}
	for id, a := range out.Answers {
		p := map[string]float64{}
		for k, v := range a.Probabilities {
			p[k] = v
		}
		if a.Noul != nil && len(p) == 0 {
			p["true"], p["false"] = *a.Noul, 1-*a.Noul
		}
		if len(p) == 0 && a.Choice != "" {
			p[a.Choice] = 1
		}
		res[id] = p
	}
	return res, out.Usage.InputTokens, nil
}

// WaitReady polls /health (llama-server, helper) or /models until the server answers.
func WaitReady(ctx context.Context, base string, exited <-chan error) error {
	root := strings.TrimSuffix(strings.TrimRight(base, "/"), "/v1")
	hc := &http.Client{Timeout: 3 * time.Second}
	for {
		for _, p := range []string{root + "/health", root + "/v1/models"} {
			if resp, err := hc.Get(p); err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-exited:
			return fmt.Errorf("server exited before it was ready: %v", err)
		case <-time.After(500 * time.Millisecond):
		}
	}
}
