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
		INSERT INTO documents (id, doc_type, filename, object_key, content_text, status, error, fields, issues, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		d.ID, d.DocType, d.Filename, d.ObjectKey, d.Text, d.Status, d.Error, fields, issues, d.UploadedBy)
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
		SET content_text = $2, status = $3, error = $4, fields = $5, issues = $6, reviewed_by = $7, updated_at = now()
		WHERE id = $1`,
		d.ID, d.Text, d.Status, d.Error, fields, issues, d.ReviewedBy)
	return err
}

// GetDocument 按 ID 取文档;不存在时返回 pgx.ErrNoRows。
func (db *DB) GetDocument(ctx context.Context, id string) (*model.Document, error) {
	row := db.pool.QueryRow(ctx, `
		SELECT id, doc_type, filename, object_key, content_text, status, error, fields, issues, uploaded_by, reviewed_by, created_at, updated_at
		FROM documents WHERE id = $1`, id)
	return scanDocument(row)
}

// FieldFilter 是对提取字段(fields JSONB)的精确/比较过滤(如「金额>10000」「甲方 包含 某公司」)。
type FieldFilter struct {
	Field  string   `json:"field"`            // 字段 name(与 doctype schema 一致)
	Op     string   `json:"op"`               // eq|ne|contains|gt|gte|lt|lte|in
	Value  string   `json:"value,omitempty"`  // 单值(eq/ne/contains/gt/gte/lt/lte);数值比较时按数值解释(自动去千分位逗号)
	Values []string `json:"values,omitempty"` // in 的候选值
}

// QueryFilter 是 query_document 的查询条件,零值字段不参与过滤。
type QueryFilter struct {
	DocType      string
	Status       string
	Keyword      string // 匹配文件名或正文
	UploadedBy   string // 按上传人精确匹配
	FieldFilters []FieldFilter
	Limit        int
}

// numericOps 需要把字符串字段值当数值比较(去逗号 + 正则守卫防 cast 失败)。
var numericOps = map[string]string{"gt": ">", "gte": ">=", "lt": "<", "lte": "<="}

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
	if f.UploadedBy != "" {
		add("uploaded_by = $%d", f.UploadedBy)
	}
	for _, ff := range f.FieldFilters {
		cond, err := fieldFilterCond(ff, &args)
		if err != nil {
			return nil, err
		}
		conds = append(conds, cond)
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
		SELECT id, doc_type, filename, object_key, content_text, status, error, fields, issues, uploaded_by, reviewed_by, created_at, updated_at
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

// fieldFilterCond 把一个 FieldFilter 编译成对 fields JSONB 的 EXISTS 子查询条件,
// 并把参数追加到 args。fields 是 [{name,value,confidence},...],这里对匹配 name 的元素
// 按 op 比较其 value。数值比较自动去千分位逗号,并用正则守卫防止非数值 value 触发 cast 报错。
func fieldFilterCond(ff FieldFilter, args *[]any) (string, error) {
	if ff.Field == "" {
		return "", fmt.Errorf("field_filter: field 不能为空")
	}
	*args = append(*args, ff.Field)
	nameP := len(*args)

	// EXISTS (SELECT 1 FROM jsonb_array_elements(fields) fe WHERE fe->>'name' = $name AND <pred>)
	wrap := func(pred string) string {
		return fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(fields) fe WHERE fe->>'name' = $%d AND %s)", nameP, pred)
	}

	switch ff.Op {
	case "eq":
		*args = append(*args, ff.Value)
		return wrap(fmt.Sprintf("fe->>'value' = $%d", len(*args))), nil
	case "ne":
		*args = append(*args, ff.Value)
		return wrap(fmt.Sprintf("fe->>'value' <> $%d", len(*args))), nil
	case "contains":
		*args = append(*args, "%"+ff.Value+"%")
		return wrap(fmt.Sprintf("fe->>'value' ILIKE $%d", len(*args))), nil
	case "in":
		if len(ff.Values) == 0 {
			return "", fmt.Errorf("field_filter: op=in 需要 values")
		}
		*args = append(*args, ff.Values)
		return wrap(fmt.Sprintf("fe->>'value' = ANY($%d)", len(*args))), nil
	default:
		op, ok := numericOps[ff.Op]
		if !ok {
			return "", fmt.Errorf("field_filter: 不支持的 op %q", ff.Op)
		}
		num := strings.ReplaceAll(ff.Value, ",", "")
		*args = append(*args, num)
		valP := len(*args)
		// 去逗号后用正则确认是数值再 cast,否则该元素跳过(不参与比较)
		pred := fmt.Sprintf(
			"replace(fe->>'value', ',', '') ~ '^-?[0-9]+(\\.[0-9]+)?$' AND replace(fe->>'value', ',', '')::numeric %s $%d::numeric",
			op, valP)
		return wrap(pred), nil
	}
}

// AuditEntry 是一条审计日志(谁在何时对哪份文档做了什么)。
type AuditEntry struct {
	Actor       string         // 展示口径,同 documents.uploaded_by;可为空(匿名)
	ActorSource string         // oauth | admin | anonymous
	Action      string         // upload | extract | validate | save
	DocumentID  string         // 可为空(动作未产生文档时)
	Detail      map[string]any // 全量身份 claims 及动作参数,兜底可回溯
}

// InsertAudit 写入审计日志。调用方应视其为尽力而为:失败只告警,不阻断主流程。
func (db *DB) InsertAudit(ctx context.Context, e AuditEntry) error {
	detail, err := json.Marshal(e.Detail)
	if err != nil {
		return err
	}
	if e.Detail == nil {
		detail = []byte("{}")
	}
	var docID any
	if e.DocumentID != "" {
		docID = e.DocumentID
	}
	_, err = db.pool.Exec(ctx, `
		INSERT INTO audit_log (actor, actor_source, action, document_id, detail)
		VALUES ($1, $2, $3, $4, $5)`,
		e.Actor, e.ActorSource, e.Action, docID, detail)
	return err
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
		&fields, &issues, &d.UploadedBy, &d.ReviewedBy, &d.CreatedAt, &d.UpdatedAt)
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
