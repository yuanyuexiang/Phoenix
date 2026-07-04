// workflow 服务 REST API 的数据类型(与 backend/internal/api 对齐)。

export interface Field {
  name: string;
  value: string;
  confidence: number;
}

export interface Issue {
  field: string;
  rule: string;
  message: string;
}

export interface Doc {
  id: string;
  doc_type: string;
  filename: string;
  status: string;
  error?: string;
  fields?: Field[];
  issues?: Issue[];
  created_at?: string;
}

export interface FieldRule {
  required?: boolean;
  pattern?: string;
  enum?: string[];
}

export interface FieldSpec {
  name: string;
  label: string;
  description?: string;
  aliases?: string[];
  rule?: FieldRule;
}

export interface DocType {
  name: string;
  title: string;
  description?: string;
  fields: FieldSpec[];
}

export interface QueryResult {
  total: number;
  documents: Doc[];
}

export interface Component {
  name: string;
  ok: boolean;
  latency_ms: number;
  error?: string;
}

export const STATUS_META: Record<string, { text: string; tone: "gray" | "blue" | "green" | "amber" | "red" }> = {
  uploaded: { text: "已上传", tone: "gray" },
  extracted: { text: "已提取", tone: "blue" },
  validated: { text: "校验通过", tone: "green" },
  needs_review: { text: "待人工审核", tone: "amber" },
  saved: { text: "已入库", tone: "green" },
  failed: { text: "失败", tone: "red" },
};
