"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import * as api from "@/lib/api";
import type { Doc, DocType } from "@/lib/types";
import { DOCTYPE_SPECIAL, STATUS_META } from "@/lib/types";
import { btnCls, btnPrimaryCls, inputCls, PageHeader, StatusBadge, ToastProvider, useToast } from "@/components/ui";

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
  const [showUpload, setShowUpload] = useState(false);
  const [upload, setUpload] = useState({ doc_type: "auto", filename: "test.txt", content: "" });

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

  const doUpload = async () => {
    if (!upload.content.trim()) return toast("请粘贴文本内容", false);
    try {
      const doc = await api.uploadDocument(upload.doc_type, upload.filename || "test.txt", upload.content);
      toast(`上传成功:${doc.id.slice(0, 8)}…`);
      setUpload((u) => ({ ...u, content: "" }));
      load();
    } catch (e) {
      fail(e);
    }
  };

  const typeTitle = (name: string) =>
    DOCTYPE_SPECIAL[name] ?? doctypes.find((t) => t.name === name)?.title ?? name;

  return (
    <>
      <PageHeader
        title="文档"
        desc="已上传文档的处理进度与结果"
        extra={
          <button className={btnCls} onClick={() => setShowUpload((v) => !v)}>
            {showUpload ? "收起上传" : "上传测试文档"}
          </button>
        }
      />

      <div className="min-h-0 flex-1 overflow-y-auto p-6">
        {showUpload && (
          <div className="mb-4 rounded-lg border border-surface-300 bg-surface-0 p-4 shadow-card">
            <div className="flex flex-wrap items-center gap-2">
              <select
                className={inputCls}
                value={upload.doc_type}
                onChange={(e) => setUpload((u) => ({ ...u, doc_type: e.target.value }))}
              >
                <option value="auto">自动识别类型</option>
                {doctypes.map((t) => (
                  <option key={t.name} value={t.name}>
                    {t.title} ({t.name})
                  </option>
                ))}
              </select>
              <input
                className={inputCls}
                style={{ width: 180 }}
                value={upload.filename}
                onChange={(e) => setUpload((u) => ({ ...u, filename: e.target.value }))}
                placeholder="文件名"
              />
              <button className={btnPrimaryCls} onClick={doUpload}>
                上传
              </button>
            </div>
            <textarea
              className={`${inputCls} mt-3 w-full font-mono text-xs`}
              rows={5}
              placeholder={"编号: XXX-001\n标题: ……\n(演示用;WorkBuddy 侧走 MCP 的 file_url 上传)"}
              value={upload.content}
              onChange={(e) => setUpload((u) => ({ ...u, content: e.target.value }))}
            />
          </div>
        )}

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
                <th className="px-4 py-2.5 font-medium">创建时间</th>
                <th className="px-4 py-2.5" />
              </tr>
            </thead>
            <tbody>
              {docs.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-4 py-10 text-center text-ink-300">
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
