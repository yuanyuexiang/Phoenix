// workflow REST API 调用封装。开发期经 next dev rewrites、生产经 nginx 反代到 workflow。
// 所有请求自动携带访问密钥;收到 401 时清掉本地密钥并跳回登录页。
import { authHeaders, clearAccessKey } from "./auth";
import type { AppUser, Component, Doc, DocType, Field, ObjectDetail, OntObject, OntologyType, QueryResult } from "./types";

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

// 对人工编辑后的字段做规则校验(后端不再抽取,字段由人工/WorkBuddy 提供)。
export const validateDocument = (id: string, fields: Field[], docType?: string) =>
  post<Doc>(`/api/documents/${id}/validate`, { fields, doc_type: docType });

export const saveDocument = (
  id: string,
  body: { fields?: Field[]; content_text?: string; doc_type?: string; force?: boolean },
) => post<Doc>(`/api/documents/${id}/save`, body);

// 删除文档:结构化数据 + 知识库切片 + 归档原件一并清除。
export const deleteDocument = (id: string) =>
  request<{ ok: boolean }>(`/api/documents/${id}`, { method: "DELETE" });

export const fetchStatus = () => request<{ components: Component[] }>("/api/status");

/* ---------- 本体层(对象/关系;configs/ontology) ---------- */

export const listOntologyTypes = () => request<{ types: OntologyType[] }>("/api/ontology/types");

export const queryObjects = (params: Record<string, string>) =>
  request<{ total: number; objects: OntObject[] }>("/api/objects?" + new URLSearchParams(params));

export const getObject = (id: string) => request<ObjectDetail>(`/api/objects/${id}`);

export const getDocumentObjects = (docID: string) =>
  request<{ objects: OntObject[] }>(`/api/documents/${docID}/objects`);

// 全量重建对象层(本体 YAML 大改后使用;文档层不受影响)
export const rebuildOntology = () =>
  post<{ documents: number; warnings: string[] }>("/api/ontology/rebuild", {});

/* ---------- 员工账号管理(/pub/v1 登录凭证来源) ---------- */

export const listUsers = () => request<{ total: number; users: AppUser[] }>("/api/users");

export const createUser = (body: { username: string; password: string; display_name?: string; email?: string }) =>
  post<AppUser>("/api/users", body);

export const updateUser = (id: number, body: { display_name?: string; email?: string; disabled?: boolean }) =>
  request<AppUser>(`/api/users/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

export const resetUserPassword = (id: number, password: string) =>
  post<{ ok: boolean }>(`/api/users/${id}/password`, { password });
