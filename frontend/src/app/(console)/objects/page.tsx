"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";

import * as api from "@/lib/api";
import type { ObjectDetail, OntObject, OntologyType } from "@/lib/types";
import { btnCls, inputCls, PageHeader, StatusBadge, ToastProvider, useToast } from "@/components/ui";

/** 本体对象页:跨文档归一后的实体(公司/发票/合同/报销单/员工)与关系。 */
export default function ObjectsPage() {
  return (
    <ToastProvider>
      <Suspense>
        <ObjectsView />
      </Suspense>
    </ToastProvider>
  );
}

function ObjectsView() {
  const toast = useToast();
  const preselect = useSearchParams().get("id");

  const [types, setTypes] = useState<OntologyType[]>([]);
  const [tab, setTab] = useState<string>("");
  const [keyword, setKeyword] = useState("");
  const [objects, setObjects] = useState<OntObject[]>([]);
  const [detail, setDetail] = useState<ObjectDetail | null>(null);

  const fail = (e: unknown) => toast(e instanceof Error ? e.message : String(e), false);

  const load = useCallback(
    (type: string, kw: string) => {
      const params: Record<string, string> = { limit: "100" };
      if (type) params.type = type;
      if (kw) params.keyword = kw;
      api
        .queryObjects(params)
        .then((r) => setObjects(r.objects))
        .catch(fail);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  const open = useCallback((id: string) => {
    api.getObject(id).then(setDetail).catch(fail);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    api
      .listOntologyTypes()
      .then((r) => setTypes(r.types))
      .catch(fail);
    load("", "");
    if (preselect) open(preselect);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const switchTab = (t: string) => {
    setTab(t);
    load(t, keyword);
  };

  return (
    <>
      <PageHeader
        title="对象"
        desc="跨文档归一后的业务实体与关系(configs/ontology/*.yaml 定义,加实体不改代码);文档是对象的证据"
      />
      <div className="flex min-h-0 flex-1 overflow-hidden">
        {/* 左:列表 */}
        <div className="flex min-w-0 flex-1 flex-col border-r border-surface-300">
          <div className="flex flex-wrap items-center gap-2 border-b border-surface-300 bg-surface-0 px-4 py-2.5">
            <button
              className={`rounded-md px-3 py-1 text-sm ${tab === "" ? "bg-accent-500/10 text-accent-700" : "text-ink-500 hover:bg-surface-100"}`}
              onClick={() => switchTab("")}
            >
              全部
            </button>
            {types.map((t) => (
              <button
                key={t.name}
                className={`rounded-md px-3 py-1 text-sm ${tab === t.name ? "bg-accent-500/10 text-accent-700" : "text-ink-500 hover:bg-surface-100"}`}
                onClick={() => switchTab(t.name)}
              >
                {t.title}
              </button>
            ))}
            <input
              className={`${inputCls} ml-auto w-44`}
              placeholder="按名称搜索…"
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && load(tab, keyword)}
            />
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-4">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-xs text-ink-300">
                  <th className="px-3 py-2 font-medium">对象</th>
                  <th className="px-3 py-2 font-medium">类型</th>
                  <th className="px-3 py-2 font-medium">属性</th>
                  <th className="px-3 py-2 font-medium">版本</th>
                </tr>
              </thead>
              <tbody>
                {objects.length === 0 && (
                  <tr>
                    <td colSpan={4} className="px-3 py-10 text-center text-ink-300">
                      暂无对象 —— 单据入库后自动物化生成
                    </td>
                  </tr>
                )}
                {objects.map((o) => (
                  <tr
                    key={o.id}
                    onClick={() => open(o.id)}
                    className={`cursor-pointer border-t border-surface-300/60 ${detail?.object.id === o.id ? "bg-accent-500/5" : "hover:bg-surface-100"}`}
                  >
                    <td className="px-3 py-2.5 font-medium text-ink-900">{o.display_name || "(未命名)"}</td>
                    <td className="px-3 py-2.5 text-xs text-ink-500">
                      {types.find((t) => t.name === o.object_type)?.title ?? o.object_type}
                    </td>
                    <td className="max-w-[280px] truncate px-3 py-2.5 text-xs text-ink-300">
                      {Object.entries(o.properties)
                        .map(([k, v]) => `${k}=${v}`)
                        .join("  ")}
                    </td>
                    <td className="px-3 py-2.5 text-xs text-ink-300">v{o.version}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* 右:详情 */}
        <aside className="w-[380px] shrink-0 overflow-y-auto bg-surface-0 p-5">
          {!detail ? (
            <p className="pt-10 text-center text-sm text-ink-300">点击左侧对象查看详情</p>
          ) : (
            <ObjectDetailPane detail={detail} onOpen={open} />
          )}
        </aside>
      </div>
    </>
  );
}

function ObjectDetailPane({ detail, onOpen }: { detail: ObjectDetail; onOpen: (id: string) => void }) {
  const { object, type_title, links_out, links_in, documents } = detail;
  return (
    <div>
      <p className="text-xs text-ink-300">{type_title}</p>
      <h2 className="mb-1 text-base font-semibold text-ink-900">{object.display_name || "(未命名)"}</h2>
      <p className="mb-4 text-xs text-ink-300">
        v{object.version} · 更新 {object.updated_at?.slice(0, 16).replace("T", " ")}
        {documents.length === 0 && <span className="ml-2 rounded bg-amber-100 px-1.5 py-0.5 text-amber-700">无在库证据</span>}
      </p>

      <Section title="属性">
        <table className="w-full text-sm">
          <tbody>
            {Object.entries(object.properties).map(([k, v]) => (
              <tr key={k} className="border-t border-surface-300/60 first:border-t-0">
                <td className="py-1.5 pr-3 text-xs text-ink-300">{k}</td>
                <td className="py-1.5 text-ink-700 [font-variant-numeric:tabular-nums]">{String(v)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Section>

      {links_out.length > 0 && (
        <Section title="关联(出)">
          {links_out.map((l, i) => (
            <button key={i} className="block w-full rounded-md px-2 py-1.5 text-left text-sm hover:bg-surface-100" onClick={() => onOpen(l.to_id)}>
              <span className="text-xs text-ink-300">{l.link_type} → </span>
              <span className="text-accent-500">{l.to_display}</span>
            </button>
          ))}
        </Section>
      )}
      {links_in.length > 0 && (
        <Section title="关联(入)">
          {links_in.map((l, i) => (
            <button key={i} className="block w-full rounded-md px-2 py-1.5 text-left text-sm hover:bg-surface-100" onClick={() => onOpen(l.from_id)}>
              <span className="text-accent-500">{l.from_display}</span>
              <span className="text-xs text-ink-300"> — {l.link_type} →</span>
            </button>
          ))}
        </Section>
      )}

      <Section title={`证据单据(${documents.length})`}>
        {documents.map((d) => (
          <Link
            key={d.id}
            href={`/review?doc=${d.id}`}
            className="block rounded-md px-2 py-1.5 text-sm text-ink-700 no-underline hover:bg-surface-100"
          >
            <span className="mr-2">{d.filename}</span>
            <StatusBadge status={d.status} />
          </Link>
        ))}
      </Section>
      <p className="mt-4 text-xs text-ink-300">
        对象由单据入库自动物化;修正/删除请从证据单据入手(审核台),对象层会同步跟随。
      </p>
      <Link href="/objects" className={`${btnCls} mt-2 inline-block no-underline`}>
        清除选中
      </Link>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mb-4 rounded-lg border border-surface-300 p-3">
      <p className="mb-2 text-xs font-medium text-ink-300">{title}</p>
      {children}
    </div>
  );
}
