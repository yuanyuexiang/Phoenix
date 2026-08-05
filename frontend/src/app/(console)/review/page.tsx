"use client";

import { Suspense, useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";

import * as api from "@/lib/api";
import type { Doc, DocType, Field, Issue, OntObject } from "@/lib/types";
import { DOCTYPE_SPECIAL } from "@/lib/types";
import { btnCls, btnDangerCls, btnPrimaryCls, inputCls, StatusBadge, ToastProvider, useToast } from "@/components/ui";

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
  // check = 最近一次「重新校验」的预检结果(不落库);null 表示未预检。
  const [check, setCheck] = useState<{ ok: boolean; issues: Issue[] } | null>(null);
  // 当前文档物化出的本体对象(chips 联动;本体层未启用/未入库时为空)
  const [docObjects, setDocObjects] = useState<OntObject[]>([]);
  // 归档原件预览(blob URL);null=加载中/无选中,error=原件不可用
  const [preview, setPreview] = useState<{ url: string; contentType: string } | "error" | null>(null);

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

  useEffect(() => {
    setDocObjects([]);
    if (currentID) {
      api.getDocumentObjects(currentID).then((r) => setDocObjects(r.objects)).catch(() => {});
    }
  }, [currentID]);

  useEffect(() => {
    let revoked: string | null = null;
    setPreview(null);
    if (currentID) {
      api
        .fetchDocumentFile(currentID)
        .then((p) => {
          revoked = p.url;
          setPreview(p);
        })
        .catch(() => setPreview("error"));
    }
    return () => {
      if (revoked) URL.revokeObjectURL(revoked);
    };
  }, [currentID]);

  const pending = docs.filter((d) => d.status === "needs_review");
  const others = docs.filter((d) => d.status !== "needs_review");

  const labelOf = (doc: Doc, name: string) =>
    doctypes.find((t) => t.name === doc.doc_type)?.fields.find((f) => f.name === name)?.label ?? name;

  const select = (id: string) => {
    setCurrentID(id);
    setEdited({});
    setCheck(null);
  };

  const patchDoc = (doc: Doc) => {
    setDocs((list) => list.map((d) => (d.id === doc.id ? doc : d)));
    setEdited({});
    setCheck(null);
  };

  // 重新校验:纯预检,不落库。只提示当前编辑能否通过,不改文档状态(徽标只由入库改变)。
  const runCheck = async () => {
    if (!current) return;
    try {
      const r = await api.validateDocument(current.id, reviewedFields(), current.doc_type);
      const issues = r.issues ?? [];
      setCheck({ ok: issues.length === 0, issues });
      toast(issues.length === 0 ? "预检通过,可点「保存并入库」正式入库" : `预检发现 ${issues.length} 个问题(未入库,见下方)`);
    } catch (e) {
      fail(e);
    }
  };

  const reviewedFields = (): Field[] =>
    (current?.fields ?? []).map((f) => {
      const v = edited[f.name];
      return v === undefined || v === f.value
        ? f
        : {
            ...f,
            value: v,
            confidence: undefined,
            evidence: {
              ...f.evidence,
              value_source: "manual" as const,
              notes: f.evidence?.notes || "管理后台人工修正",
            },
          };
    });

  const act = async (fn: () => Promise<Doc>, tip: string) => {
    try {
      const doc = await fn();
      patchDoc(doc);
      toast(tip);
      // 入库后本体物化摘要:警告(如重复报销)醒目提示,并刷新对象 chips
      for (const w of doc.ontology?.warnings ?? []) toast(w, false);
      if (doc.ontology?.objects?.length) {
        api.getDocumentObjects(doc.id).then((r) => setDocObjects(r.objects)).catch(() => {});
      }
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
      <aside className="flex w-[230px] shrink-0 flex-col border-r border-surface-300 bg-surface-0 md:w-[260px] 2xl:w-[280px]">
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
          <div className="flex gap-4 p-4 sm:p-6 2xl:gap-6">
          <div className="min-w-0 max-w-[760px] flex-1">
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

            {docObjects.length > 0 && (
              <div className="mb-4 flex flex-wrap items-center gap-1.5">
                <span className="text-xs text-ink-300">关联对象:</span>
                {docObjects.map((o) => (
                  <Link
                    key={o.id}
                    href={`/objects?id=${o.id}`}
                    className="rounded-full bg-accent-100 px-2.5 py-0.5 text-xs text-accent-700 no-underline hover:bg-accent-500/20"
                    title="修正字段重新入库时,关联对象会同步更新"
                  >
                    {o.display_name}
                  </Link>
                ))}
              </div>
            )}

            {current.error && (
              <div className="mb-4 rounded-md border border-red-500/30 bg-red-100 px-4 py-3 text-sm text-red-700">
                {current.error}
              </div>
            )}

            {/* 预检结果优先展示(不落库);未预检时展示上次入库尝试留下的校验问题 */}
            {check ? (
              check.ok ? (
                <div className="mb-4 rounded-md border border-green-500/30 bg-green-100 px-4 py-3 text-sm text-green-700">
                  ✓ 预检通过(尚未入库)。点「保存并入库」正式写入。
                </div>
              ) : (
                <div className="mb-4 rounded-md border border-amber-500/30 bg-amber-100 px-4 py-3">
                  <p className="mb-1 text-sm font-medium text-amber-700">预检发现问题(尚未入库,修正后可重试,或强制入库)</p>
                  <ul className="list-disc pl-5 text-sm text-amber-700">
                    {check.issues.map((i, idx) => (
                      <li key={idx}>{i.message}</li>
                    ))}
                  </ul>
                </div>
              )
            ) : (
              (current.issues ?? []).length > 0 && (
                <div className="mb-4 rounded-md border border-amber-500/30 bg-amber-100 px-4 py-3">
                  <p className="mb-1 text-sm font-medium text-amber-700">校验问题</p>
                  <ul className="list-disc pl-5 text-sm text-amber-700">
                    {current.issues!.map((i, idx) => (
                      <li key={idx}>{i.message}</li>
                    ))}
                  </ul>
                </div>
              )
            )}

            <div className="overflow-hidden rounded-lg border border-surface-300 bg-surface-0 shadow-card">
              <div className="overflow-x-auto">
              <table className="min-w-[680px] w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-ink-300">
                    <th className="w-[190px] px-4 py-2.5 font-medium">字段</th>
                    <th className="px-4 py-2.5 font-medium">值(可修改)</th>
                    <th className="w-[170px] px-4 py-2.5 font-medium">证据</th>
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
                      <td className="px-4 py-2.5 text-xs text-ink-300">
                        <span className="block">
                          {f.evidence?.value_source === "manual"
                            ? "人工修正"
                            : f.evidence?.value_source === "calculated"
                              ? "计算得出"
                              : f.evidence
                                ? "原文提取"
                                : "未提供"}
                          {f.confidence ? ` · ${(f.confidence * 100).toFixed(0)}%` : ""}
                        </span>
                        {f.evidence?.raw_text && (
                          <span className="mt-0.5 block max-w-[220px] truncate" title={f.evidence.raw_text}>
                            “{f.evidence.raw_text}”
                          </span>
                        )}
                        {(f.evidence?.page || f.evidence?.region) && (
                          <span className="block text-[11px]">
                            {f.evidence.page ? `第${f.evidence.page}页` : ""}
                            {f.evidence.page && f.evidence.region ? " · " : ""}
                            {f.evidence.region}
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              </div>
            </div>

            <div className="mt-4 flex flex-wrap gap-2">
              <button className={btnCls} onClick={runCheck}>
                重新校验(预检,不入库)
              </button>
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
              {(check ? !check.ok : (current.issues?.length ?? 0) > 0) && (
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
              建议先「重新校验」预检(不入库,只看当前编辑能否通过),确认无误再「保存并入库」正式写入。
              {current.status === "saved" && " 本文档已入库,修改后「保存更正」即更新数据。"}
            </p>
          </div>

          {/* 原件对照:图片/PDF 内联预览,其余提供下载(核对字段无需切窗口) */}
          <aside className="hidden w-[44%] min-w-0 shrink-0 xl:block">
            <div className="sticky top-6">
              <div className="mb-2 flex items-center justify-between">
                <p className="text-xs font-medium text-ink-300">原件对照</p>
                {preview && preview !== "error" && (
                  <a
                    href={preview.url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-xs text-accent-500 no-underline hover:underline"
                  >
                    新窗口打开
                  </a>
                )}
              </div>
              {preview === null && <p className="text-xs text-ink-300">原件加载中…</p>}
              {preview === "error" && (
                <p className="rounded-md border border-surface-300 bg-surface-100 px-3 py-4 text-xs text-ink-300">
                  原件不可用(未归档或读取失败)
                </p>
              )}
              {preview && preview !== "error" && preview.contentType.startsWith("image/") && (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={preview.url}
                  alt={current.filename}
                  className="max-h-[80vh] w-full rounded-lg border border-surface-300 bg-surface-0 object-contain shadow-card"
                />
              )}
              {preview && preview !== "error" &&
                (preview.contentType.includes("pdf") || preview.contentType.startsWith("text/")) && (
                  <iframe
                    src={preview.url}
                    title={current.filename}
                    className="h-[80vh] w-full rounded-lg border border-surface-300 bg-surface-0 shadow-card"
                  />
                )}
              {preview && preview !== "error" &&
                !preview.contentType.startsWith("image/") &&
                !preview.contentType.includes("pdf") &&
                !preview.contentType.startsWith("text/") && (
                  <a href={preview.url} download={current.filename} className={`${btnCls} inline-block no-underline`}>
                    下载原件({current.filename})
                  </a>
                )}
            </div>
          </aside>
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
