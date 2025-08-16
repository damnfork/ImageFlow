'use client'

import Link from 'next/link'
import { useTheme } from '../hooks/useTheme'
import { usePathname } from 'next/navigation'
import { motion } from 'framer-motion'
import { useAuth } from '../contexts/AuthContext'
import UserProfile from './UserProfile'
import { ImageIcon, HamburgerMenuIcon, LockClosedIcon, SunIcon, MoonIcon, LoginIcon } from './ui/icons'

interface HeaderProps {
  onLoginClick?: () => void
  title?: string
}

export default function Header({ onLoginClick, title }: HeaderProps) {
  const { isDarkMode, toggleTheme } = useTheme()
  const { isAuthenticated, isLoading, authType } = useAuth()
  const pathname = usePathname()

  const getTitle = () => {
    if (title) return title
    if (pathname === '/manage') return '图片管理'
    return 'ImageFlow'
  }

  return (
    <div className="flex items-center justify-between mb-10">
      <div className="flex items-center">
        <Link href="/" className="mr-4">
          <div className="bg-gradient-primary w-14 h-14 rounded-2xl flex items-center justify-center shadow-lg transform rotate-12 hover:rotate-0 transition-transform duration-300">
            <ImageIcon className="h-8 w-8 text-white" />
          </div>
        </Link>
        <h1 className="text-4xl font-bold bg-clip-text text-transparent bg-gradient-primary pb-1">
          {getTitle()}
        </h1>
      </div>

      <div className="flex items-center space-x-2">
        {/* 管理页面链接 */}
        {isAuthenticated && pathname !== '/manage' && (
          <Link href="/manage" className="btn-icon">
            <HamburgerMenuIcon className="h-6 w-6" />
          </Link>
        )}

        {/* 主题切换按钮 */}
        <button onClick={toggleTheme} className="btn-icon">
          {isDarkMode ? (
            <SunIcon className="h-6 w-6 text-amber-500" />
          ) : (
            <MoonIcon className="h-6 w-6 text-gray-700 dark:text-gray-300" />
          )}
        </button>

        {/* 认证状态显示 */}
        {isLoading ? (
          <div className="btn-icon">
            <div className="w-6 h-6 animate-spin rounded-full border-2 border-gray-300 border-t-indigo-600"></div>
          </div>
        ) : isAuthenticated ? (
          // 显示用户配置文件
          <UserProfile />
        ) : (
          // 显示登录按钮
          <button 
            onClick={onLoginClick} 
            className="btn-icon relative group"
            title="登录"
          >
            <LoginIcon className="h-6 w-6" />
            <motion.div
              className="absolute -bottom-8 left-1/2 transform -translate-x-1/2 bg-gray-900 text-white text-xs px-2 py-1 rounded opacity-0 group-hover:opacity-100 pointer-events-none whitespace-nowrap"
              initial={{ opacity: 0, y: -5 }}
              animate={{ opacity: 0, y: -5 }}
              whileHover={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.2 }}
            >
              点击登录
            </motion.div>
          </button>
        )}

        {/* API Key认证状态指示器（向后兼容） */}
        {isAuthenticated && authType === 'api_key' && (
          <motion.div
            className="absolute -top-1 -right-1 w-4 h-4 bg-green-500 rounded-full flex items-center justify-center shadow-lg"
            initial={{ scale: 0, opacity: 0 }}
            animate={{
              scale: 1,
              opacity: 1,
              boxShadow: ["0 0 0 0 rgba(34, 197, 94, 0.4)", "0 0 0 8px rgba(34, 197, 94, 0)", "0 0 0 0 rgba(34, 197, 94, 0)"],
            }}
            transition={{
              scale: { duration: 0.3, ease: "easeOut" },
              opacity: { duration: 0.3, ease: "easeOut" },
              boxShadow: {
                duration: 2,
                repeat: Infinity,
                ease: "easeOut"
              }
            }}
          >
            <motion.div
              initial={{ scale: 0 }}
              animate={{ scale: 1 }}
              transition={{ delay: 0.2, duration: 0.2 }}
              className="w-2 h-2 bg-white rounded-full"
            />
          </motion.div>
        )}
      </div>
    </div>
  )
} 
