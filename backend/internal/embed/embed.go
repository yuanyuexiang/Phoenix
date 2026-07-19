// Package embed 调用 OpenAI 兼容的 embeddings 端点,把文本向量化(RAG 知识库检索用)。
// 基准为阿里 DashScope 兼容模式(text-embedding-v3/v4),任何 OpenAI 兼容端点均可。
// 注意:这是"检索索引"用途,不是内容识别/提取(识别在 WorkBuddy 侧完成)。
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client 是 embedding 客户端。Dim 是期望维度,须与 store 迁移里的 vector(N) 一致。
type Client struct {
	Endpoint string // 如 https://dashscope.aliyuncs.com/compatible-mode/v1,不含 /embeddings
	APIKey   string
	Model    string
	Dim      int
	HTTP     *http.Client
}

func New(endpoint, apiKey, model string, dim int) *Client {
	return &Client{
		Endpoint: strings.TrimRight(endpoint, "/"),
		APIKey:   apiKey,
		Model:    model,
		Dim:      dim,
		HTTP:     &http.Client{Timeout: 60 * time.Second},
	}
}

type embedRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embedResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Embed 批量向量化;返回顺序与入参一致(按 response.data.index 归位)。
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(embedRequest{Model: c.Model, Input: texts, Dimensions: c.Dim})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: 请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	var er embedResponse
	if err := json.Unmarshal(raw, &er); err != nil {
		return nil, fmt.Errorf("embed: 响应非 JSON(HTTP %d)", resp.StatusCode)
	}
	if er.Error != nil {
		return nil, fmt.Errorf("embed: 端点返回错误: %s", er.Error.Message)
	}
	if len(er.Data) != len(texts) {
		return nil, fmt.Errorf("embed: 返回向量数 %d 与输入 %d 不一致", len(er.Data), len(texts))
	}
	out := make([][]float32, len(texts))
	for _, d := range er.Data {
		if d.Index < 0 || d.Index >= len(out) {
			return nil, fmt.Errorf("embed: 非法 index %d", d.Index)
		}
		out[d.Index] = d.Embedding
	}
	return out, nil
}
