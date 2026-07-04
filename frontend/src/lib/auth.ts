// 访问密钥管理(参考 Atlas 的 entry-gate 模式):
// localStorage 存密钥,API 请求带 X-Access-Key,401 时回登录页。

const KEY = "phx-access-key";

export function getAccessKey(): string | null {
  if (typeof window === "undefined") return null;
  try {
    return localStorage.getItem(KEY);
  } catch {
    return null;
  }
}

export function setAccessKey(key: string) {
  try {
    localStorage.setItem(KEY, key);
  } catch {}
}

export function clearAccessKey() {
  try {
    localStorage.removeItem(KEY);
  } catch {}
}

/** 把访问密钥拼进请求头(有才拼)。 */
export function authHeaders(extra?: Record<string, string>): Record<string, string> {
  const k = getAccessKey();
  return { ...(extra ?? {}), ...(k ? { "X-Access-Key": k } : {}) };
}

/** 后端是否要求访问密钥。 */
export async function isAuthRequired(): Promise<boolean> {
  try {
    const res = await fetch("/api/auth/status", { cache: "no-store" });
    if (!res.ok) return false;
    const body = (await res.json()) as { required: boolean };
    return !!body.required;
  } catch {
    return false;
  }
}

/** 用给定密钥向后端验证;正确返回 true。 */
export async function verifyAccessKey(key: string): Promise<boolean> {
  try {
    const res = await fetch("/api/auth/check", {
      cache: "no-store",
      headers: { "X-Access-Key": key },
    });
    return res.ok;
  } catch {
    return false;
  }
}

/** 退出:清掉本地密钥并回登录页。 */
export function logout() {
  clearAccessKey();
  if (typeof window !== "undefined") window.location.href = "/login";
}
