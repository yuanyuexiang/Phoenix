"use client";

import { Suspense, useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import {
  Background,
  BackgroundVariant,
  Controls,
  MarkerType,
  MiniMap,
  ReactFlow,
  type Edge,
  type Node,
  type NodeMouseHandler,
  useEdgesState,
  useNodesState,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";

import * as api from "@/lib/api";
import type { ObjectDetail, OntGraph, OntObject, OntologyType } from "@/lib/types";
import { btnCls, inputCls, PageHeader, StatusBadge, ToastProvider, useToast } from "@/components/ui";

const TYPE_META: Record<string, { color: string; soft: string; glyph: string }> = {
  company: { color: "#2563eb", soft: "#dbeafe", glyph: "企" },
  employee: { color: "#7c3aed", soft: "#ede9fe", glyph: "人" },
  contract: { color: "#0891b2", soft: "#cffafe", glyph: "合" },
  invoice: { color: "#ea580c", soft: "#ffedd5", glyph: "票" },
  reimbursement: { color: "#16a34a", soft: "#dcfce7", glyph: "报" },
  settlement: { color: "#ca8a04", soft: "#fef9c3", glyph: "结" },
  loan: { color: "#db2777", soft: "#fce7f3", glyph: "借" },
};
const FALLBACK = { color: "#64748b", soft: "#e2e8f0", glyph: "物" };

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
  const [type, setType] = useState("");
  const [keyword, setKeyword] = useState("");
  const [objects, setObjects] = useState<OntObject[]>([]);
  const [detail, setDetail] = useState<ObjectDetail | null>(null);
  const [graph, setGraph] = useState<OntGraph | null>(null);
  const [mode, setMode] = useState<"global" | "focus">(preselect ? "focus" : "global");
  const [focusID, setFocusID] = useState<string | null>(preselect);
  const [depth, setDepth] = useState(1);
  const [includeIsolated, setIncludeIsolated] = useState(true);
  const [loading, setLoading] = useState(false);

  const fail = useCallback((e: unknown) => toast(e instanceof Error ? e.message : String(e), false), [toast]);

  const loadList = useCallback(
    (objectType: string, query: string) => {
      const params: Record<string, string> = { limit: "100" };
      if (objectType) params.type = objectType;
      if (query) params.keyword = query;
      api.queryObjects(params).then((r) => setObjects(r.objects)).catch(fail);
    },
    [fail],
  );

  const loadGlobal = useCallback(
    async (objectType = "", query = "", isolated = true) => {
      setLoading(true);
      try {
        setGraph(await api.getGlobalObjectGraph({ objectType, keyword: query, includeIsolated: isolated }));
        setMode("global");
      } catch (e) {
        fail(e);
      } finally {
        setLoading(false);
      }
    },
    [fail],
  );

  const explore = useCallback(
    async (id: string, nextDepth: number) => {
      setLoading(true);
      try {
        const [nextGraph, nextDetail] = await Promise.all([api.getObjectGraph(id, nextDepth), api.getObject(id)]);
        setGraph(nextGraph);
        setDetail(nextDetail);
        setFocusID(id);
        setMode("focus");
      } catch (e) {
        fail(e);
      } finally {
        setLoading(false);
      }
    },
    [fail],
  );

  useEffect(() => {
    api.listOntologyTypes().then((r) => setTypes(r.types)).catch(fail);
    loadList("", "");
    if (preselect) explore(preselect, 1);
    else loadGlobal();
  }, [explore, fail, loadGlobal, loadList, preselect]);

  const selectNode: NodeMouseHandler = useCallback(
    (_, node) => {
      setFocusID(node.id);
      api.getObject(node.id).then(setDetail).catch(fail);
    },
    [fail],
  );
  const centerNode: NodeMouseHandler = useCallback((_, node) => explore(node.id, depth), [depth, explore]);
  const flow = useMemo(() => graphToFlow(graph, types), [graph, types]);

  const changeDepth = (value: number) => {
    setDepth(value);
    if (focusID) explore(focusID, value);
  };

  const search = () => {
    loadList(type, keyword);
    if (mode === "global") loadGlobal(type, keyword, includeIsolated);
  };

  return (
    <>
      <PageHeader title="对象关系" desc="默认纵览全部业务对象及其连接，也可聚焦任一对象进行一跳或两跳调查" />
      <div className="flex min-h-0 flex-1 overflow-hidden">
        <aside className="flex w-[220px] shrink-0 flex-col border-r border-surface-300 bg-surface-0 2xl:w-[250px]">
          <div className="space-y-2 border-b border-surface-300 p-3">
            <input
              className={`${inputCls} w-full`}
              placeholder="搜索对象名称…"
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && search()}
            />
            <select
              className={`${inputCls} w-full`}
              value={type}
              onChange={(e) => {
                setType(e.target.value);
                loadList(e.target.value, keyword);
                if (mode === "global") loadGlobal(e.target.value, keyword, includeIsolated);
              }}
            >
              <option value="">全部对象类型</option>
              {types.map((item) => <option key={item.name} value={item.name}>{item.title}</option>)}
            </select>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-2">
            {objects.length === 0 && <p className="px-3 py-8 text-center text-xs text-ink-300">暂无匹配对象</p>}
            {objects.map((object) => {
              const meta = TYPE_META[object.object_type] ?? FALLBACK;
              return (
                <button
                  key={object.id}
                  onClick={() => explore(object.id, depth)}
                  className={`mb-1 flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left hover:bg-surface-100 ${focusID === object.id && mode === "focus" ? "bg-accent-500/10" : ""}`}
                >
                  <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg text-xs font-semibold" style={{ background: meta.soft, color: meta.color }}>
                    {meta.glyph}
                  </span>
                  <span className="min-w-0">
                    <span className="block truncate text-sm text-ink-700">{object.display_name || "(未命名)"}</span>
                    <span className="block text-[11px] text-ink-300">{types.find((item) => item.name === object.object_type)?.title ?? object.object_type}</span>
                  </span>
                </button>
              );
            })}
          </div>
        </aside>

        <main className="relative min-w-0 flex-1 bg-surface-100">
          <div className="absolute inset-x-3 top-3 z-10 flex flex-wrap items-center gap-2 rounded-lg border border-surface-300 bg-surface-0/95 p-1.5 shadow-card backdrop-blur xl:right-auto">
            <button className={mode === "global" ? btnCls + " bg-accent-100 text-accent-700" : btnCls} onClick={() => loadGlobal(type, keyword, includeIsolated)}>全局关系图</button>
            <span className="h-5 w-px bg-surface-300" />
            <button disabled={!focusID} className={mode === "focus" && depth === 1 ? btnCls + " bg-accent-100 text-accent-700" : btnCls} onClick={() => changeDepth(1)}>中心一跳</button>
            <button disabled={!focusID} className={mode === "focus" && depth === 2 ? btnCls + " bg-accent-100 text-accent-700" : btnCls} onClick={() => changeDepth(2)}>中心两跳</button>
            {mode === "global" && (
              <label className="flex items-center gap-1 text-xs text-ink-300">
                <input
                  type="checkbox"
                  checked={includeIsolated}
                  onChange={(e) => {
                    setIncludeIsolated(e.target.checked);
                    loadGlobal(type, keyword, e.target.checked);
                  }}
                />
                孤立对象
              </label>
            )}
            <span className="px-1 text-xs text-ink-300">
              {graph ? `${graph.nodes.length} 个对象 · ${graph.edges.length} 条关系` : "加载对象关系中"}
            </span>
          </div>
          {loading && <div className="absolute inset-x-0 top-0 z-20 h-0.5 animate-pulse bg-accent-500" />}
          {graph?.truncated && (
            <div className="absolute right-3 top-3 z-10 rounded-md bg-amber-100 px-3 py-1.5 text-xs text-amber-700">对象较多，已按当前视图上限截断</div>
          )}
          {!graph ? (
            <div className="grid h-full place-items-center">
              <div className="text-center">
                <div className="mx-auto mb-3 grid h-16 w-16 place-items-center rounded-full border border-accent-500/30 bg-accent-100 text-2xl text-accent-700">◎</div>
                <p className="text-sm text-ink-500">选择一个业务对象，查看它与企业数据的连接</p>
                <p className="mt-1 text-xs text-ink-300">双击图中节点可将其设为新的探索中心</p>
              </div>
            </div>
          ) : (
            <GraphCanvas
              key={`${mode}-${graph.center ?? "all"}-${depth}-${graph.nodes.length}-${graph.edges.length}`}
              initialNodes={flow.nodes}
              initialEdges={flow.edges}
              onNodeClick={selectNode}
              onNodeDoubleClick={centerNode}
            />
          )}
          <GraphLegend types={types} />
        </main>

        <aside className="w-[300px] shrink-0 overflow-y-auto border-l border-surface-300 bg-surface-0 p-4 2xl:w-[350px] 2xl:p-5">
          {!detail ? <p className="pt-10 text-center text-sm text-ink-300">全局图已展示全部对象；点击任一节点查看属性、关系和证据</p> : <ObjectDetailPane detail={detail} onExplore={(id) => explore(id, depth)} />}
        </aside>
      </div>
    </>
  );
}

