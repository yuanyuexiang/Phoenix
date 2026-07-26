package workflowapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuanyuexiang/phoenix/internal/store"
	"github.com/yuanyuexiang/phoenix/internal/userauth"
)

// 员工账号管理(管理后台「员工」页;/pub/v1 的登录凭证来源)。
// 路由(均要求 X-Access-Key):
//
//	GET    /api/users               列表
//	POST   /api/users               新建 {username,password,display_name,email}
//	PATCH  /api/users/{id}          更新 {display_name,email} 与/或 {disabled}
//	POST   /api/users/{id}/password 重置口令 {password}(旧 token 全部失效)

var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)

const minPasswordLen = 6

func (s *server) usersList(w http.ResponseWriter, r *http.Request) {
	users, err := s.opts.DB.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查询员工失败: "+err.Error())
		return
	}
	if users == nil {
		users = []store.User{}
	}
	writeJSON(w, map[string]any{"total": len(users), "users": users})
}

func (s *server) usersCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	req.Username = strings.TrimSpace(strings.ToLower(req.Username))
	if !usernameRe.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "用户名需为 2-64 位小写字母/数字/._-,且以字母或数字开头")
		return
	}
	if len(req.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "口令至少 6 位")
		return
	}
	u := &store.User{
		Username:     req.Username,
		PasswordHash: userauth.HashPassword(req.Password),
		DisplayName:  strings.TrimSpace(req.DisplayName),
		Email:        strings.TrimSpace(req.Email),
	}
	if err := s.opts.DB.CreateUser(r.Context(), u); err != nil {
		if errors.Is(err, store.ErrUsernameTaken) {
			writeError(w, http.StatusConflict, "用户名已存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "创建员工失败: "+err.Error())
		return
	}
	s.audit(r, "user_create", "", map[string]any{"username": u.Username})
	writeJSON(w, u)
}

func (s *server) usersUpdate(w http.ResponseWriter, r *http.Request) {
	id, u, ok := s.userByPath(w, r)
	if !ok {
		return
	}
	var req struct {
		DisplayName *string `json:"display_name"`
		Email       *string `json:"email"`
		Disabled    *bool   `json:"disabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if req.DisplayName != nil || req.Email != nil {
		display, email := u.DisplayName, u.Email
		if req.DisplayName != nil {
			display = strings.TrimSpace(*req.DisplayName)
		}
		if req.Email != nil {
			email = strings.TrimSpace(*req.Email)
		}
		if err := s.opts.DB.UpdateUserProfile(r.Context(), id, display, email); err != nil {
			writeError(w, http.StatusInternalServerError, "更新员工失败: "+err.Error())
			return
		}
	}
	if req.Disabled != nil {
		if err := s.opts.DB.SetUserDisabled(r.Context(), id, *req.Disabled); err != nil {
			writeError(w, http.StatusInternalServerError, "更新启用状态失败: "+err.Error())
			return
		}
		s.audit(r, "user_update", "", map[string]any{"username": u.Username, "disabled": *req.Disabled})
	}
	fresh, err := s.opts.DB.GetUserByUsername(r.Context(), u.Username)
	if err != nil || fresh == nil {
		writeError(w, http.StatusInternalServerError, "回读员工失败")
		return
	}
	writeJSON(w, fresh)
}

func (s *server) usersResetPassword(w http.ResponseWriter, r *http.Request) {
	id, u, ok := s.userByPath(w, r)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "口令至少 6 位")
		return
	}
	if err := s.opts.DB.SetUserPassword(r.Context(), id, userauth.HashPassword(req.Password)); err != nil {
		writeError(w, http.StatusInternalServerError, "重置口令失败: "+err.Error())
		return
	}
	s.audit(r, "user_password", "", map[string]any{"username": u.Username})
	writeJSON(w, map[string]any{"ok": true})
}

// userByPath 按 {id} 载入账号;id 非法或不存在时写错误响应并返回 ok=false。
func (s *server) userByPath(w http.ResponseWriter, r *http.Request) (int64, *store.User, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "非法的员工 ID")
		return 0, nil, false
	}
	users, err := s.opts.DB.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查询员工失败: "+err.Error())
		return 0, nil, false
	}
	for i := range users {
		if users[i].ID == id {
			return id, &users[i], true
		}
	}
	writeError(w, http.StatusNotFound, "员工不存在")
	return 0, nil, false
}
