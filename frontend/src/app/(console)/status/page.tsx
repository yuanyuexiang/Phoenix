"use client";

import { useCallback, useEffect, useState } from "react";

import * as api from "@/lib/api";
import type { Component } from "@/lib/types";
import { btnCls, PageHeader } from "@/components/ui";

/** 服务状态:workflow 的 /api/status 聚合探测各组件,10 秒自动刷新。 */
export default function StatusPage() {
  const [components, setComponents] = useState<Component[]>([]);
  const [error, setError] = useState("");
  const [updatedAt, setUpdatedAt] = useState("");
  const [rebuildMsg, setRebuildMsg] = useState("");
  const [rebuilding, setRebuilding] = useState(false);

  const rebuild = async () => {
    if (!window.confirm("重建对象层?将清空全部对象/关系后按已入库文档重放(文档数据不受影响)。本体配置大改后使用。")) return;
    setRebuilding(true);
    setRebuildMsg("");
    try {
      const r = await api.rebuildOntology();
      setRebuildMsg(`重建完成:重放 ${r.documents} 份已入库文档${r.warnings.length ? `,${r.warnings.length} 条警告(详见 audit_log)` : ""}`);
    } catch (e) {
      setRebuildMsg("重建失败:" + (e instanceof Error ? e.message : String(e)));
    } finally {
      setRebuilding(false);
    }
  };

  const load = useCallback(async () => {
    try {
      const res = await api.fetchStatus();
      setComponents(res.components ?? []);
      setError("");
    } catch (e) {
      // workflow 本身不可达时,给出兜底展示
      setComponents([]);
      setError(e instanceof Error ? e.message : String(e));
    }
    setUpdatedAt(new Date().toLocaleTimeString());
  }, []);

  useEffect(() => {
    load();
    const timer = setInterval(load, 10_000);
    return () => clearInterval(timer);
  }, [load]);

  return (
    <>
      <PageHeader
        title="服务状态"
        desc={`各组件健康探测,10 秒自动刷新${updatedAt ? ` · 上次更新 ${updatedAt}` : ""}`}
        extra={
          <button className={btnCls} onClick={load}>
            立即刷新
          </button>
        }
      />
      <div className="min-h-0 flex-1 overflow-y-auto p-4 sm:p-6">
        {error && (
          <div className="mb-4 rounded-md border border-red-500/30 bg-red-100 px-4 py-3 text-sm text-red-700">
            workflow 工作流引擎不可达:{error}
          </div>
        )}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {components.map((c) => (
            <div key={c.name} className="rounded-lg border border-surface-300 bg-surface-0 p-4 shadow-card">
              <div className="flex items-center justify-between">
                <span className="text-sm text-ink-700">{c.name}</span>
                <span
                  className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs ${
                    c.ok ? "bg-green-100 text-green-700" : "bg-red-100 text-red-700"
                  }`}
                >
                  <span className={`h-1.5 w-1.5 rounded-full ${c.ok ? "bg-green-500" : "bg-red-500"}`} />
                  {c.ok ? "正常" : "异常"}
                </span>
              </div>
              <p className="mt-2 text-xs text-ink-300">
                延迟 {c.latency_ms}ms
                {c.error && <span className="mt-1 block break-all text-red-500">{c.error}</span>}
              </p>
            </div>
          ))}
        </div>
        <div className="mt-6 rounded-lg border border-surface-300 bg-surface-0 p-4 shadow-card">
          <div className="flex flex-wrap items-center gap-3">
            <div>
              <p className="text-sm text-ink-700">对象层维护</p>
              <p className="mt-0.5 text-xs text-ink-300">
                本体配置(configs/ontology)大改后,清空对象/关系并按已入库文档全量重放;文档数据不受影响
              </p>
            </div>
            <button className={`${btnCls} ml-auto`} disabled={rebuilding} onClick={rebuild}>
              {rebuilding ? "重建中…" : "重建对象层"}
            </button>
          </div>
          {rebuildMsg && <p className="mt-2 text-xs text-ink-500">{rebuildMsg}</p>}
        </div>
        <p className="mt-6 text-xs text-ink-300">
          说明:探测由 workflow 服务发起(数据库、对象存储);本页面自身不在探测范围。
          MinIO 连接状态包含在 workflow 启动检查中。
        </p>
      </div>
    </>
  );
}
