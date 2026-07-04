"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";

import { getAccessKey, isAuthRequired, setAccessKey, verifyAccessKey } from "@/lib/auth";

/** 登录页:访问密码解锁(参考 Atlas entry-gate);后端未启用鉴权时直接放行。 */
export default function LoginPage() {
  const router = useRouter();
  const [pw, setPw] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // 已解锁 / 无需鉴权 → 直接进控制台
  useEffect(() => {
    let alive = true;
    (async () => {
      const required = await isAuthRequired();
      if (!alive) return;
      if (!required) return router.replace("/");
      const k = getAccessKey();
      if (k && (await verifyAccessKey(k)) && alive) router.replace("/");
    })();
    return () => {
      alive = false;
    };
  }, [router]);

  async function unlock(e: React.FormEvent) {
    e.preventDefault();
    const key = pw.trim();
    if (!key || busy) return;
    setBusy(true);
    setErr(null);
    const ok = await verifyAccessKey(key);
    setBusy(false);
    if (ok) {
      setAccessKey(key);
      router.push("/");
    } else {
      setErr("访问密码不正确");
      setPw("");
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <div className="w-full max-w-sm">
        {/* Phoenix 印记 */}
        <div className="mb-6 flex flex-col items-center gap-3">
          <span className="relative flex h-14 w-14 items-center justify-center">
            <span className="absolute inset-0 rounded-full border border-accent-500/50" />
            <span className="absolute inset-[6px] rounded-full border border-accent-500/25" />
            <span className="text-xl font-semibold text-accent-500">凤</span>
          </span>
          <div className="text-center">
            <h1 className="text-lg font-semibold text-ink-900">Phoenix 管理后台</h1>
            <p className="mt-1 text-xs text-ink-300">企业智能文档处理平台 · 人工审核 / 查询</p>
          </div>
        </div>

        <form
          onSubmit={unlock}
          className="rounded-xl border border-surface-300 bg-surface-0 p-5 shadow-card"
        >
          <label className="mb-1.5 block text-xs text-ink-500">访问密码</label>
          <div className="flex items-center gap-2 rounded-lg border border-surface-300 bg-surface-50 py-1.5 pl-3 pr-1.5 transition focus-within:border-accent-500 focus-within:ring-2 focus-within:ring-accent-500/15">
            {/* 锁图标 */}
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" className="h-4 w-4 shrink-0 text-ink-300">
              <rect x="5" y="10.5" width="14" height="9" rx="2" />
              <path d="M8 10.5V8a4 4 0 0 1 8 0v2.5" strokeLinecap="round" />
            </svg>
            <input
              type="password"
              value={pw}
              onChange={(e) => {
                setPw(e.target.value);
                setErr(null);
              }}
              autoFocus
              placeholder="输入访问密码"
              aria-label="访问密码"
              className="min-w-0 flex-1 bg-transparent py-1 text-sm text-ink-900 placeholder:text-ink-300 focus:outline-none"
            />
            <button
              type="submit"
              disabled={busy || !pw.trim()}
              className="inline-flex h-8 shrink-0 items-center rounded-md bg-accent-500 px-4 text-xs tracking-wider text-white transition-colors hover:bg-accent-700 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {busy ? "验证中…" : "解锁"}
            </button>
          </div>
          {err && <p className="mt-2 text-xs text-red-500">{err}</p>}
          <p className="mt-3 text-[11px] text-ink-300">
            管理后台仅限授权人员使用;访问密码由平台管理员配置(PHX_ADMIN_PASSWORD)。
          </p>
        </form>
      </div>
    </div>
  );
}
