import type { Metadata } from "next";
import { Inter, Noto_Sans_SC } from "next/font/google";
import Script from 'next/script';
import "./globals.css";
import { AuthProvider } from './contexts/AuthContext';

// Configure Inter font
const inter = Inter({
  subsets: ['latin'],
  display: 'swap',
  weight: ['400', '700'],
});

// Configure Noto Sans SC font with bold weight
const notoSansSC = Noto_Sans_SC({
  subsets: ['latin'],
  weight: ['700'],  // Only use bold weight
  display: 'swap',
});

export const metadata: Metadata = {
  title: "ImageFlow - 图片管理",
  description: "一个简单而强大的图片管理工具",
  icons: {
    icon: [
      { url: "/static/favicon.ico", sizes: "any" },
      { url: "/static/favicon.svg", type: "image/svg+xml" },
      { url: "/static/favicon-48.png", sizes: "48x48", type: "image/png" },
      { url: "/static/favicon-32.png", sizes: "32x32", type: "image/png" },
      { url: "/static/favicon-16.png", sizes: "16x16", type: "image/png" },
    ],
    apple: [
      { url: "/static/favicon-48.png", sizes: "48x48", type: "image/png" },
    ],
  },
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN" className="light">
      <body className={`${inter.className} ${notoSansSC.className} page-bg py-10 transition-colors duration-300 light-mode font-bold`}>
        {/* 动态背景 */}
        <div className="animated-bg">
          <div className="bubble"></div>
          <div className="bubble"></div>
          <div className="bubble"></div>
          <div className="bubble"></div>
          <div className="bubble"></div>
        </div>

        <AuthProvider>
          {children}
        </AuthProvider>

        {/* 页脚 */}
        <div className="max-w-7xl mx-auto px-6 mt-8 text-center text-gray-600 dark:text-gray-400">
        </div>

        {/* 监控脚本 - 网站统计分析 */}
        <Script 
          src="https://p.dd.kg/script.js"
          data-website-id="1705671c-af07-4f72-a4d4-b32bbbabbf52"
          strategy="afterInteractive"
        />
      </body>
    </html>
  );
}
