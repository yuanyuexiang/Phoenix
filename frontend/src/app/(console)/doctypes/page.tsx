"use client";

import { useEffect, useState } from "react";

import * as api from "@/lib/api";
import type { DocType } from "@/lib/types";
import { PageHeader } from "@/components/ui";

/** 单据类型(schema)只读展示;配置文件在 backend/configs/doctypes/*.yaml。 */
export default function DoctypesPage() {
  const [doctypes, setDoctypes] = useState<DocType[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .listDocTypes()
      .then(setDoctypes)
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  return (
    <>
      <PageHeader title="单据类型" desc="每种单据要提取的字段、校验规则与入库后物化的实体(configs/doctypes 与 configs/ontology,新增类型无需改代码)" />
      <div className="min-h-0 flex-1 overflow-y-auto p-6">
        {error && (
          <div className="mb-4 rounded-md border border-red-500/30 bg-red-100 px-4 py-3 text-sm text-red-700">
            {error}
          </div>
        )}
        <div className="grid gap-4">
          {doctypes.map((dt) => (
            <div key={dt.name} className="rounded-lg border border-surface-300 bg-surface-0 shadow-card">
              <div className="border-b border-surface-300 px-5 py-3.5">
                <div className="flex items-baseline gap-2">
                  <h2 className="text-[15px] font-semibold text-ink-900">{dt.title}</h2>
                  <code className="text-xs text-ink-300">{dt.name}</code>
                </div>
                {dt.description && <p className="mt-1 text-xs text-ink-300">{dt.description}</p>}
                {dt.ontology && dt.ontology.objects.length > 0 && (
                  <p className="mt-1.5 text-xs text-ink-500">
                    入库后物化:
                    {dt.ontology.objects.map((o) => (
                      <span key={o.name} className="ml-1.5 rounded bg-accent-100 px-1.5 py-0.5 text-accent-700">
                        {o.title}对象
                      </span>
                    ))}
                    {Object.keys(dt.ontology.entities).length > 0 && (
                      <span className="ml-2 text-ink-300">
                        关联 {[...new Set(Object.values(dt.ontology.entities).map((e) => e.title))].join(" / ")}
                      </span>
                    )}
                  </p>
                )}
              </div>
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-ink-300">
                    <th className="px-5 py-2 font-medium">字段</th>
                    <th className="px-5 py-2 font-medium">文档中的叫法</th>
                    <th className="px-5 py-2 font-medium">校验规则</th>
                  </tr>
                </thead>
                <tbody>
                  {dt.fields.map((f) => (
                    <tr key={f.name} className="border-t border-surface-300/60">
                      <td className="px-5 py-2.5">
                        <span className="text-ink-700">{f.label}</span>
                        {f.type === "number" && (
                          <span className="ml-1.5 rounded bg-surface-100 px-1.5 py-0.5 text-xs text-ink-500">数值</span>
                        )}
                        {f.type === "date" && (
                          <span className="ml-1.5 rounded bg-surface-100 px-1.5 py-0.5 text-xs text-ink-500">日期</span>
                        )}
                        {dt.ontology?.entities[f.name] && (
                          <span className="ml-1.5 rounded bg-accent-100 px-1.5 py-0.5 text-xs text-accent-700">
                            → {dt.ontology.entities[f.name].title}
                          </span>
                        )}
                        <span className="block text-xs text-ink-300">{f.name}</span>
                      </td>
                      <td className="px-5 py-2.5 text-xs text-ink-500">
                        {[f.label, ...(f.aliases ?? [])].join(" / ")}
                      </td>
                      <td className="px-5 py-2.5">
                        <RuleBadges f={f} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))}
        </div>
      </div>
    </>
  );
}

function RuleBadges({ f }: { f: DocType["fields"][number] }) {
  const rule = f.rule ?? {};
  const badges: React.ReactNode[] = [];
  if (rule.required)
    badges.push(
      <span key="req" className="rounded bg-red-100 px-1.5 py-0.5 text-xs text-red-700">
        必填
      </span>,
    );
  if (rule.pattern)
    badges.push(
      <code key="pat" className="rounded bg-surface-100 px-1.5 py-0.5 text-xs text-ink-500">
        {rule.pattern}
      </code>,
    );
  if (rule.enum?.length)
    badges.push(
      <span key="enum" className="rounded bg-accent-100 px-1.5 py-0.5 text-xs text-accent-700">
        {rule.enum.join(" | ")}
      </span>,
    );
  if (badges.length === 0) return <span className="text-xs text-ink-300">—</span>;
  return <span className="flex flex-wrap gap-1.5">{badges}</span>;
}
