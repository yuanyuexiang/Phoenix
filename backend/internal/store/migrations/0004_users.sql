-- 员工账号(自研账号体系,V1.3 取代 Keycloak):/pub/v1 登录与身份来源。
-- 注意:迁移在每次启动时全量重放(无版本表),所有语句必须幂等。

CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,        -- 登录名,也是 uploaded_by/reviewed_by 的落库口径
    password_hash TEXT NOT NULL,               -- pbkdf2$sha256$iter$salt$dk(见 internal/userauth)
    display_name  TEXT NOT NULL DEFAULT '',
    email         TEXT NOT NULL DEFAULT '',
    disabled      BOOLEAN NOT NULL DEFAULT FALSE,
    token_version INT NOT NULL DEFAULT 1,      -- 改密/禁用时 +1,已签发的 token 立即失效
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
