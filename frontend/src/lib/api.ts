// workflow REST API 调用封装。开发期经 next dev rewrites、生产经 nginx 反代到 workflow。
// 所有请求自动携带访问密钥;收到 401 时清掉本地密钥并跳回登录页。
import { authHeaders, clearAccessKey } from "./auth";
import type { Component, Doc, DocType, Field, QueryResult } from "./types";

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(url, {
    ...init,
    headers: authHeaders(init?.headers as Record<string, string> | undefined),
  });
  if (resp.status === 401) {
    clearAccessKey();
    if (typeof window !== "undefined") window.location.href = "/login";
    throw new Error("未登录或登录已失效");
  }
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) {
    throw new Error((data as { error?: string }).error || `HTTP ${resp.status}`);
  }
  return data as T;
}

const post = <T,>(url: string, body: unknown): Promise<T> =>
  request<T>(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body ?? {}),
  });

export const listDocTypes = () => request<DocType[]>("/api/doctypes");

export const queryDocuments = (params: Record<string, string>) =>
  request<QueryResult>("/api/documents?" + new URLSearchParams(params));

export const uploadDocument = (docType: string, filename: string, contentText: string) =>
  post<Doc>("/api/documents", { doc_type: docType, filename, content_text: contentText });

export const extractDocument = (id: string) => post<Doc>(`/api/documents/${id}/extract`, {});

export const validateDocument = (id: string) => post<Doc>(`/api/documents/${id}/validate`, {});

export const saveDocument = (id: string, body: { fields?: Field[]; force?: boolean }) =>
  post<Doc>(`/api/documents/${id}/save`, body);

export const fetchStatus = () => request<{ components: Component[] }>("/api/status");
