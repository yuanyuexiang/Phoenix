package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/yuanyuexiang/phoenix/internal/model"
)

// OntObject 是本体对象(configs/ontology 定义的实体实例)。
type OntObject struct {
	ID          string         `json:"id"`
	Type        string         `json:"object_type"`
	DisplayName string         `json:"display_name"`
	Properties  map[string]any `json:"properties"`
	Version     int            `json:"version"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// OntLink 是一条类型化关系(含两端展示信息与来源文档)。
type OntLink struct {
	LinkType    string `json:"link_type"`
	FromID      string `json:"from_id"`
	FromType    string `json:"from_type"`
	FromDisplay string `json:"from_display"`
	ToID        string `json:"to_id"`
	ToType      string `json:"to_type"`
	ToDisplay   string `json:"to_display"`
	DocumentID  string `json:"document_id"`
}

// ErrKeyConflict 表示并发创建同一实体时归一键撞车(调用方重试解析一次即可)。
var ErrKeyConflict = errors.New("对象归一键冲突")

// UpsertObjectByKeys 按归一键解析对象:命中则合并属性(version+1),
// 未命中则新建对象与键。返回对象 ID 与是否新建。
func (db *DB) UpsertObjectByKeys(ctx context.Context, objType, display string, props map[string]any, keyHashes []string) (string, bool, error) {
	id, found, err := db.resolveByKeys(ctx, keyHashes)
	if err != nil {
		return "", false, err
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return "", false, err
	}
	if found {
		if _, err := db.pool.Exec(ctx, `
			UPDATE objects SET display_name = $2, properties = properties || $3::jsonb,
			       version = version + 1, updated_at = now() WHERE id = $1`,
			id, display, propsJSON); err != nil {
			return "", false, err
		}
		for _, h := range keyHashes { // 回填后学到的新键(如后补税号)
			if _, err := db.pool.Exec(ctx, `
				INSERT INTO object_keys (object_id, key_hash) VALUES ($1, $2)
				ON CONFLICT (key_hash) DO NOTHING`, id, h); err != nil {
				return "", false, err
			}
		}
		return id, false, nil
	}

	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `
		INSERT INTO objects (object_type, display_name, properties)
		VALUES ($1, $2, $3) RETURNING id`, objType, display, propsJSON).Scan(&id); err != nil {
		return "", false, err
	}
	for _, h := range keyHashes {
		if _, err := tx.Exec(ctx, `INSERT INTO object_keys (object_id, key_hash) VALUES ($1, $2)`, id, h); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" { // 并发创建撞键 → 调用方重试
				return "", false, ErrKeyConflict
			}
			return "", false, err
		}
	}
	return id, true, tx.Commit(ctx)
}

func (db *DB) resolveByKeys(ctx context.Context, hashes []string) (string, bool, error) {
	for _, h := range hashes { // 按优先级逐组解析(税号先于名称)
		var id string
		err := db.pool.QueryRow(ctx, `SELECT object_id FROM object_keys WHERE key_hash = $1`, h).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return "", false, err
		}
		return id, true, nil
	}
	return "", false, nil
}

/* ---------- 链接与证据 ---------- */

func (db *DB) InsertLink(ctx context.Context, linkType, from, to, docID string) error {
	_, err := db.pool.Exec(ctx, `
		INSERT INTO links (link_type, from_object, to_object, document_id)
		VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, linkType, from, to, docID)
	return err
}

// DeleteDocumentLinks 撤销某文档贡献的全部链接(重新 save 前的"先撤销后重放")。
func (db *DB) DeleteDocumentLinks(ctx context.Context, docID string) error {
	_, err := db.pool.Exec(ctx, `DELETE FROM links WHERE document_id = $1`, docID)
	return err
}

// OtherLinkSources 统计同类型指向同一对象、来自其他文档的链接数
// (重复报销检测:发票被其他报销单引用)。
func (db *DB) OtherLinkSources(ctx context.Context, linkType, toObject, excludeDoc string) (int, error) {
	var n int
	err := db.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT document_id) FROM links
		WHERE link_type = $1 AND to_object = $2 AND document_id <> $3`,
		linkType, toObject, excludeDoc).Scan(&n)
	return n, err
}

func (db *DB) InsertObjectEvidence(ctx context.Context, objectID, docID string) error {
	_, err := db.pool.Exec(ctx, `
		INSERT INTO object_evidence (object_id, document_id)
		VALUES ($1, $2) ON CONFLICT DO NOTHING`, objectID, docID)
	return err
}

// DeleteDocumentOntology 清除某文档在本体层的全部痕迹(删除文档时调用)。
func (db *DB) DeleteDocumentOntology(ctx context.Context, docID string) error {
	if err := db.DeleteDocumentLinks(ctx, docID); err != nil {
		return err
	}
	_, err := db.pool.Exec(ctx, `DELETE FROM object_evidence WHERE document_id = $1`, docID)
	return err
}

// TruncateOntology 清空本体层(全量重建前调用;文档层不受影响)。
func (db *DB) TruncateOntology(ctx context.Context) error {
	_, err := db.pool.Exec(ctx, `TRUNCATE object_evidence, links, object_keys, objects`)
	return err
}

/* ---------- 查询 ---------- */

// ObjectFilter 是对象列表查询条件;PropFilters 复用文档字段过滤的语法与运算符。
type ObjectFilter struct {
	Type        string
	Keyword     string // display_name 模糊
	PropFilters []FieldFilter
	Limit       int
}

func (db *DB) ListObjects(ctx context.Context, f ObjectFilter) ([]OntObject, error) {
	var conds []string
	var args []any
	add := func(cond string, vals ...any) {
		for _, v := range vals {
			args = append(args, v)
		}
		conds = append(conds, cond)
	}
	if f.Type != "" {
		add(fmt.Sprintf("object_type = $%d", len(args)+1), f.Type)
	}
	if f.Keyword != "" {
		add(fmt.Sprintf("display_name ILIKE $%d", len(args)+1), "%"+f.Keyword+"%")
	}
	for _, pf := range f.PropFilters {
		cond, err := propFilterCond(pf, &args)
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
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args = append(args, limit)

	rows, err := db.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, object_type, display_name, properties, version, created_at, updated_at
		FROM objects %s ORDER BY updated_at DESC LIMIT $%d`, where, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanObjects(rows)
}