function GraphCanvas({
  initialNodes,
  initialEdges,
  onNodeClick,
  onNodeDoubleClick,
}: {
  initialNodes: Node[];
  initialEdges: Edge[];
  onNodeClick: NodeMouseHandler;
  onNodeDoubleClick: NodeMouseHandler;
}) {
  const [nodes, , onNodesChange] = useNodesState(initialNodes);
  const [edges, , onEdgesChange] = useEdgesState(initialEdges);
  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onNodeClick={onNodeClick}
      onNodeDoubleClick={onNodeDoubleClick}
      fitView
      fitViewOptions={{ padding: 0.12, maxZoom: 1 }}
      minZoom={0.35}
      maxZoom={1.8}
      nodesDraggable
      nodesConnectable={false}
      elementsSelectable
    >
      <Background variant={BackgroundVariant.Dots} gap={22} size={1} color="#cbd5e1" />
      <Controls position="bottom-left" showInteractive={false} />
      <MiniMap position="bottom-right" pannable zoomable nodeColor={(node) => String(node.style?.borderColor ?? "#64748b")} />
    </ReactFlow>
  );
}

function graphToFlow(graph: OntGraph | null, types: OntologyType[]): { nodes: Node[]; edges: Edge[] } {
  if (!graph) return { nodes: [], edges: [] };
  const positions = new Map<string, { x: number; y: number }>();
  if (graph.center) {
    const adjacency = new Map<string, Set<string>>();
    for (const edge of graph.edges) {
      if (!adjacency.has(edge.from_id)) adjacency.set(edge.from_id, new Set());
      if (!adjacency.has(edge.to_id)) adjacency.set(edge.to_id, new Set());
      adjacency.get(edge.from_id)!.add(edge.to_id);
      adjacency.get(edge.to_id)!.add(edge.from_id);
    }
    const levels = new Map<string, number>([[graph.center, 0]]);
    const queue = [graph.center];
    while (queue.length) {
      const id = queue.shift()!;
      for (const next of adjacency.get(id) ?? []) {
        if (!levels.has(next)) {
          levels.set(next, (levels.get(id) ?? 0) + 1);
          queue.push(next);
        }
      }
    }
    const rings = new Map<number, OntObject[]>();
    for (const object of graph.nodes) {
      const level = levels.get(object.id) ?? 2;
      if (!rings.has(level)) rings.set(level, []);
      rings.get(level)!.push(object);
    }
    for (const [level, objects] of rings) {
      const radius = level === 0 ? 0 : level * 290;
      objects.forEach((object, index) => {
        const angle = -Math.PI / 2 + (Math.PI * 2 * index) / Math.max(objects.length, 1);
        positions.set(object.id, { x: Math.cos(angle) * radius, y: Math.sin(angle) * radius });
      });
    }
  } else {
    // 全局图按对象类型形成语义分区，并用 shelf packing 自动换行。
    // 旧实现把所有类型只沿 X 轴依次排开，类型一多画布就会极宽，fitView 只能把
    // 节点缩成一条难以阅读的细线。这里限制每个分区和整行宽度，使图谱优先利用
    // 画布的二维空间；位置仍由类型名和对象名决定，刷新时不会随机跳动。
    const groups = new Map<string, OntObject[]>();
    for (const object of graph.nodes) {
      if (!groups.has(object.object_type)) groups.set(object.object_type, []);
      groups.get(object.object_type)!.push(object);
    }
    const ordered = [...groups.entries()]
      .map(([name, objects]) => [name, [...objects].sort((a, b) => a.display_name.localeCompare(b.display_name))] as const)
      .sort(([a], [b]) => a.localeCompare(b));
    const nodeStepX = 250;
    const nodeStepY = 125;
    const groupGapX = 80;
    const groupGapY = 90;
    const maxRowWidth = 1150;
    let cursorX = 0;
    let cursorY = 0;
    let rowHeight = 0;
    ordered.forEach(([, objects]) => {
      const itemColumns = Math.min(3, Math.max(1, Math.ceil(Math.sqrt(objects.length))));
      const itemRows = Math.ceil(objects.length / itemColumns);
      const groupWidth = itemColumns * nodeStepX;
      const groupHeight = itemRows * nodeStepY;
      if (cursorX > 0 && cursorX + groupWidth > maxRowWidth) {
        cursorX = 0;
        cursorY += rowHeight + groupGapY;
        rowHeight = 0;
      }
      objects.forEach((object, index) => {
        positions.set(object.id, {
          x: cursorX + (index % itemColumns) * nodeStepX,
          y: cursorY + Math.floor(index / itemColumns) * nodeStepY,
        });
      });
      cursorX += groupWidth + groupGapX;
      rowHeight = Math.max(rowHeight, groupHeight);
    });
  }

  const nodes: Node[] = graph.nodes.map((object) => {
    const meta = TYPE_META[object.object_type] ?? FALLBACK;
    const title = types.find((item) => item.name === object.object_type)?.title ?? object.object_type;
    const center = object.id === graph.center;
    return {
        id: object.id,
        position: positions.get(object.id) ?? { x: 0, y: 0 },
        data: {
          label: (
            <div className="flex items-center gap-2 text-left">
              <span className="grid h-9 w-9 shrink-0 place-items-center rounded-lg text-sm font-semibold" style={{ background: meta.soft, color: meta.color }}>{meta.glyph}</span>
              <span className="min-w-0">
                <span className="block max-w-[150px] truncate text-[13px] font-medium text-slate-800">{object.display_name || "(未命名)"}</span>
                <span className="block text-[10px] text-slate-400">{title} · v{object.version}</span>
              </span>
            </div>
          ),
        },
        style: {
          width: 210,
          padding: 10,
          borderRadius: 12,
          border: `${center ? 2 : 1}px solid ${meta.color}`,
          borderColor: meta.color,
          background: "rgba(255,255,255,.96)",
          boxShadow: center ? `0 0 0 5px ${meta.soft}, 0 12px 30px rgba(15,23,42,.13)` : "0 7px 20px rgba(15,23,42,.09)",
        },
      };
  });
  const edges = graph.edges.map((edge, index) => ({
    id: `${edge.link_type}-${edge.from_id}-${edge.to_id}-${edge.document_id}-${index}`,
    source: edge.from_id,
    target: edge.to_id,
    label: `${edge.link_title || edge.link_type}${(edge.source_count ?? 1) > 1 ? ` ×${edge.source_count}` : ""}`,
    type: "smoothstep",
    markerEnd: { type: MarkerType.ArrowClosed, width: 16, height: 16, color: "#94a3b8" },
    style: { stroke: "#94a3b8", strokeWidth: 1.4 },
    labelStyle: { fill: "#64748b", fontSize: 10, fontWeight: 500 },
    labelBgStyle: { fill: "rgba(248,250,252,.92)" },
    labelBgPadding: [5, 3] as [number, number],
    labelBgBorderRadius: 4,
  }));
  return { nodes, edges };
}

