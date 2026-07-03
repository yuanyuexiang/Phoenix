// 文档解析服务(说明书 §7):把 PDF/Word/Excel 等办公文档转成纯文本。
// 无状态,POST /parse multipart(file) → {"text": ...}。
package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/yuanyuexiang/phoenix/internal/api"
	"github.com/yuanyuexiang/phoenix/internal/parser"
)

func main() {
	addr := os.Getenv("PHX_PARSER_ADDR")
	if addr == "" {
		addr = ":8082"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /parse", handleParse)

	slog.Info("parser 文档解析服务已启动", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("parser 服务退出", "error", err)
		os.Exit(1)
	}
}

func handleParse(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "缺少 multipart 字段 file")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 128<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	text, err := parser.ExtractText(header.Filename, data)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.ParseResponse{Text: text})
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}
