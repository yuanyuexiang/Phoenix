package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yuanyuexiang/phoenix/internal/model"
	"github.com/yuanyuexiang/phoenix/internal/schema"
)

// LLM 通过 OpenAI 兼容的 chat/completions 端点做字段提取。
// DeepSeek、Qwen(DashScope)及多数私有化推理服务均兼容该协议,
// 因此"模型来源可配置"只是换 Endpoint/Model 两个配置项。
type LLM struct {
	Endpoint string // 形如 https://api.deepseek.com/v1,不含 /chat/completions
	APIKey   string
	Model    string
	Client   *http.Client
}

func NewLLM(endpoint, apiKey, modelName string) *LLM {
	return &LLM{
		Endpoint: strings.TrimRight(endpoint, "/"),
		APIKey:   apiKey,
		Model:    modelName,
		Client:   &http.Client{Timeout: 120 * time.Second},
	}
}

func (l *LLM) Name() string { return "llm:" + l.Model }

func (l *LLM) Extract(ctx context.Context, text string, dt *schema.DocType) ([]model.Field, error) {
	prompt, err := buildPrompt(text, dt)
	if err != nil {
		return nil, err
	}
	content, err := l.chat(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return parseFields(content, dt)
}

// ExtractOpen 开放提取:不套 schema,让模型抽取文档中实际存在的键值对。
func (l *LLM) ExtractOpen(ctx context.Context, text string) ([]model.Field, error) {
	prompt := fmt.Sprintf(`你是企业文档字段提取引擎。文档类型未知,请把正文中实际存在的关键信息以键值对形式全部提取出来。

要求:
- 只输出一个 JSON 对象,不要输出任何其他内容。
- 顶层键 "fields" 是数组,每个元素形如 {"name": 键名(用文档中的原始叫法), "value": 值, "confidence": 0~1 的置信度}。
- 只提取文档中真实存在的信息,不要编造;金额、日期保留原始写法。
- 最多提取 %d 项,优先编号、名称、金额、日期、当事方等关键信息。

文档正文:
<<<
%s
>>>`, openMaxFields, text)

	content, err := l.chat(ctx, prompt)
	if err != nil {
		return nil, err
	}
	var out struct {
		Fields []model.Field `json:"fields"`
	}
	if err := json.Unmarshal([]byte(stripFence(content)), &out); err != nil {
		return nil, fmt.Errorf("llm: 无法解析开放提取输出: %w", err)
	}
	if len(out.Fields) > openMaxFields {
		out.Fields = out.Fields[:openMaxFields]
	}
	return out.Fields, nil
}

// Classify 让模型在候选单据类型中判断文档归属。
func (l *LLM) Classify(ctx context.Context, text string, candidates []Candidate) (string, float64, error) {
	cands, err := json.MarshalIndent(candidates, "", "  ")
	if err != nil {
		return "", 0, err
	}
	prompt := fmt.Sprintf(`你是企业单据分类引擎。判断下面的文档属于候选类型中的哪一种。

要求:
- 只输出一个 JSON 对象:{"doc_type": 候选类型的 Name 值, "confidence": 0~1 的置信度}。
- 必须从候选列表中选择;都不像时 doc_type 置为空字符串,confidence 置 0。

候选类型:
%s

文档正文:
<<<
%s
>>>`, cands, text)

	content, err := l.chat(ctx, prompt)
	if err != nil {
		return "", 0, err
	}
	var out struct {
		DocType    string  `json:"doc_type"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(stripFence(content)), &out); err != nil {
		return "", 0, fmt.Errorf("llm: 无法解析分类输出: %w", err)
	}
	for _, c := range candidates { // 只信任候选集内的答案
		if c.Name == out.DocType {
			return out.DocType, out.Confidence, nil
		}
	}
	return "", 0, nil
}

func buildPrompt(text string, dt *schema.DocType) (string, error) {
	specs, err := json.MarshalIndent(dt.Fields, "", "  ")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`你是企业文档字段提取引擎。从下面的文档正文中提取指定字段。

要求:
- 只输出一个 JSON 对象,不要输出任何其他内容。
- 顶层键 "fields" 是数组,每个元素形如 {"name": 字段名, "value": 提取到的值, "confidence": 0~1 的置信度}。
- 文档中找不到的字段,value 置为空字符串,confidence 置为 0。不要编造。
- 金额、日期等保留文档中的原始写法,不做换算。

字段定义(name 为输出键名,label/aliases 是它在文档中可能的叫法):
%s

文档正文:
<<<
%s
>>>`, specs, text), nil
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	ResponseFormat *respFormat   `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type respFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (l *LLM) chat(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model:          l.Model,
		Messages:       []chatMessage{{Role: "user", Content: prompt}},
		Temperature:    0,
		ResponseFormat: &respFormat{Type: "json_object"},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.Endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.APIKey)

	resp, err := l.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: 请求模型失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	var cr chatResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return "", fmt.Errorf("llm: 响应非 JSON(HTTP %d): %s", resp.StatusCode, truncate(string(data), 200))
	}
	if cr.Error != nil {
		return "", fmt.Errorf("llm: 模型端返回错误: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("llm: 响应中没有 choices(HTTP %d)", resp.StatusCode)
	}
	return cr.Choices[0].Message.Content, nil
}

// stripFence 容忍模型把 JSON 包在 ```json ... ``` 里的情况。
func stripFence(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}

func parseFields(content string, dt *schema.DocType) ([]model.Field, error) {
	var out struct {
		Fields []model.Field `json:"fields"`
	}
	if err := json.Unmarshal([]byte(stripFence(content)), &out); err != nil {
		return nil, fmt.Errorf("llm: 无法解析模型输出: %w", err)
	}

	// 对齐 schema:只保留声明过的字段,缺的补空条目,顺序与 schema 一致。
	byName := map[string]model.Field{}
	for _, f := range out.Fields {
		byName[f.Name] = f
	}
	fields := make([]model.Field, 0, len(dt.Fields))
	for _, spec := range dt.Fields {
		if f, ok := byName[spec.Name]; ok {
			fields = append(fields, f)
		} else {
			fields = append(fields, model.Field{Name: spec.Name})
		}
	}
	return fields, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
