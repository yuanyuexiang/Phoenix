-- RAG 知识库:document_chunks 存文档正文切片与 embedding 向量,供语义检索(ask_document)。
-- 注意:迁移每次启动全量重放,所有语句必须幂等。
-- 需要 postgres 镜像预装 pgvector 扩展(pgvector/pgvector:pg16);向量维度 1024 随 embedding 模型固化,
-- 换模型改维度需新迁移。

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS document_chunks (
    id          BIGSERIAL PRIMARY KEY,
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    chunk_index INT  NOT NULL,
    content     TEXT NOT NULL,
    embedding   vector(1024) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (document_id, chunk_index)
);

CREATE INDEX IF NOT EXISTS idx_chunks_document ON document_chunks (document_id);
CREATE INDEX IF NOT EXISTS idx_chunks_embedding ON document_chunks
    USING hnsw (embedding vector_cosine_ops);