// propFilterCond 编译对 properties(扁平 JSONB map)的过滤条件。
// 数值比较带正则守卫防 cast 报错;属性值已在物化时归一化,无需再去千分位。
func propFilterCond(pf FieldFilter, args *[]any) (string, error) {
	if pf.Field == "" {
		return "", fmt.Errorf("property_filters 缺少 field")
	}
	val := func(v any) int { *args = append(*args, v); return len(*args) }
	col := fmt.Sprintf("properties->>%s", quoteArg(args, pf.Field))
	switch pf.Op {
	case "", "eq":
		return fmt.Sprintf("%s = $%d", col, val(pf.Value)), nil
	case "ne":
		return fmt.Sprintf("%s <> $%d", col, val(pf.Value)), nil
	case "contains":
		return fmt.Sprintf("%s ILIKE $%d", col, val("%"+pf.Value+"%")), nil
	case "in":
		return fmt.Sprintf("%s = ANY($%d)", col, val(pf.Values)), nil
	case "gt", "gte", "lt", "lte":
		op := map[string]string{"gt": ">", "gte": ">=", "lt": "<", "lte": "<="}[pf.Op]
		return fmt.Sprintf("(%s ~ '^-?[0-9]+(\\.[0-9]+)?$' AND (%s)::numeric %s $%d::numeric)",
			col, col, op, val(pf.Value)), nil
	default:
		return "", fmt.Errorf("不支持的运算符 %q", pf.Op)
	}
}

func quoteArg(args *[]any, v string) string {
	*args = append(*args, v)
	return fmt.Sprintf("$%d", len(*args))
}

// GetObject 返回对象详情:属性 + 出链/入链(带对端展示)+ 证据文档 ID 列表。
func (db *DB) GetObject(ctx context.Context, id string) (*OntObject, []OntLink, []OntLink, []string, error) {
	row := db.pool.QueryRow(ctx, `
		SELECT id, object_type, display_name, properties, version, created_at, updated_at
		FROM objects WHERE id = $1`, id)
	o, err := scanObject(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, nil, err
	}

	linkQuery := func(where string) ([]OntLink, error) {
		rows, err := db.pool.Query(ctx, `
			SELECT l.link_type, l.from_object, f.object_type, f.display_name,
			       l.to_object, t.object_type, t.display_name, l.document_id
			FROM links l
			JOIN objects f ON f.id = l.from_object
			JOIN objects t ON t.id = l.to_object
			WHERE `+where+` ORDER BY l.link_type`, id)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []OntLink
		for rows.Next() {
			var l OntLink
			if err := rows.Scan(&l.LinkType, &l.FromID, &l.FromType, &l.FromDisplay,
				&l.ToID, &l.ToType, &l.ToDisplay, &l.DocumentID); err != nil {
				return nil, err
			}
			out = append(out, l)
		}
		return out, rows.Err()
	}
	outLinks, err := linkQuery("l.from_object = $1")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	inLinks, err := linkQuery("l.to_object = $1")
	if err != nil {
		return nil, nil, nil, nil, err
	}

	rows, err := db.pool.Query(ctx, `
		SELECT document_id FROM object_evidence WHERE object_id = $1`, id)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	defer rows.Close()
	var docIDs []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, nil, nil, nil, err
		}
		docIDs = append(docIDs, d)
	}
	return o, outLinks, inLinks, docIDs, rows.Err()
}

// ListObjectsByDocument 返回某文档物化出的全部对象(文档/审核页联动用)。
func (db *DB) ListObjectsByDocument(ctx context.Context, docID string) ([]OntObject, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT o.id, o.object_type, o.display_name, o.properties, o.version, o.created_at, o.updated_at
		FROM object_evidence e JOIN objects o ON o.id = e.object_id
		WHERE e.document_id = $1 ORDER BY o.object_type, o.display_name`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanObjects(rows)
}

// ListSavedDocuments 返回全部已入库文档(本体全量重建用,不设上限、按时间正序重放)。
func (db *DB) ListSavedDocuments(ctx context.Context) ([]*model.Document, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, doc_type, filename, object_key, content_text, status, error, fields, issues, uploaded_by, reviewed_by, created_at, updated_at
		FROM documents WHERE status = 'saved' ORDER BY created_at`)
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

/* ---------- 扫描 ---------- */

type rowScanner interface{ Scan(dest ...any) error }

func scanObject(r rowScanner) (*OntObject, error) {
	var o OntObject
	var props []byte
	if err := r.Scan(&o.ID, &o.Type, &o.DisplayName, &props, &o.Version, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(props, &o.Properties); err != nil {
		return nil, err
	}
	return &o, nil
}

func scanObjects(rows pgx.Rows) ([]OntObject, error) {
	var out []OntObject
	for rows.Next() {
		o, err := scanObject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}
