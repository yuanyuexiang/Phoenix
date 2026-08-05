package store

import (
	"context"
	"time"
)

// Dashboard 汇总管理工作台所需的业务指标。周期统计按文档 created_at 计算，
// 待办统计采用当前全局积压，避免旧的未处理事项因时间范围而消失。
type Dashboard struct {
	RangeDays int                `json:"range_days"`
	Summary   DashboardSummary   `json:"summary"`
	Trends    []DashboardTrend   `json:"trends"`
	DocTypes  []DashboardDocType `json:"doctype_stats"`
	WorkItems []DashboardWork    `json:"work_items"`
	Activity  []DashboardEvent   `json:"recent_activity"`
}

type DashboardSummary struct {
	Uploaded       int     `json:"uploaded"`
	Saved          int     `json:"saved"`
	NeedsReview    int     `json:"needs_review"`
	Failed         int     `json:"failed"`
	SaveRate       float64 `json:"save_rate"`
	ObjectsChanged int     `json:"objects_changed"`
	ObjectsTotal   int     `json:"objects_total"`
}

type DashboardTrend struct {
	Date        string `json:"date"`
	Uploaded    int    `json:"uploaded"`
	Saved       int    `json:"saved"`
	NeedsReview int    `json:"needs_review"`
	Failed      int    `json:"failed"`
}

type DashboardDocType struct {
	DocType     string  `json:"doc_type"`
	Total       int     `json:"total"`
	Saved       int     `json:"saved"`
	NeedsReview int     `json:"needs_review"`
	Failed      int     `json:"failed"`
	SaveRate    float64 `json:"save_rate"`
}

type DashboardWork struct {
	ID         string    `json:"id"`
	Filename   string    `json:"filename"`
	DocType    string    `json:"doc_type"`
	Status     string    `json:"status"`
	UploadedBy string    `json:"uploaded_by"`
	Issue      string    `json:"issue"`
	CreatedAt  time.Time `json:"created_at"`
}

type DashboardEvent struct {
	ID         int64     `json:"id"`
	Action     string    `json:"action"`
	Actor      string    `json:"actor"`
	DocumentID string    `json:"document_id,omitempty"`
	Filename   string    `json:"filename,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

func normalizeDashboardDays(days int) int {
	if days != 30 {
		return 7
	}
	return days
}

// DashboardStats 一次读取工作台聚合。各查询彼此独立且只读，任一失败即返回错误，
// 避免前端同时展示来自不同时间点的半套统计。
func (db *DB) DashboardStats(ctx context.Context, days int) (*Dashboard, error) {
	days = normalizeDashboardDays(days)
	out := &Dashboard{RangeDays: days, Trends: []DashboardTrend{}, DocTypes: []DashboardDocType{}, WorkItems: []DashboardWork{}, Activity: []DashboardEvent{}}
	interval := time.Duration(days) * 24 * time.Hour
	since := time.Now().Add(-interval)

	err := db.pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'saved'),
		       count(*) FILTER (WHERE status = 'failed')
		FROM documents WHERE created_at >= $1`, since,
	).Scan(&out.Summary.Uploaded, &out.Summary.Saved, &out.Summary.Failed)
	if err != nil {
		return nil, err
	}
	if out.Summary.Uploaded > 0 {
		out.Summary.SaveRate = float64(out.Summary.Saved) / float64(out.Summary.Uploaded)
	}
	if err := db.pool.QueryRow(ctx, `SELECT count(*) FROM documents WHERE status = 'needs_review'`).Scan(&out.Summary.NeedsReview); err != nil {
		return nil, err
	}
	if err := db.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE updated_at >= $1), count(*) FROM objects`, since,
	).Scan(&out.Summary.ObjectsChanged, &out.Summary.ObjectsTotal); err != nil {
		return nil, err
	}

	rows, err := db.pool.Query(ctx, `
		SELECT to_char(day, 'YYYY-MM-DD'),
		       count(d.id),
		       count(d.id) FILTER (WHERE d.status = 'saved'),
		       count(d.id) FILTER (WHERE d.status = 'needs_review'),
		       count(d.id) FILTER (WHERE d.status = 'failed')
		FROM generate_series(current_date - ($1::int - 1), current_date, interval '1 day') AS days(day)
		LEFT JOIN documents d ON d.created_at >= day AND d.created_at < day + interval '1 day'
		GROUP BY day ORDER BY day`, days)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v DashboardTrend
		if err := rows.Scan(&v.Date, &v.Uploaded, &v.Saved, &v.NeedsReview, &v.Failed); err != nil {
			rows.Close()
			return nil, err
		}
		out.Trends = append(out.Trends, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	rows, err = db.pool.Query(ctx, `
		SELECT doc_type, count(*),
		       count(*) FILTER (WHERE status = 'saved'),
		       count(*) FILTER (WHERE status = 'needs_review'),
		       count(*) FILTER (WHERE status = 'failed')
		FROM documents WHERE created_at >= $1
		GROUP BY doc_type ORDER BY count(*) DESC, doc_type`, since)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v DashboardDocType
		if err := rows.Scan(&v.DocType, &v.Total, &v.Saved, &v.NeedsReview, &v.Failed); err != nil {
			rows.Close()
			return nil, err
		}
		if v.Total > 0 {
			v.SaveRate = float64(v.Saved) / float64(v.Total)
		}
		out.DocTypes = append(out.DocTypes, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	rows, err = db.pool.Query(ctx, `
		SELECT id, filename, doc_type, status, uploaded_by,
		       COALESCE(issues->0->>'message', NULLIF(error, ''), ''), created_at
		FROM documents WHERE status IN ('needs_review', 'failed')
		ORDER BY CASE status WHEN 'failed' THEN 0 ELSE 1 END, created_at ASC LIMIT 8`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v DashboardWork
		if err := rows.Scan(&v.ID, &v.Filename, &v.DocType, &v.Status, &v.UploadedBy, &v.Issue, &v.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out.WorkItems = append(out.WorkItems, v)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	rows, err = db.pool.Query(ctx, `
		SELECT a.id, a.action, a.actor, COALESCE(a.document_id::text, ''),
		       COALESCE(d.filename, ''), a.occurred_at
		FROM audit_log a LEFT JOIN documents d ON d.id = a.document_id
		ORDER BY a.occurred_at DESC LIMIT 12`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v DashboardEvent
		if err := rows.Scan(&v.ID, &v.Action, &v.Actor, &v.DocumentID, &v.Filename, &v.OccurredAt); err != nil {
			return nil, err
		}
		out.Activity = append(out.Activity, v)
	}
	return out, rows.Err()
}
