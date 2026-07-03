-- 文档主表。提取字段与校验问题以 JSONB 存储,
-- 以承载"单据类型/字段可配置"的需求(不同单据字段不同,不适合固定列)。
CREATE TABLE IF NOT EXISTS documents (
    id           UUID PRIMARY KEY,
    doc_type     TEXT        NOT NULL,
    filename     TEXT        NOT NULL DEFAULT '',
    object_key   TEXT        NOT NULL DEFAULT '',
    content_text TEXT        NOT NULL DEFAULT '',
    status       TEXT        NOT NULL,
    error        TEXT        NOT NULL DEFAULT '',
    fields       JSONB       NOT NULL DEFAULT '[]',
    issues       JSONB       NOT NULL DEFAULT '[]',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_documents_doc_type ON documents (doc_type);
CREATE INDEX IF NOT EXISTS idx_documents_status   ON documents (status);
CREATE INDEX IF NOT EXISTS idx_documents_fields   ON documents USING GIN (fields);
