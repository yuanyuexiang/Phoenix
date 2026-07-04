import type { NextConfig } from "next";

// 生产构建走静态导出(BUILD_STATIC=1),产物交给 nginx 托管并由其反代 /api;
// 开发模式用 next dev 的 rewrites 把 /api 代理到本机 workflow(8081)。
const isStaticExport = process.env.BUILD_STATIC === "1";

const nextConfig: NextConfig = {
  output: isStaticExport ? "export" : undefined,
  async rewrites() {
    if (isStaticExport) return [];
    return [
      {
        source: "/api/:path*",
        destination: "http://localhost:8081/api/:path*",
      },
    ];
  },
};

export default nextConfig;
