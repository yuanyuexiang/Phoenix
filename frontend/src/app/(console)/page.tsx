"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";

import * as api from "@/lib/api";
import type { Component, DashboardData, DocType } from "@/lib/types";
import { DOCTYPE_SPECIAL, STATUS_META } from "@/lib/types";
import { btnCls, PageHeader, StatusBadge } from "@/components/ui";

const ACTION_LABEL: Record<string, string> = {
  upload: "上传了文档",
  extract: "获取了提取字段",
  validate: "校验了文档",
  save: "确认文档入库",
  delete: "删除了文档",
  ontology_rebuild: "重建了对象层",
  user_create: "创建了员工账号",
  user_update: "更新了员工账号",
  user_password: "重置了员工口令",
};

export default function DashboardPage() {
  const [range, setRange] = useState<"7d" | "30d">("7d");
  const [data, setData] = useState<DashboardData | null>(null);
  const [health, setHealth] = useState<Component[]>([]);
  const [doctypes, setDoctypes] = useState<DocType[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [dashboard, status, types] = await Promise.all([
        api.fetchDashboard(range), api.fetchStatus(), api.listDocTypes(),
      ]);
      setData(dashboard);
      setHealth(status.components ?? []);
      setDoctypes(types);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [range]);

  useEffect(() => { load(); }, [load]);

  const typeTitle = useCallback((name: string) =>
    DOCTYPE_SPECIAL[name] ?? doctypes.find((item) => item.name === name)?.title ?? name, [doctypes]);

  return (
    <>
      <PageHeader
        title="工作台"
        desc="文档处理、人工审核与业务对象的统一运营视图"
        extra={
          <div className="flex items-center gap-2">
            <div className="flex rounded-md border border-surface-300 bg-surface-50 p-0.5">
              {(["7d", "30d"] as const).map((value) => (
                <button key={value} onClick={() => setRange(value)} className={`rounded px-3 py-1 text-xs ${range === value ? "bg-surface-0 text-accent-500 shadow-card" : "text-ink-300"}`}>
                  {value === "7d" ? "近 7 天" : "近 30 天"}
                </button>
              ))}
            </div>
            <button className={btnCls} disabled={loading} onClick={load}>{loading ? "刷新中…" : "刷新"}</button>
          </div>
        }
      />
      <div className="min-h-0 flex-1 overflow-y-auto p-4 sm:p-6">
        {error && <div className="mb-4 rounded-lg border border-red-500/30 bg-red-100 px-4 py-3 text-sm text-red-700">工作台加载失败：{error}</div>}
        {!data ? <DashboardSkeleton /> : <DashboardContent data={data} health={health} typeTitle={typeTitle} />}
      </div>
    </>
  );
}

function DashboardContent({ data, health, typeTitle }: { data: DashboardData; health: Component[]; typeTitle: (name: string) => string }) {
  const s = data.summary;
  const cards = [
    { label: `${data.range_days} 天文档`, value: s.uploaded, hint: `${s.saved} 份已入库`, tone: "blue", href: "/documents" },
    { label: "入库率", value: `${(s.save_rate * 100).toFixed(1)}%`, hint: "按周期内文档当前状态", tone: "green", href: "/documents" },
    { label: "待人工审核", value: s.needs_review, hint: "当前全部积压", tone: "amber", href: "/review" },
    { label: "处理失败", value: s.failed, hint: `${data.range_days} 天内`, tone: "red", href: "/documents" },
    { label: "活跃业务对象", value: s.objects_changed, hint: `共 ${s.objects_total} 个对象`, tone: "violet", href: "/objects" },
  ];
  return (
    <div className="mx-auto max-w-[1600px] space-y-5">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
        {cards.map((card) => <MetricCard key={card.label} {...card} />)}
      </div>
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.6fr)_minmax(340px,.8fr)]">
        <Panel title="文档处理趋势" desc={`最近 ${data.range_days} 天的新增文档及当前处理结果`}>
          <TrendChart values={data.trends} />
        </Panel>
        <Panel title="待处理事项" desc="优先显示处理失败和等待最久的文档" action={<Link href="/review" className="text-xs text-accent-500 no-underline">进入审核台 →</Link>}>
          <WorkList items={data.work_items} typeTitle={typeTitle} />
        </Panel>
      </div>
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.25fr)_minmax(340px,.75fr)]">
        <Panel title="单据类型表现" desc="识别规则和审核工作量的直接反馈">
          <DocTypeTable items={data.doctype_stats} typeTitle={typeTitle} />
        </Panel>
        <Panel title="系统运行状态" desc="核心依赖的实时健康摘要" action={<Link href="/status" className="text-xs text-accent-500 no-underline">查看详情 →</Link>}>
          <HealthGrid items={health} />
        </Panel>
      </div>
      <Panel title="最近业务活动" desc="上传、审核、入库和管理动作的审计轨迹">
        <ActivityList items={data.recent_activity} />
      </Panel>
    </div>
  );
}

