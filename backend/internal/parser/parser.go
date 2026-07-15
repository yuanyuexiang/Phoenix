// Package parser 把办公文档格式统一转成纯文本(文档解析服务的核心逻辑)。
//
// 已支持:纯文本、.docx(解压取 word/document.xml)。
// 图片不在此处理——workflow 会把图片路由到 ai 服务做视觉转写。
// TODO:.pdf(需区分文字层与扫描件)、.xlsx、老版 .doc —— 见产品说明书 §5 功能③。
package parser

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

// ImageExts 是应当交由 ai 服务视觉转写而非本包的图片扩展名(小写、含点)。
// 取值须与视觉端点支持的格式一致(extract.imageMIME,单测强制同步):
// DashScope 不支持 tiff,故 .tif/.tiff 不在此表(见 ExtractText 的显式报错)。
var ImageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".bmp": true, ".webp": true, ".heic": true,
}

// ExtractText 按扩展名解析文档为纯文本。
func ExtractText(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch {
	case ext == ".txt" || ext == ".md":
		return string(data), nil
	case ext == ".docx":
		return docxText(data)
	case ImageExts[ext]:
		return "", fmt.Errorf("parser: 图片应交由 AI 视觉转写处理")
	case ext == ".tif" || ext == ".tiff":
		return "", fmt.Errorf("parser: 暂不支持 TIFF 图片,请转换为 PNG/JPEG 后重试")
	case ext == ".pdf":
		return "", fmt.Errorf("parser: PDF 解析尚未实现(需区分文字层与扫描件)")
	case ext == ".xlsx" || ext == ".xls" || ext == ".doc":
		return "", fmt.Errorf("parser: %s 解析尚未实现", ext)
	default:
		return "", fmt.Errorf("parser: 不支持的文件类型 %q", ext)
	}
}

var (
	docxPara = regexp.MustCompile(`</w:p>`)
	docxTags = regexp.MustCompile(`<[^>]+>`)
)

// docxText 解压 .docx 提取 word/document.xml 的正文,段落转换行。
func docxText(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("parser: 打开 docx 失败: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		xmlData, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		if err != nil {
			return "", err
		}
		text := docxPara.ReplaceAllString(string(xmlData), "\n")
		text = docxTags.ReplaceAllString(text, "")
		return strings.TrimSpace(text), nil
	}
	return "", fmt.Errorf("parser: docx 中没有 word/document.xml")
}
