// AI 服务(说明书 §7):基于大模型的字段提取与图片转写,无状态。
// POST /extract    {text, doc_type, fields[]}   → {extractor, fields[]}
// POST /transcribe {filename, content_base64}   → {text, transcriber}(视觉大模型)
//
// 字段定义随请求下发(单据类型配置归 workflow 管);
// 模型来源可配置(说明书 §13):设 PHX_LLM_ENDPOINT 用真实模型,否则用 Mock;
// 设 PHX_VISION_ENDPOINT 启用图片转写,否则 /transcribe 返回"未启用"。
package main

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/yuanyuexiang/phoenix/internal/api"
	"github.com/yuanyuexiang/phoenix/internal/config"
	"github.com/yuanyuexiang/phoenix/internal/extract"
	"github.com/yuanyuexiang/phoenix/internal/httpx"
	"github.com/yuanyuexiang/phoenix/internal/schema"
)

func main() {
	cfg := config.Load()
	addr := os.Getenv("PHX_AI_ADDR")
	if addr == "" {
		addr = ":8083"
	}

	var extractor extract.Extractor = extract.Mock{}
	if cfg.LLMEndpoint != "" {
		extractor = extract.NewLLM(cfg.LLMEndpoint, cfg.LLMAPIKey, cfg.LLMModel)
	}
	slog.Info("ai 字段提取器就绪", "extractor", extractor.Name())

	var transcriber extract.Transcriber // nil = 图片转写未启用
	if cfg.VisionEndpoint != "" {
		transcriber = extract.NewVLM(cfg.VisionEndpoint, cfg.VisionAPIKey, cfg.VisionModel)
		slog.Info("ai 图片转写器就绪", "transcriber", transcriber.Name())
	} else {
		slog.Warn("未配置 PHX_VISION_ENDPOINT,图片转写未启用(上传图片将在提取阶段报错)")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /extract", func(w http.ResponseWriter, r *http.Request) {
		handleExtract(w, r, extractor)
	})
	mux.HandleFunc("POST /classify", func(w http.ResponseWriter, r *http.Request) {
		handleClassify(w, r, extractor)
	})
	mux.HandleFunc("POST /transcribe", func(w http.ResponseWriter, r *http.Request) {
		handleTranscribe(w, r, transcriber)
	})

	if err := httpx.Serve(addr, mux, "ai 字段提取服务"); err != nil {
		slog.Error("ai 服务退出", "error", err)
		os.Exit(1)
	}
}

func handleExtract(w http.ResponseWriter, r *http.Request, extractor extract.Extractor) {
	var req api.ExtractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text 不能为空")
		return
	}

	// Fields 为空 = 开放提取模式(类型识别失败的兜底,见 internal/extract)
	if len(req.Fields) == 0 {
		fields, err := extractor.ExtractOpen(r.Context(), req.Text)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, api.ExtractResponse{Extractor: extractor.Name() + ":open", Fields: fields})
		return
	}

	dt := &schema.DocType{Name: req.DocType}
	for _, f := range req.Fields {
		dt.Fields = append(dt.Fields, schema.FieldSpec{
			Name:        f.Name,
			Label:       f.Label,
			Description: f.Description,
			Aliases:     f.Aliases,
		})
	}

	fields, err := extractor.Extract(r.Context(), req.Text, dt)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, api.ExtractResponse{Extractor: extractor.Name(), Fields: fields})
}

func handleClassify(w http.ResponseWriter, r *http.Request, extractor extract.Extractor) {
	var req api.ClassifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if req.Text == "" || len(req.Candidates) == 0 {
		writeError(w, http.StatusBadRequest, "text 与 candidates 均不能为空")
		return
	}
	candidates := make([]extract.Candidate, 0, len(req.Candidates))
	for _, c := range req.Candidates {
		candidates = append(candidates, extract.Candidate{
			Name:        c.Name,
			Title:       c.Title,
			Description: c.Description,
			Labels:      c.Labels,
		})
	}
	docType, confidence, err := extractor.Classify(r.Context(), req.Text, candidates)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, api.ClassifyResponse{DocType: docType, Confidence: confidence, Classifier: extractor.Name()})
}

func handleTranscribe(w http.ResponseWriter, r *http.Request, transcriber extract.Transcriber) {
	if transcriber == nil {
		writeError(w, http.StatusNotImplemented,
			"图片转写未启用:请为 ai 服务配置 PHX_VISION_ENDPOINT / PHX_VISION_API_KEY / PHX_VISION_MODEL(OpenAI 兼容视觉端点,如阿里云百炼)")
		return
	}
	var req api.TranscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if req.Filename == "" || req.ContentBase64 == "" {
		writeError(w, http.StatusBadRequest, "filename 与 content_base64 均不能为空")
		return
	}
	data, err := base64.StdEncoding.DecodeString(req.ContentBase64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "content_base64 解码失败: "+err.Error())
		return
	}
	text, err := transcriber.Transcribe(r.Context(), req.Filename, data)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, api.TranscribeResponse{Text: text, Transcriber: transcriber.Name()})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}