function MetricCard({ label, value, hint, tone, href }: { label: string; value: string | number; hint: string; tone: string; href: string }) {
  const colors: Record<string, string> = { blue: "bg-accent-500", green: "bg-green-500", amber: "bg-amber-500", red: "bg-red-500", violet: "bg-violet-500" };
  return <Link href={href} className="group rounded-xl border border-surface-300 bg-surface-0 p-4 no-underline shadow-card transition hover:-translate-y-0.5 hover:shadow-pop">
    <div className={`mb-4 h-1 w-8 rounded-full ${colors[tone]}`} />
    <p className="text-xs text-ink-300">{label}</p><p className="mt-1 text-2xl font-semibold tracking-tight text-ink-900">{value}</p>
    <p className="mt-2 truncate text-[11px] text-ink-300">{hint}</p>
  </Link>;
}

function Panel({ title, desc, action, children }: { title: string; desc?: string; action?: React.ReactNode; children: React.ReactNode }) {
  return <section className="min-w-0 overflow-hidden rounded-xl border border-surface-300 bg-surface-0 shadow-card">
    <div className="flex items-start justify-between gap-3 border-b border-surface-300 px-5 py-4"><div><h2 className="text-sm font-semibold text-ink-900">{title}</h2>{desc && <p className="mt-0.5 text-xs text-ink-300">{desc}</p>}</div>{action}</div>
    {children}
  </section>;
}

function TrendChart({ values }: { values: DashboardData["trends"] }) {
  const max = Math.max(1, ...values.map((item) => item.uploaded));
  const labelsEvery = values.length > 10 ? 5 : 1;
  return <div className="p-5"><div className="flex h-52 items-end gap-1.5 border-b border-surface-300 px-1">
    {values.map((item) => <div key={item.date} className="group relative flex h-full min-w-0 flex-1 items-end justify-center gap-[2px]" title={`${item.date}：上传 ${item.uploaded}，入库 ${item.saved}，待审核 ${item.needs_review}，失败 ${item.failed}`}>
      <span className="w-[42%] min-w-[3px] rounded-t bg-accent-300 transition group-hover:bg-accent-500" style={{ height: `${Math.max(item.uploaded ? 4 : 0, item.uploaded / max * 100)}%` }} />
      <span className="w-[42%] min-w-[3px] rounded-t bg-green-500" style={{ height: `${Math.max(item.saved ? 4 : 0, item.saved / max * 100)}%` }} />
    </div>)}
  </div><div className="mt-2 flex gap-1.5 px-1">{values.map((item, index) => <span key={item.date} className="min-w-0 flex-1 text-center text-[9px] text-ink-300">{index % labelsEvery === 0 || index === values.length - 1 ? item.date.slice(5) : ""}</span>)}</div>
  <div className="mt-3 flex justify-center gap-4 text-[11px] text-ink-300"><span><i className="mr-1 inline-block h-2 w-2 rounded-sm bg-accent-300" />上传</span><span><i className="mr-1 inline-block h-2 w-2 rounded-sm bg-green-500" />已入库</span></div></div>;
}

function WorkList({ items, typeTitle }: { items: DashboardData["work_items"]; typeTitle: (name: string) => string }) {
  if (!items.length) return <Empty text="当前没有待审核或失败文档" />;
  return <div className="divide-y divide-surface-300/60">{items.map((item) => <Link key={item.id} href={`/review?doc=${item.id}`} className="flex items-start gap-3 px-5 py-3 no-underline hover:bg-surface-100">
    <span className={`mt-1 h-2 w-2 shrink-0 rounded-full ${item.status === "failed" ? "bg-red-500" : "bg-amber-500"}`} />
    <span className="min-w-0 flex-1"><span className="block truncate text-sm text-ink-700">{item.filename}</span><span className="mt-0.5 block truncate text-[11px] text-ink-300">{item.issue || `${typeTitle(item.doc_type)}等待处理`}</span></span><StatusBadge status={item.status} />
  </Link>)}</div>;
}

