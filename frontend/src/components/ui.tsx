"use client";

import { createContext, useCallback, useContext, useRef, useState } from "react";

import { STATUS_META } from "@/lib/types";

/* ---------- 状态徽标 ---------- */

const TONE_CLASS: Record<string, string> = {
  gray: "bg-surface-100 text-ink-500",
  blue: "bg-accent-100 text-accent-700",
  green: "bg-green-100 text-green-700",
  amber: "bg-amber-100 text-amber-700",
  red: "bg-red-100 text-red-700",
};

export function StatusBadge({ status }: { status: string }) {
  const meta = STATUS_META[status] ?? { text: status, tone: "gray" as const };
  return (
    <span className={`inline-block rounded-full px-2.5 py-0.5 text-xs ${TONE_CLASS[meta.tone]}`}>
      {meta.text}
    </span>
  );
}

/* ---------- 页头 ---------- */

export function PageHeader({ title, desc, extra }: { title: string; desc?: string; extra?: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between border-b border-surface-300 bg-surface-0 px-6 py-4">
      <div>
        <h1 className="text-base font-semibold text-ink-900">{title}</h1>
        {desc && <p className="mt-0.5 text-xs text-ink-300">{desc}</p>}
      </div>
      {extra}
    </div>
  );
}

/* ---------- 基础控件(统一风格的原生控件封装) ---------- */

export const inputCls =
  "rounded-md border border-surface-300 bg-surface-0 px-3 py-1.5 text-sm text-ink-700 " +
  "placeholder:text-ink-300 focus:border-accent-500 focus:outline-none";

export const btnCls =
  "rounded-md border border-surface-300 bg-surface-0 px-3.5 py-1.5 text-sm text-ink-700 " +
  "transition-colors hover:border-accent-300 hover:text-accent-500 disabled:opacity-50";

export const btnPrimaryCls =
  "rounded-md bg-accent-500 px-3.5 py-1.5 text-sm text-white transition-colors " +
  "hover:bg-accent-700 disabled:opacity-50";

export const btnDangerCls =
  "rounded-md border border-red-500/40 bg-surface-0 px-3.5 py-1.5 text-sm text-red-500 " +
  "transition-colors hover:bg-red-100 disabled:opacity-50";

/* ---------- 轻量 toast ---------- */

type Toast = { id: number; text: string; ok: boolean };

const ToastCtx = createContext<(text: string, ok?: boolean) => void>(() => {});

export const useToast = () => useContext(ToastCtx);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<Toast[]>([]);
  const seq = useRef(0);

  const push = useCallback((text: string, ok = true) => {
    const id = ++seq.current;
    setItems((list) => [...list, { id, text, ok }]);
    setTimeout(() => setItems((list) => list.filter((t) => t.id !== id)), 3500);
  }, []);

  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed right-4 top-4 z-50 flex flex-col gap-2">
        {items.map((t) => (
          <div
            key={t.id}
            className={`rounded-md px-4 py-2 text-sm text-white shadow-pop ${t.ok ? "bg-green-500" : "bg-red-500"}`}
          >
            {t.text}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}
