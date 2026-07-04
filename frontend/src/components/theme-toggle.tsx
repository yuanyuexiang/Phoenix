"use client";

import { useEffect, useState } from "react";

/** light/dark 切换:只改 <html data-theme>,配色由 globals.css 的 raw palette 层接管。 */
export function ThemeToggle() {
  const [theme, setTheme] = useState<"light" | "dark">("light");

  useEffect(() => {
    const t = document.documentElement.dataset.theme;
    if (t === "dark" || t === "light") setTheme(t);
  }, []);

  const toggle = () => {
    const next = theme === "dark" ? "light" : "dark";
    setTheme(next);
    document.documentElement.dataset.theme = next;
    try {
      localStorage.setItem("phx-theme", next);
    } catch {}
  };

  return (
    <button
      onClick={toggle}
      title={theme === "dark" ? "切换到浅色" : "切换到深色"}
      className="flex flex-col items-center gap-1 rounded-md py-2.5 text-ink-300 transition-colors hover:bg-surface-100 hover:text-ink-700"
    >
      {theme === "dark" ? (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" className="h-5 w-5">
          <circle cx="12" cy="12" r="4" />
          <path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M18.4 5.6L17 7M7 17l-1.4 1.4" strokeLinecap="round" />
        </svg>
      ) : (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" className="h-5 w-5">
          <path d="M20 13.5A8 8 0 0 1 10.5 4 8 8 0 1 0 20 13.5z" strokeLinejoin="round" />
        </svg>
      )}
      <span className="text-[10px] tracking-wide">{theme === "dark" ? "浅色" : "深色"}</span>
    </button>
  );
}
