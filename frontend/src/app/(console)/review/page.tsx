"use client";

import { Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "next/navigation";

import * as api from "@/lib/api";
import type { Doc, DocType, Field } from "@/lib/types";
import { DOCTYPE_SPECIAL } from "@/lib/types";
import { btnDangerCls, btnPrimaryCls, inputCls, StatusBadge, ToastProvider, useToast } from "@/components/ui";

export default function ReviewPage() {
  return (
    <ToastProvider>
      <Suspense>
        <ReviewView />
      </Suspense>
    </ToastProvider>
  );
}

/**
 * 三列审核台(外壳 NavRail 为第一列):
 * 中列 = 文档队列(待人工审核置顶),右区 = 字段编辑与流水线操作。
 */
function ReviewView() {
  const toast = useToast();
  const preselect = useSearchParams().get("doc");

  const [doctypes, setDoctypes] = useState<DocType[]>([]);
  const [docs, setDocs] = useState<Doc[]>([]);
  const [currentID, setCurrentID] = useState<string | null>(preselect);
  const [edited, setEdited] = useState<Record<string, string>>({});

  const fail = (e: unknown) => toast(e instanceof Error ? e.message : String(e), false);

  const load = useCallback(async () => {
    try {
      const res = await api.queryDocuments({ limit: "100" });
      setDocs(res.documents ?? []);
    } catch (e) {
      fail(e);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    api.listDocTypes().then(setDoctypes).catch(fail);
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [load]);

  const current = useMemo(() => docs.find((d) => d.id === currentID) ?? null, [docs, currentID]);

  const pending = docs.filter((d) => d.status === "needs_review");
  const others = docs.filter((d) => d.status !== "needs_review");

  const labelOf = (doc: Doc, name: string) =>
    doctypes.find((t) => t.name === doc.doc_type)?.fields.find((f) => f.name === name)?.label ?? name;

  const select = (id: string) => {
    setCurrentID(id);
    setEdited({});
  };

  const patchDoc = (doc: Doc) => {
    setDocs((list) => list.map((d) => (d.id === doc.id ? doc : d)));
    setEdited({});
  };

  const reviewedFields = (): Field[] =>
    (current?.fields ?? []).map((f) => {
      const v = edited[f.name];
      return v === undefined || v === f.value ? f : { name: f.name, value: v, confidence: 1.0 };
    });

  const act = async (fn: () => Promise<Doc>, tip: string) => {
    try {
      patchDoc(await fn());
      toast(tip);
    } catch (e) {
      fail(e);
    }
  };

  const remove = async (doc: Doc) => {
    if (!window.confirm(`确认删除「${doc.filename}」?将一并清除结构化数据、知识库切片与归档原件,不可恢复。`)) return;
    try {
      await api.deleteDocument(doc.id);
      setDocs((list) => list.filter((d) => d.id !== doc.id));
      setCurrentID(null);
      toast("已删除");
    } catch (e) {
      fail(e);
    }
  };

  return (
    <div className="flex min-h-0 flex-1 overflow-hidden">
      {/* 中列:文档队列 */}
      <aside className="flex w-[250px] shrink-0 flex-col border-r border-surface-300 bg-surface-0 md:w-[280px]">
        <div className="border-b border-surface-300 px-4 py-[15px]">
          <h1 className="text-base font-semibold text-ink-900">审核</h1>
          <p className="mt-0.5 text-xs text-ink-300">待人工审核 {pending.length} 件</p>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          <QueueSection title="待人工审核" docs={pending} currentID={currentID} onSelect={select} highlight />
          <QueueSection title="全部文档" docs={others} currentID={currentID} onSelect={select} />
        </div>
      </aside>

      {/* 右区:字段编辑 */}
      <div className="min-h-0 flex-1 overflow-y-auto">
        {!current ? (
          <div className="flex h-full items-center justify-center text-sm text-ink-300">
            从左侧选择一份文档开始审核
          </div>
        ) : (
          <div className="mx-auto max-w-[760px] p-6">
            <div className="mb-4 flex flex-wrap items-center gap-3">
              <h2 className="text-base font-semibold text-ink-900">{current.filename}</h2>
              <StatusBadge status={current.status} />
              <span className="text-xs text-ink-300">
                {DOCTYPE_SPECIAL[current.doc_type] ??
                  doctypes.find((t) => t.name === current.doc_type)?.title ??
                  current.doc_type}
              </span>
              {current.uploaded_by && <span className="text-xs text-ink-300">上传:{current.uploaded_by}</span>}
              {current.reviewed_by && <span className="text-xs text-ink-300">入库:{current.reviewed_by}</span>}
              <span className="text-xs text-ink-300">{current.created_at}</span>
            </div>

            {current.error && (
              <div className="mb-4 rounded-md border border-red-500/30 bg-red-100 px-4 py-3 text-sm text-red-700">
                {current.error}
              </div>
            )}

            {(current.issues ?? []).length > 0 && (
              <div className="mb-4 rounded-md border border-amber-500/30 bg-amber-100 px-4 py-3">
                <p className="mb-1 text-sm font-medium text-amber-700">校验问题</p>
                <ul className="list-disc pl-5 text-sm text-amber-700">
                  {current.issues!.map((i, idx) => (
                    <li key={idx}>{i.message}</li>
                  ))}
                </ul>
              </div>
            )}

            <div className="rounded-lg border border-surface-300 bg-surface-0 shadow-card">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-ink-300">
                    <th className="w-[190px] px-4 py-2.5 font-medium">字段</th>
                    <th className="px-4 py-2.5 font-medium">值(可修改)</th>
                    <th className="w-[80px] px-4 py-2.5 font-medium">置信度</th>
                  </tr>
                </thead>
                <tbody>
                  {(current.fields ?? []).length === 0 && (
                    <tr>
                      <td colSpan={3} className="px-4 py-8 text-center text-ink-300">
                        暂无字段(字段由 WorkBuddy 识别后回传)
                      </td>
                    </tr>
                  )}
                  {(current.fields ?? []).map((f) => (
                    <tr key={f.name} className="border-t border-surface-300/60">
                      <td className="px-4 py-2.5">
                        <span className="text-ink-700">{labelOf(current, f.name)}</span>
                        <span className="block text-xs text-ink-300">{f.name}</span>
                      </td>
                      <td className="px-4 py-2">
                        <input
                          className={`${inputCls} w-full`}
                          value={edited[f.name] ?? f.value}
                          onChange={(e) => setEdited((m) => ({ ...m, [f.name]: e.target.value }))}
                        />
                      </td>
                      <td className="px-4 py-2.5 text-xs text-ink-300">{(f.confidence ?? 0).toFixed(2)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="mt-4 flex flex-wrap gap-2">
              <button
                className={btnPrimaryCls}
                onClick={() =>
                  act(
                    () => api.saveDocument(current.id, { fields: reviewedFields(), doc_type: current.doc_type }),
                    current.status === "saved" ? "已保存更正" : "已入库",
                  )
                }
              >
                {current.status === "saved" ? "保存更正" : "保存并入库"}
              </button>
              {(current.issues?.length ?? 0) > 0 && (
                <button
                  className={btnDangerCls}
                  onClick={() =>
                    act(
                      () => api.saveDocument(current.id, { fields: reviewedFields(), doc_type: current.doc_type, force: true }),
                      "已强制入库",
                    )
                  }
                >
                  强制入库(忽略校验问题)
                </button>
              )}
              <button className={`${btnDangerCls} ml-auto`} onClick={() => remove(current)}>
                删除文档
              </button>
            </div>
            <p className="mt-2 text-xs text-ink-300">
              {current.status === "saved"
                ? "本文档已入库。修改字段后「保存更正」即更新入库数据。"
                : "「保存并入库」会先做规则校验:通过即入库(已入库);不通过会标记为待人工审核并列出问题,可修正后重试或强制入库。"}
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

function QueueSection({
  title,
  docs,
  currentID,
  onSelect,
  highlight,
}: {
  title: string;
  docs: Doc[];
  currentID: string | null;
  onSelect: (id: string) => void;
  highlight?: boolean;
}) {
  if (docs.length === 0 && !highlight) return null;
  return (
    <div className="mb-2">
      <p className="px-2 pb-1 pt-2 text-xs text-ink-300">{title}</p>
      {docs.length === 0 && <p className="px-2 pb-2 text-xs text-ink-300/70">(空)</p>}
      {docs.map((d) => {
        const on = d.id === currentID;
        return (
          <button
            key={d.id}
            onClick={() => onSelect(d.id)}
            className={`relative mb-0.5 block w-full rounded-md px-3 py-2 text-left transition-colors ${
              on ? "bg-accent-500/10" : "hover:bg-surface-100"
            }`}
          >
            {on && <span className="absolute inset-y-2 left-0 w-[2px] rounded-full bg-accent-500" />}
            <span className={`block truncate text-[13px] ${on ? "text-accent-700" : "text-ink-700"}`}>
              {d.filename}
            </span>
            <span className="mt-0.5 block">
              <StatusBadge status={d.status} />
            </span>
          </button>
        );
      })}
    </div>
  );
}
