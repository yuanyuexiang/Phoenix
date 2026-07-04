// Package store 实现持久化:PostgreSQL 存结构化数据,MinIO 存原始文件。
package store

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yuanyuexiang/phoenix/internal/model"
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB 封装 documents 表的读写。
type DB struct {
	pool *pgxpool.Pool
}

// Open 连接 PostgreSQL 并执行迁移。compose 启动时数据库可能尚未就绪,做有限重试。
func Open(ctx context.Context, dsn string) (*DB, error) {
	var pool *pgxpool.Pool
	var err error
	for attempt := 0; attempt < 15; attempt++ {
		pool, err = pgxpool.New(ctx, dsn)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				break
			}
			pool.Close()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if err != nil {
		return nil, fmt.Errorf("store: 连接数据库失败: %w", err)
	}
	db := &DB{pool: pool}
	if err := db.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) migrate(ctx context.Context) error {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, e := range entries {
		sqlBytes, err := migrations.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		if _, err := db.pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("store: 执行迁移 %s 失败: %w", e.Name(), err)
		}
	}
	return nil
}

func (db *DB) Close() { db.pool.Close() }

// Ping 用于健康检查。
func (db *DB) Ping(ctx context.Context) error { return db.pool.Ping(ctx) }

// CreateDocument 插入新文档记录。
func (db *DB) CreateDocument(ctx context.Context, d *model.Document) error {
	fields, issues, err := marshalJSON(d)
	if err != nil {
		return err
	}
	_, err = db.pool.Exec(ctx, `
		INSERT INTO documents (id, doc_type, filename, object_key, content_text, status, error, fields, issues)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		d.ID, d.DocType, d.Filename, d.ObjectKey, d.Text, d.Status, d.Error, fields, issues)
	return err
}

// UpdateDocument 全量更新处理结果(正文、状态、字段、问题)。
func (db *DB) UpdateDocument(ctx context.Context, d *model.Document) error {
	fields, issues, err := marshalJSON(d)
	if err != nil {
		return err
	}
	_, err = db.pool.Exec(ctx, `
		UPDATE documents
		SET content_text = $2, status = $3, error = $4, fields = $5, issues = $6, updated_at = now()
		WHERE id = $1`,
		d.ID, d.Text, d.Status, d.Error, fields, issues)
	return err
}

// GetDocument 按 ID 取文档;不存在时返回 pgx.ErrNoRows。
func (db *DB) GetDocument(ctx context.Context, id string) (*model.Document, error) {
	row := db.pool.QueryRow(ctx, `
		SELECT id, doc_type, filename, object_key, content_text, status, error, fields, issues, created_at, updated_at
		FROM documents WHERE id = $1`, id)
	return scanDocument(row)
}

// QueryFilter 是 query_document 的查询条件,零值字段不参与过滤。
type QueryFilter struct {
	DocType string
	Status  string
	Keyword string // 匹配文件名或正文
	Limit   int
}

// QueryDocuments 按条件查询,按创建时间倒序。
func (db *DB) QueryDocuments(ctx context.Context, f QueryFilter) ([]*model.Document, error) {
	var conds []string
	var args []any
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.DocType != "" {
		add("doc_type = $%d", f.DocType)
	}
	if f.Status != "" {
		add("status = $%d", f.Status)
	}
	if f.Keyword != "" {
		add("(filename ILIKE $%d OR content_text ILIKE $%[1]d)", "%"+f.Keyword+"%")
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	args = append(args, limit)

	rows, err := db.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, doc_type, filename, object_key, content_text, status, error, fields, issues, created_at, updated_at
		FROM documents %s ORDER BY created_at DESC LIMIT $%d`, where, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []*model.Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

func marshalJSON(d *model.Document) (fields, issues []byte, err error) {
	if fields, err = json.Marshal(orEmptyFields(d.Fields)); err != nil {
		return nil, nil, err
	}
	if issues, err = json.Marshal(orEmptyIssues(d.Issues)); err != nil {
		return nil, nil, err
	}
	return fields, issues, nil
}

func orEmptyFields(f []model.Field) []model.Field {
	if f == nil {
		return []model.Field{}
	}
	return f
}

func orEmptyIssues(i []model.ValidationIssue) []model.ValidationIssue {
	if i == nil {
		return []model.ValidationIssue{}
	}
	return i
}

func scanDocument(row pgx.Row) (*model.Document, error) {
	var d model.Document
	var fields, issues []byte
	err := row.Scan(&d.ID, &d.DocType, &d.Filename, &d.ObjectKey, &d.Text, &d.Status, &d.Error,
		&fields, &issues, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(fields, &d.Fields); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(issues, &d.Issues); err != nil {
		return nil, err
	}
	return &d, nil
}
