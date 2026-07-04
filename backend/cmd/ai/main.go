// AI 服务(说明书 §7):基于大模型的字段提取,无状态。
// POST /extract {text, doc_type, fields[]} → {extractor, fields[]}。
//
// 字段定义随请求下发(单据类型配置归 workflow 管);
// 模型来源可配置:设 PHX_LLM_ENDPOINT 用真实模型,否则用 Mock(说明书 §13)。
package main

import (
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /extract", func(w http.ResponseWriter, r *http.Request) {
		handleExtract(w, r, extractor)
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
	if req.Text == "" || len(req.Fields) == 0 {
		writeError(w, http.StatusBadRequest, "text 与 fields 均不能为空")
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.ExtractResponse{Extractor: extractor.Name(), Fields: fields})
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}
