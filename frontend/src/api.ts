// workflow 服务 REST API 的类型与调用封装。
// 开发期经 vite 代理、生产经 nginx 代理到 workflow(见 vite.config.ts / nginx.conf)。

export interface Field {
  name: string
  value: string
  confidence: number
}

export interface Issue {
  field: string
  rule: string
  message: string
}

export interface Doc {
  id: string
  doc_type: string
  filename: string
  status: string
  error?: string
  fields?: Field[]
  issues?: Issue[]
  created_at?: string
}

export interface FieldSpec {
  name: string
  label: string
}

export interface DocType {
  name: string
  title: string
  description?: string
  fields: FieldSpec[]
}

export interface QueryResult {
  total: number
  documents: Doc[]
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(url, init)
  const data = await resp.json().catch(() => ({}))
  if (!resp.ok) {
    throw new Error((data as { error?: string }).error || `HTTP ${resp.status}`)
  }
  return data as T
}

const post = <T,>(url: string, body: unknown): Promise<T> =>
  request<T>(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body ?? {}),
  })

export const listDocTypes = () => request<DocType[]>('/api/doctypes')

export const queryDocuments = (params: Record<string, string>) =>
  request<QueryResult>('/api/documents?' + new URLSearchParams(params))

export const uploadDocument = (docType: string, filename: string, contentText: string) =>
  post<Doc>('/api/documents', { doc_type: docType, filename, content_text: contentText })

export const extractDocument = (id: string) => post<Doc>(`/api/documents/${id}/extract`, {})

export const validateDocument = (id: string) => post<Doc>(`/api/documents/${id}/validate`, {})

export const saveDocument = (id: string, body: { fields?: Field[]; force?: boolean }) =>
  post<Doc>(`/api/documents/${id}/save`, body)
