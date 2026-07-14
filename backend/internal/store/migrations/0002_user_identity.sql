-- 操作人身份(MCP OAuth 方案,docs/MCP-OAuth鉴权方案.md §4)。
-- 注意:迁移在每次启动时全量重放(无版本表),所有语句必须幂等。

-- uploaded_by / reviewed_by 存展示口径(username → email → sub,或后台的 'admin')。
ALTER TABLE documents ADD COLUMN IF NOT EXISTS uploaded_by TEXT NOT NULL DEFAULT '';
ALTER TABLE documents ADD COLUMN IF NOT EXISTS reviewed_by TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_documents_uploaded_by ON documents (uploaded_by);

-- 审计日志:每个写动作(upload/extract/validate/save)一条,detail 存全量身份 claims 兜底。
CREATE TABLE IF NOT EXISTS audit_log (
    id           BIGSERIAL PRIMARY KEY,
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor        TEXT NOT NULL DEFAULT '',   -- 展示口径,同 uploaded_by
    actor_source TEXT NOT NULL DEFAULT '',   -- oauth | admin | anonymous
    action       TEXT NOT NULL,              -- upload | extract | validate | save
    document_id  UUID,
    detail       JSONB NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_audit_document ON audit_log (document_id);
CREATE INDEX IF NOT EXISTS idx_audit_actor    ON audit_log (actor);
