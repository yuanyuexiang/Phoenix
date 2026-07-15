package extract

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// Transcriber 把图片转写为纯文本(替代原独立 OCR 服务)。
// 与 Extractor 相互独立:Mock 无需实现;未配置视觉模型时 ai 服务不装配
// Transcriber,/transcribe 直接返回"未启用"。
type Transcriber interface {
	Transcribe(ctx context.Context, filename string, data []byte) (string, error)
	Name() string // vision:<model>
}

// VLM 通过 OpenAI 兼容 chat/completions 端点的 vision 消息做图片转写。
// 基准为阿里 DashScope 兼容模式(qwen-vl-*),任何 OpenAI 兼容视觉端点均可。
type VLM struct {
	Endpoint string // 形如 https://dashscope.aliyuncs.com/compatible-mode/v1,不含 /chat/completions
	APIKey   string
	Model    string
	Client   *http.Client
}

func NewVLM(endpoint, apiKey, modelName string) *VLM {
	return &VLM{
		Endpoint: strings.TrimRight(endpoint, "/"),
		APIKey:   apiKey,
		Model:    modelName,
		Client:   &http.Client{Timeout: 120 * time.Second},
	}
}

func (v *VLM) Name() string { return "vision:" + v.Model }

// imageMIME 是支持转写的图片扩展名 → data URL 的 MIME。
// 必须与 parser.ImageExts(路由表)保持一致,由 TestImageExtsHaveMIME 强制。
var imageMIME = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".bmp":  "image/bmp",
	".webp": "image/webp",
	".heic": "image/heic",
}

// maxImageBase64 是 DashScope 对单图 base64 编码后的大小上限。
const maxImageBase64 = 10 << 20

// transcribePrompt 要求忠实转写。「标签: 值」独立成行是刻意的:
// Mock 提取器与分类启发式依赖该格式(internal/extract/mock.go)。
const transcribePrompt = `你是文档转写引擎。请把图片中的全部文字忠实转写出来。

要求:
- 只输出转写结果,不要任何解释、开场白或代码围栏。
- 按原始阅读顺序转写,保留换行与分段;"标签: 值"式内容各占一行。
- 表格用 Markdown 表格语法输出,保持行列对应。
- 只转写图片中真实存在的文字,禁止补全、翻译或编造;数字、金额、日期保持原样。
- 印章、手写签名尽量辨认,无法辨认处标注 [印章] / [签名];无法辨认的字符用〓占位。
- 图片中没有任何文字时,输出空字符串。`

// vision 消息的 content 是多模态数组,与 llm.go 的纯文本 chatMessage 不同;
// 且不能携带 response_format(DashScope vision 模型会拒绝),故独立建组。
type visionChatRequest struct {
	Model       string          `json:"model"`
	Messages    []visionMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
}

type visionMessage struct {
	Role    string       `json:"role"`
	Content []visionPart `json:"content"`
}

type visionPart struct {
	Type     string        `json:"type"` // image_url | text
	Text     string        `json:"text,omitempty"`
	ImageURL *visionImgURL `json:"image_url,omitempty"`
}

type visionImgURL struct {
	URL string `json:"url"` // data:{mime};base64,{payload}
}

func (v *VLM) Transcribe(ctx context.Context, filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	mime, ok := imageMIME[ext]
	if !ok {
		return "", fmt.Errorf("vision: 不支持的图片格式 %q", ext)
	}
	if base64.StdEncoding.EncodedLen(len(data)) > maxImageBase64 {
		return "", fmt.Errorf("vision: 图片过大(base64 编码后超过 %dMB),请压缩后重试", maxImageBase64>>20)
	}

	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	body, err := json.Marshal(visionChatRequest{
		Model: v.Model,
		Messages: []visionMessage{{
			Role: "user",
			Content: []visionPart{
				{Type: "image_url", ImageURL: &visionImgURL{URL: dataURL}},
				{Type: "text", Text: transcribePrompt},
			},
		}},
		Temperature: 0,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.Endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.APIKey)

	resp, err := v.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vision: 请求视觉模型失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("vision: 响应非 JSON(HTTP %d): %s", resp.StatusCode, truncate(string(raw), 200))
	}
	if cr.Error != nil {
		return "", fmt.Errorf("vision: 模型端返回错误: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("vision: 响应中没有 choices(HTTP %d)", resp.StatusCode)
	}
	// 空文本不在此处报错:pipeline 对空正文有统一的失败分支与文案
	return strings.TrimSpace(stripTranscriptFence(cr.Choices[0].Message.Content)), nil
}

// stripTranscriptFence 剥离整段包裹的代码围栏(如 ```markdown ... ```)。
// 部分模型(实测 qwen-vl-ocr)无视"不要代码围栏"的指令。
func stripTranscriptFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:] // 丢掉 ```lang 行
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}
