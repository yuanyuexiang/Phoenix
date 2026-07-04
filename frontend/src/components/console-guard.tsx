"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";

import { getAccessKey, isAuthRequired } from "@/lib/auth";

/**
 * 控制台门卫:进入各页面前确认已解锁。
 * - 本地已有访问密钥 → 乐观放行(密钥失效时 API 会 401,api.ts 统一跳回 /login)。
 * - 没有密钥且后端要求鉴权 → 跳登录页,避免先发一串 401。
 * - 后端未启用密码 → 放行。
 */
export function ConsoleGuard({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [ok, setOk] = useState(false);

  useEffect(() => {
    if (getAccessKey()) {
      setOk(true);
      return;
    }
    let alive = true;
    (async () => {
      const required = await isAuthRequired();
      if (!alive) return;
      if (required) router.replace("/login");
      else setOk(true);
    })();
    return () => {
      alive = false;
    };
  }, [router]);

  if (!ok) {
    return (
      <div className="flex h-full flex-1 items-center justify-center">
        <div className="h-9 w-28 animate-pulse rounded-lg bg-surface-100" />
      </div>
    );
  }
  return <>{children}</>;
}
