// workflow 服务 REST API 的数据类型(与 backend/internal/api 对齐)。

export interface Field {
  name: string;
  value: string;
  confidence?: number;
  evidence?: FieldEvidence;
}

export interface FieldEvidence {
  raw_text?: string;
  page?: number;
  region?: string;
  value_source?: "document" | "calculated" | "manual";
  formula?: string;
  notes?: string;
  candidates?: { value: string; confidence?: number; raw_text?: string }[];
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
  uploaded_by?: string;
  reviewed_by?: string;
  created_at?: string;
  ontology?: OntologySummary; // save 响应附带的本体物化摘要
}

/* ---------- 本体层(对象/关系,configs/ontology) ---------- */

export interface OntologyType {
  name: string;
  title: string;
}

export interface OntObject {
  id: string;
  object_type: string;
  display_name: string;
  properties: Record<string, unknown>;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface OntLink {
  link_type: string;
  link_title?: string;
  from_id: string;
  from_type: string;
  from_display: string;
  to_id: string;
  to_type: string;
  to_display: string;
  document_id: string;
}

export interface ObjectDetail {
  object: OntObject;
  type_title: string;
  links_out: OntLink[];
  links_in: OntLink[];
  documents: Doc[];
}

export interface OntGraph {
  center: string;
  nodes: OntObject[];
  edges: OntLink[];
  truncated: boolean;
}

export interface OntologySummary {
  objects?: { type: string; title: string; id: string; display: string; is_new: boolean }[];
  warnings?: string[];
}

export interface FieldRule {
  required?: boolean;
  pattern?: string;
  enum?: string[];
}

export interface FieldSpec {
  name: string;
  label: string;
  type?: string; // number | date(归一化落库)
  description?: string;
  aliases?: string[];
  rule?: FieldRule;
}

export interface DocType {
  name: string;
  title: string;
  description?: string;
  /** 本体映射:该类型入库后物化的对象与主体字段(本体层启用时返回) */
  ontology?: {
    objects: OntologyType[];
    entities: Record<string, OntologyType>;
  };
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

/** 员工账号(/pub/v1 登录凭证来源,管理见「员工」页)。 */
export interface AppUser {
  id: number;
  username: string;
  display_name: string;
  email: string;
  disabled: boolean;
  created_at: string;
}

/** 特殊单据类型(不在 doctypes 配置内)的展示名。 */
export const DOCTYPE_SPECIAL: Record<string, string> = {
  auto: "待识别",
  unknown: "未识别",
};

export const STATUS_META: Record<string, { text: string; tone: "gray" | "blue" | "green" | "amber" | "red" }> = {
  uploaded: { text: "已上传", tone: "gray" },
  extracted: { text: "已提取", tone: "blue" },
  validated: { text: "校验通过", tone: "green" },
  needs_review: { text: "待人工审核", tone: "amber" },
  saved: { text: "已入库", tone: "green" },
  failed: { text: "失败", tone: "red" },
};
