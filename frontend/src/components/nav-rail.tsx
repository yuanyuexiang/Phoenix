"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { logout } from "@/lib/auth";
import { ThemeToggle } from "./theme-toggle";

type RailItemDef = {
  href: string;
  label: string;
  icon: React.ReactNode;
  exact?: boolean; // "/" 只精确匹配,避免所有路由都高亮
};

// 极简线性图标,stroke=currentColor,随主题变色
const DocsIcon = (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" className="h-5 w-5">
    <path d="M7 3.5h7L18.5 8v12.5h-11.5z" strokeLinejoin="round" />
    <path d="M14 3.5V8h4.5M9.5 12h5M9.5 15.5h5" strokeLinecap="round" />
  </svg>
);

const ReviewIcon = (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" className="h-5 w-5">
    <circle cx="10.5" cy="10.5" r="6.5" />
    <path d="M15.5 15.5L20 20M8 10.5l2 2 3-3.5" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

const SchemaIcon = (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" className="h-5 w-5">
    <rect x="4" y="4" width="7" height="7" rx="1" />
    <rect x="13" y="4" width="7" height="7" rx="1" />
    <rect x="4" y="13" width="7" height="7" rx="1" />
    <path d="M16.5 13.5v7M13 17h7" strokeLinecap="round" />
  </svg>
);

const StatusIcon = (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" className="h-5 w-5">
    <path d="M3 13h3.5l2-6 3.5 12 2.5-7 1.5 3H21" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

const ITEMS: RailItemDef[] = [
  { href: "/", label: "文档", icon: DocsIcon, exact: true },
  { href: "/review", label: "审核", icon: ReviewIcon },
  { href: "/doctypes", label: "单据类型", icon: SchemaIcon },
  { href: "/status", label: "服务状态", icon: StatusIcon },
];

export function NavRail() {
  const pathname = usePathname();

  return (
    <nav className="flex w-[76px] shrink-0 flex-col items-center border-r border-surface-300 bg-surface-0 py-4">
      {/* Phoenix 印记 */}
      <Link href="/" title="Phoenix" className="relative mb-6 flex h-11 w-11 items-center justify-center no-underline">
        <span className="absolute inset-0 rounded-full border border-accent-500/50" />
        <span className="absolute inset-[5px] rounded-full border border-accent-500/25" />
        <span className="text-lg font-semibold text-accent-500">凤</span>
      </Link>

      {/* 导航项 */}
      <div className="flex flex-col items-stretch gap-1 self-stretch px-2">
        {ITEMS.map((it) => {
          const on = it.exact ? pathname === it.href : pathname === it.href || pathname.startsWith(it.href + "/");
          return (
            <Link
              key={it.href}
              href={it.href}
              aria-current={on ? "page" : undefined}
              title={it.label}
              className={`relative flex flex-col items-center gap-1 rounded-md py-2.5 no-underline transition-colors ${
                on
                  ? "bg-accent-500/10 text-accent-500"
                  : "text-ink-300 hover:bg-surface-100 hover:text-ink-700"
              }`}
            >
              {/* 选中:左侧主色细条 */}
              {on && <span className="absolute inset-y-1.5 left-0 w-[2px] rounded-full bg-accent-500" />}
              {it.icon}
              <span className="text-[10px] tracking-wide">{it.label}</span>
            </Link>
          );
        })}
      </div>

      {/* 底部:主题切换 + 退出 */}
      <div className="mt-auto flex flex-col items-stretch self-stretch px-2 pt-2">
        <div className="mx-auto mb-2 h-px w-8 bg-surface-300" />
        <ThemeToggle />
        <button
          onClick={logout}
          title="退出登录"
          className="flex flex-col items-center gap-1 rounded-md py-2.5 text-ink-300 transition-colors hover:bg-surface-100 hover:text-red-500"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" className="h-5 w-5">
            <path d="M14 4h-8v16h8M10 12h11M18 8.5L21.5 12 18 15.5" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
          <span className="text-[10px] tracking-wide">退出</span>
        </button>
      </div>
    </nav>
  );
}
