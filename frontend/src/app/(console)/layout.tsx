import { ConsoleGuard } from "@/components/console-guard";
import { NavRail } from "@/components/nav-rail";

/**
 * 控制台外壳:最左导航栏(文档 / 审核 / 单据类型 / 服务状态 + 主题切换)常驻,
 * 右侧内容区由具体路由填充。ConsoleGuard 确认已解锁后才渲染,未登录跳 /login。
 */
export default function ConsoleLayout({ children }: { children: React.ReactNode }) {
  return (
    <ConsoleGuard>
      <div className="flex h-screen">
        <NavRail />
        <div className="flex min-h-0 flex-1 flex-col overflow-hidden">{children}</div>
      </div>
    </ConsoleGuard>
  );
}
