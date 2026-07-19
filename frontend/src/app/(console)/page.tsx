"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import * as api from "@/lib/api";
import type { Doc, DocType } from "@/lib/types";
import { DOCTYPE_SPECIAL, STATUS_META } from "@/lib/types";
import { btnCls, inputCls, PageHeader, StatusBadge, ToastProvider, useToast } from "@/components/ui";

export default function DocumentsPage() {
  return (
    <ToastProvider>
      <DocumentsView />
    </ToastProvider>
  );
}

function DocumentsView() {
  const toast = useToast();
  const [doctypes, setDoctypes] = useState<DocType[]>([]);
  const [docs, setDocs] = useState<Doc[]>([]);
  const [loading, setLoading] = useState(false);
  const [filters, setFilters] = useState({ doc_type: "", status: "", keyword: "" });

  const fail = (e: unknown) => toast(e instanceof Error ? e.message : String(e), false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, string> = { limit: "50" };
      for (const [k, v] of Object.entries(filters)) if (v) params[k] = v;
      setDocs((await api.queryDocuments(params)).documents ?? []);
    } catch (e) {
      fail(e);
    } finally {
      setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters]);

  useEffect(() => {
    api.listDocTypes().then(setDoctypes).catch(fail);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const typeTitle = (name: string) =>
    DOCTYPE_SPECIAL[name] ?? doctypes.find((t) => t.name === name)?.title ?? name;

  return (
    <>
      <PageHeader title="文档" desc="WorkBuddy 处理的文档记录(上传与识别在 WorkBuddy 侧完成)" />

      <div className="min-h-0 flex-1 overflow-y-auto p-6">
        <div className="rounded-lg border border-surface-300 bg-surface-0 shadow-card">
          <div className="flex flex-wrap items-center gap-2 border-b border-surface-300 p-3">
            <select
              className={inputCls}
              value={filters.doc_type}
              onChange={(e) => setFilters((f) => ({ ...f, doc_type: e.target.value }))}
            >
              <option value="">全部类型</option>
              {doctypes.map((t) => (
                <option key={t.name} value={t.name}>
                  {t.title}
                </option>
              ))}
            </select>
            <select
              className={inputCls}
              value={filters.status}
              onChange={(e) => setFilters((f) => ({ ...f, status: e.target.value }))}
            >
              <option value="">全部状态</option>
              {Object.entries(STATUS_META).map(([value, meta]) => (
                <option key={value} value={value}>
                  {meta.text}
                </option>
              ))}
            </select>
            <input
              className={inputCls}
              style={{ width: 220 }}
              placeholder="关键词(文件名/正文),回车搜索"
              onKeyDown={(e) => {
                if (e.key === "Enter") setFilters((f) => ({ ...f, keyword: (e.target as HTMLInputElement).value }));
              }}
            />
            <button className={btnCls} onClick={load}>
              刷新
            </button>
          </div>

          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-ink-300">
                <th className="px-4 py-2.5 font-medium">文件名</th>
                <th className="px-4 py-2.5 font-medium">类型</th>
                <th className="px-4 py-2.5 font-medium">状态</th>
                <th className="px-4 py-2.5 font-medium">操作人</th>
                <th className="px-4 py-2.5 font-medium">创建时间</th>
                <th className="px-4 py-2.5" />
              </tr>
            </thead>
            <tbody>
              {docs.length === 0 && (
                <tr>
                  <td colSpan={6} className="px-4 py-10 text-center text-ink-300">
                    {loading ? "加载中…" : "暂无文档"}
                  </td>
                </tr>
              )}
              {docs.map((d) => (
                <tr key={d.id} className="border-t border-surface-300/60 transition-colors hover:bg-surface-100/60">
                  <td className="max-w-[280px] truncate px-4 py-2.5 text-ink-700">{d.filename}</td>
                  <td className="px-4 py-2.5 text-ink-500">{typeTitle(d.doc_type)}</td>
                  <td className="px-4 py-2.5">
                    <StatusBadge status={d.status} />
                  </td>
                  <td className="px-4 py-2.5 text-ink-500">{d.uploaded_by || "—"}</td>
                  <td className="px-4 py-2.5 text-xs text-ink-300">{d.created_at}</td>
                  <td className="px-4 py-2.5 text-right">
                    <Link
                      href={`/review?doc=${d.id}`}
                      className="text-sm text-accent-500 no-underline hover:text-accent-700"
                    >
                      查看/审核
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </>
  );
}