function GraphLegend({ types }: { types: OntologyType[] }) {
  return (
    <div className="absolute bottom-3 left-1/2 z-10 flex -translate-x-1/2 flex-wrap justify-center gap-3 rounded-full border border-surface-300 bg-surface-0/90 px-4 py-2 shadow-card backdrop-blur">
      {types.map((type) => {
        const meta = TYPE_META[type.name] ?? FALLBACK;
        return <span key={type.name} className="flex items-center gap-1.5 text-[11px] text-ink-500"><i className="h-2 w-2 rounded-full" style={{ background: meta.color }} />{type.title}</span>;
      })}
    </div>
  );
}

function ObjectDetailPane({ detail, onExplore }: { detail: ObjectDetail; onExplore: (id: string) => void }) {
  const { object, type_title, links_out, links_in, documents } = detail;
  return (
    <div>
      <p className="text-xs text-ink-300">{type_title}</p>
      <h2 className="mb-1 text-base font-semibold text-ink-900">{object.display_name || "(未命名)"}</h2>
      <p className="mb-4 text-xs text-ink-300">v{object.version} · 更新 {object.updated_at?.slice(0, 16).replace("T", " ")}</p>
      <Section title="核心属性">
        <table className="w-full text-sm"><tbody>{Object.entries(object.properties).map(([key, value]) => (
          <tr key={key} className="border-t border-surface-300/60 first:border-t-0"><td className="py-1.5 pr-3 text-xs text-ink-300">{key}</td><td className="break-all py-1.5 text-ink-700">{String(value)}</td></tr>
        ))}</tbody></table>
      </Section>
      <Section title={`关系(${links_out.length + links_in.length})`}>
        {[...links_out.map((link) => ({ ...link, target: link.to_id, display: link.to_display, direction: "→" })), ...links_in.map((link) => ({ ...link, target: link.from_id, display: link.from_display, direction: "←" }))].map((link, index) => (
          <button key={`${link.document_id}-${index}`} className="block w-full rounded-md px-2 py-1.5 text-left text-sm hover:bg-surface-100" onClick={() => onExplore(link.target)}>
            <span className="text-xs text-ink-300">{link.direction} {link.link_title || link.link_type} </span><span className="text-accent-500">{link.display}</span>
          </button>
        ))}
      </Section>
      <Section title={`证据单据(${documents.length})`}>
        {documents.length === 0 && <p className="text-xs text-ink-300">暂无在库证据</p>}
        {documents.map((document) => (
          <Link key={document.id} href={`/review?doc=${document.id}`} className="block rounded-md px-2 py-1.5 text-sm text-ink-700 no-underline hover:bg-surface-100">
            <span className="mr-2">{document.filename}</span><StatusBadge status={document.status} />
          </Link>
        ))}
      </Section>
      <button className={`${btnCls} w-full`} onClick={() => onExplore(object.id)}>以此对象为中心探索</button>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return <div className="mb-4 rounded-lg border border-surface-300 p-3"><p className="mb-2 text-xs font-medium text-ink-300">{title}</p>{children}</div>;
}