function DocTypeTable({ items, typeTitle }: { items: DashboardData["doctype_stats"]; typeTitle: (name: string) => string }) {
  if (!items.length) return <Empty text="当前周期内还没有文档" />;
  return <div className="overflow-x-auto"><table className="min-w-[620px] w-full text-sm"><thead><tr className="text-left text-xs text-ink-300"><th className="px-5 py-2.5 font-medium">单据类型</th><th className="px-4 py-2.5 font-medium">处理量</th><th className="px-4 py-2.5 font-medium">入库率</th><th className="px-4 py-2.5 font-medium">待审核</th><th className="px-4 py-2.5 font-medium">失败</th></tr></thead><tbody>{items.map((item) => <tr key={item.doc_type} className="border-t border-surface-300/60"><td className="px-5 py-3 text-ink-700">{typeTitle(item.doc_type)}<code className="ml-2 text-[10px] text-ink-300">{item.doc_type}</code></td><td className="px-4 py-3 text-ink-500">{item.total}</td><td className="px-4 py-3"><div className="flex items-center gap-2"><span className="h-1.5 w-20 overflow-hidden rounded-full bg-surface-100"><i className="block h-full rounded-full bg-green-500" style={{ width: `${item.save_rate * 100}%` }} /></span><span className="text-xs text-ink-500">{(item.save_rate * 100).toFixed(0)}%</span></div></td><td className="px-4 py-3 text-amber-700">{item.needs_review}</td><td className="px-4 py-3 text-red-500">{item.failed}</td></tr>)}</tbody></table></div>;
}

function HealthGrid({ items }: { items: Component[] }) {
  if (!items.length) return <Empty text="暂无健康检查结果" />;
  return <div className="grid gap-3 p-5 sm:grid-cols-2 xl:grid-cols-1 2xl:grid-cols-2">{items.map((item) => <div key={item.name} className="rounded-lg bg-surface-50 p-3"><div className="flex items-center justify-between gap-2"><span className="truncate text-sm text-ink-700">{item.name}</span><span className={`h-2 w-2 shrink-0 rounded-full ${item.ok ? "bg-green-500" : "bg-red-500"}`} /></div><p className="mt-1 text-[11px] text-ink-300">{item.ok ? `响应 ${item.latency_ms}ms` : item.error || "服务异常"}</p></div>)}</div>;
}

function ActivityList({ items }: { items: DashboardData["recent_activity"] }) {
  if (!items.length) return <Empty text="暂无业务活动" />;
  return <div className="grid divide-y divide-surface-300/60 lg:grid-cols-2 lg:divide-y-0">{items.map((item, index) => <div key={item.id} className={`flex items-center gap-3 px-5 py-3 ${index > 1 ? "lg:border-t lg:border-surface-300/60" : ""} ${index % 2 ? "lg:border-l lg:border-surface-300/60" : ""}`}><span className="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-accent-100 text-[10px] text-accent-700">{(item.actor || "系").slice(0, 1)}</span><span className="min-w-0 flex-1"><span className="text-xs text-ink-500"><b className="font-medium text-ink-700">{item.actor || "系统"}</b> {ACTION_LABEL[item.action] ?? item.action}</span>{item.filename && <span className="ml-1 text-xs text-accent-500">{item.filename}</span>}</span><time className="shrink-0 text-[10px] text-ink-300">{relativeTime(item.occurred_at)}</time></div>)}</div>;
}

function Empty({ text }: { text: string }) { return <div className="px-5 py-12 text-center text-xs text-ink-300">{text}</div>; }
function DashboardSkeleton() { return <div className="mx-auto max-w-[1600px] animate-pulse space-y-5"><div className="grid grid-cols-2 gap-3 lg:grid-cols-5">{Array.from({ length: 5 }).map((_, i) => <div key={i} className="h-32 rounded-xl bg-surface-100" />)}</div><div className="h-80 rounded-xl bg-surface-100" /></div>; }
function relativeTime(value: string) { const ms = Date.now() - new Date(value).getTime(); const mins = Math.max(0, Math.floor(ms / 60000)); if (mins < 1) return "刚刚"; if (mins < 60) return `${mins} 分钟前`; const hours = Math.floor(mins / 60); if (hours < 24) return `${hours} 小时前`; return `${Math.floor(hours / 24)} 天前`; }
