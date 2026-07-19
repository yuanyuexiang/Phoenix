package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Chunk 是一段正文切片及其向量。
type Chunk struct {
	Index     int
	Content   string
	Embedding []float32
}

// ChunkHit 是语义检索的一条命中(供 WorkBuddy 据此作答)。
type ChunkHit struct {
	DocumentID string  `json:"document_id"`
	Filename   string  `json:"filename"`
	DocType    string  `json:"doc_type"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"` // 余弦相似度,越大越相关
}

// vecLiteral 把向量格式化为 pgvector 字面量 "[a,b,c]"(配合 $n::vector 写入,无需额外依赖)。
func vecLiteral(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// ReplaceChunks 幂等重建某文档的切片:先删旧 chunk 再插新的(同事务)。
func (db *DB) ReplaceChunks(ctx context.Context, documentID string, chunks []Chunk) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM document_chunks WHERE document_id = $1`, documentID); err != nil {
		return err
	}
	for _, c := range chunks {
		if _, err := tx.Exec(ctx, `
			INSERT INTO document_chunks (document_id, chunk_index, content, embedding)
			VALUES ($1, $2, $3, $4::vector)`,
			documentID, c.Index, c.Content, vecLiteral(c.Embedding)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// SearchChunks 按向量余弦相似度检索 top-K 切片,可选按单据类型/上传人过滤。
func (db *DB) SearchChunks(ctx context.Context, queryVec []float32, limit int, docType, uploadedBy string) ([]ChunkHit, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	args := []any{vecLiteral(queryVec)}
	conds := ""
	if docType != "" {
		args = append(args, docType)
		conds += fmt.Sprintf(" AND d.doc_type = $%d", len(args))
	}
	if uploadedBy != "" {
		args = append(args, uploadedBy)
		conds += fmt.Sprintf(" AND d.uploaded_by = $%d", len(args))
	}
	args = append(args, limit)

	rows, err := db.pool.Query(ctx, fmt.Sprintf(`
		SELECT c.document_id, d.filename, d.doc_type, c.content,
		       1 - (c.embedding <=> $1::vector) AS score
		FROM document_chunks c
		JOIN documents d ON d.id = c.document_id
		WHERE 1=1%s
		ORDER BY c.embedding <=> $1::vector
		LIMIT $%d`, conds, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []ChunkHit
	for rows.Next() {
		var h ChunkHit
		if err := rows.Scan(&h.DocumentID, &h.Filename, &h.DocType, &h.Content, &h.Score); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}
