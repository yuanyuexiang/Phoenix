-- 本体层(Ontology,V1.4):对象/归一键/链接/证据。文档层是权威,本体层可全量重建。
-- 注意:迁移在每次启动时全量重放(无版本表),所有语句必须幂等。

CREATE TABLE IF NOT EXISTS objects (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    object_type  TEXT NOT NULL,               -- company / invoice / ...(configs/ontology)
    display_name TEXT NOT NULL DEFAULT '',
    properties   JSONB NOT NULL DEFAULT '{}', -- 已按声明类型归一化的值
    version      INT NOT NULL DEFAULT 1,      -- 每次物化更新 +1
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_objects_type  ON objects (object_type);
CREATE INDEX IF NOT EXISTS idx_objects_props ON objects USING gin (properties);

-- 归一键:实体解析核心。key_hash = 类型|键名|归一化键值
CREATE TABLE IF NOT EXISTS object_keys (
    object_id UUID NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
    key_hash  TEXT NOT NULL,
    UNIQUE (key_hash)
);
CREATE INDEX IF NOT EXISTS idx_object_keys_obj ON object_keys (object_id);

CREATE TABLE IF NOT EXISTS links (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    link_type   TEXT NOT NULL,               -- seller / includes / party_a ...
    from_object UUID NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
    to_object   UUID NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
    document_id UUID NOT NULL,               -- 溯源:文档修正/删除时按此精确撤销
    UNIQUE (link_type, from_object, to_object, document_id)
);
CREATE INDEX IF NOT EXISTS idx_links_doc  ON links (document_id);
CREATE INDEX IF NOT EXISTS idx_links_from ON links (from_object);
CREATE INDEX IF NOT EXISTS idx_links_to   ON links (to_object);

-- 证据:对象来自哪些文档
CREATE TABLE IF NOT EXISTS object_evidence (
    object_id   UUID NOT NULL REFERENCES objects(id) ON DELETE CASCADE,
    document_id UUID NOT NULL,
    UNIQUE (object_id, document_id)
);
CREATE INDEX IF NOT EXISTS idx_evidence_doc ON object_evidence (document_id);
