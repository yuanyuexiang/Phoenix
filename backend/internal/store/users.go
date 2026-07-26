package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// User 是员工账号(/pub/v1 登录与身份来源;管理见 /api/users)。
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	DisplayName  string    `json:"display_name"`
	Email        string    `json:"email"`
	Disabled     bool      `json:"disabled"`
	TokenVersion int       `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// ErrUsernameTaken 表示用户名已存在。
var ErrUsernameTaken = errors.New("用户名已存在")

// CreateUser 新建员工账号;用户名重复返回 ErrUsernameTaken。
func (db *DB) CreateUser(ctx context.Context, u *User) error {
	err := db.pool.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, display_name, email)
		VALUES ($1, $2, $3, $4)
		RETURNING id, token_version, created_at`,
		u.Username, u.PasswordHash, u.DisplayName, u.Email,
	).Scan(&u.ID, &u.TokenVersion, &u.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		return ErrUsernameTaken
	}
	return err
}

// GetUserByUsername 按登录名取账号;不存在返回 (nil, nil)。
func (db *DB) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	u := &User{}
	err := db.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, display_name, email, disabled, token_version, created_at
		FROM users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Email, &u.Disabled, &u.TokenVersion, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ListUsers 返回全部员工账号(按用户名排序,不含口令哈希以外的敏感数据)。
func (db *DB) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := db.pool.Query(ctx, `
		SELECT id, username, display_name, email, disabled, created_at
		FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Disabled, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUserProfile 更新展示名/邮箱(不触碰凭证,不影响已签发 token)。
func (db *DB) UpdateUserProfile(ctx context.Context, id int64, displayName, email string) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE users SET display_name = $2, email = $3, updated_at = now() WHERE id = $1`,
		id, displayName, email)
	return err
}

// SetUserDisabled 启用/禁用账号;禁用时 token_version+1,已签发 token 立即失效。
func (db *DB) SetUserDisabled(ctx context.Context, id int64, disabled bool) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE users SET disabled = $2,
		       token_version = token_version + CASE WHEN $2 THEN 1 ELSE 0 END,
		       updated_at = now()
		WHERE id = $1`, id, disabled)
	return err
}

// SetUserPassword 重置口令;token_version+1,旧 token 全部失效。
func (db *DB) SetUserPassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := db.pool.Exec(ctx, `
		UPDATE users SET password_hash = $2, token_version = token_version + 1, updated_at = now()
		WHERE id = $1`, id, passwordHash)
	return err
}
