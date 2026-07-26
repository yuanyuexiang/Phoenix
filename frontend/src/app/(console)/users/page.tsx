"use client";

import { useCallback, useEffect, useState } from "react";

import * as api from "@/lib/api";
import type { AppUser } from "@/lib/types";
import { btnCls, btnDangerCls, btnPrimaryCls, inputCls, PageHeader, ToastProvider, useToast } from "@/components/ui";

/** 员工账号管理:WorkBuddy「文档处理专家」经 /pub/v1 登录的凭证来源。 */
export default function UsersPage() {
  return (
    <ToastProvider>
      <UsersInner />
    </ToastProvider>
  );
}

function UsersInner() {
  const [users, setUsers] = useState<AppUser[]>([]);
  const [error, setError] = useState("");
  const toast = useToast();

  const reload = useCallback(() => {
    api
      .listUsers()
      .then((r) => setUsers(r.users))
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  useEffect(reload, [reload]);

  return (
    <>
      <PageHeader
        title="员工"
        desc="WorkBuddy「文档处理专家」的登录账号;每个操作按账号追溯到人。禁用或重置口令会使该员工的登录立即失效"
      />
      <div className="min-h-0 flex-1 overflow-y-auto p-6">
        {error && (
          <div className="mb-4 rounded-md border border-red-500/30 bg-red-100 px-4 py-3 text-sm text-red-700">
            {error}
          </div>
        )}

        <CreateForm onCreated={() => { reload(); toast("员工已创建"); }} />

        <div className="mt-5 rounded-lg border border-surface-300 bg-surface-0 shadow-card">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-ink-300">
                <th className="px-5 py-2.5 font-medium">用户名</th>
                <th className="px-5 py-2.5 font-medium">姓名</th>
                <th className="px-5 py-2.5 font-medium">邮箱</th>
                <th className="px-5 py-2.5 font-medium">状态</th>
                <th className="px-5 py-2.5 font-medium">创建时间</th>
                <th className="px-5 py-2.5 text-right font-medium">操作</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <UserRow key={u.id} user={u} onChanged={reload} />
              ))}
              {users.length === 0 && (
                <tr>
                  <td colSpan={6} className="px-5 py-10 text-center text-sm text-ink-300">
                    还没有员工账号 —— 在上方新建第一个,员工即可在 WorkBuddy 里登录使用
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}

/* ---------- 新建 ---------- */

function CreateForm({ onCreated }: { onCreated: () => void }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  const submit = async () => {
    if (!username || password.length < 6) {
      toast("用户名必填,口令至少 6 位", false);
      return;
    }
    setBusy(true);
    try {
      await api.createUser({ username, password, display_name: displayName, email });
      setUsername(""); setPassword(""); setDisplayName(""); setEmail("");
      onCreated();
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), false);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-lg border border-surface-300 bg-surface-0 p-4 shadow-card">
      <div className="mb-3 text-xs text-ink-300">新建员工账号(用户名:小写字母/数字/._-)</div>
      <div className="flex flex-wrap items-center gap-2">
        <input className={inputCls} placeholder="用户名 *" value={username} onChange={(e) => setUsername(e.target.value)} />
        <input className={inputCls} type="password" placeholder="初始口令(≥6位)*" value={password} onChange={(e) => setPassword(e.target.value)} />
        <input className={inputCls} placeholder="姓名" value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
        <input className={inputCls} placeholder="邮箱" value={email} onChange={(e) => setEmail(e.target.value)} />
        <button className={btnPrimaryCls} disabled={busy} onClick={submit}>
          新建
        </button>
      </div>
    </div>
  );
}

/* ---------- 行 ---------- */

function UserRow({ user, onChanged }: { user: AppUser; onChanged: () => void }) {
  const [resetting, setResetting] = useState(false);
  const [newPass, setNewPass] = useState("");
  const [busy, setBusy] = useState(false);
  const toast = useToast();

  const run = async (fn: () => Promise<unknown>, okMsg: string) => {
    setBusy(true);
    try {
      await fn();
      toast(okMsg);
      onChanged();
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), false);
    } finally {
      setBusy(false);
    }
  };

  return (
    <tr className={`border-t border-surface-300/60 ${user.disabled ? "opacity-60" : ""}`}>
      <td className="px-5 py-2.5 text-ink-900">{user.username}</td>
      <td className="px-5 py-2.5 text-ink-500">{user.display_name || "—"}</td>
      <td className="px-5 py-2.5 text-ink-500">{user.email || "—"}</td>
      <td className="px-5 py-2.5">
        {user.disabled ? (
          <span className="rounded-full bg-red-100 px-2.5 py-0.5 text-xs text-red-700">已禁用</span>
        ) : (
          <span className="rounded-full bg-green-100 px-2.5 py-0.5 text-xs text-green-700">启用中</span>
        )}
      </td>
      <td className="px-5 py-2.5 text-xs text-ink-300">{user.created_at?.slice(0, 16).replace("T", " ")}</td>
      <td className="px-5 py-2.5">
        <div className="flex items-center justify-end gap-2">
          {resetting ? (
            <>
              <input
                className={`${inputCls} w-40`}
                type="password"
                placeholder="新口令(≥6位)"
                value={newPass}
                onChange={(e) => setNewPass(e.target.value)}
                autoFocus
              />
              <button
                className={btnPrimaryCls}
                disabled={busy || newPass.length < 6}
                onClick={() =>
                  run(() => api.resetUserPassword(user.id, newPass), "口令已重置,旧登录已失效").then(() => {
                    setResetting(false);
                    setNewPass("");
                  })
                }
              >
                确定
              </button>
              <button className={btnCls} onClick={() => { setResetting(false); setNewPass(""); }}>
                取消
              </button>
            </>
          ) : (
            <>
              <button className={btnCls} disabled={busy} onClick={() => setResetting(true)}>
                重置口令
              </button>
              {user.disabled ? (
                <button
                  className={btnCls}
                  disabled={busy}
                  onClick={() => run(() => api.updateUser(user.id, { disabled: false }), "已启用")}
                >
                  启用
                </button>
              ) : (
                <button
                  className={btnDangerCls}
                  disabled={busy}
                  onClick={() => run(() => api.updateUser(user.id, { disabled: true }), "已禁用,该员工登录立即失效")}
                >
                  禁用
                </button>
              )}
            </>
          )}
        </div>
      </td>
    </tr>
  );
}
