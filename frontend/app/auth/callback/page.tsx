'use client';

import React, { useEffect, useState, Suspense } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import { oidcAuth } from '../../utils/oidc-auth';
import { useAuth } from '../../contexts/AuthContext';

function OIDCCallbackContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const { setUser, setToken, setAuthType, setIsAuthenticated } = useAuth();
  const [status, setStatus] = useState<'processing' | 'success' | 'error'>('processing');
  const [error, setError] = useState<string>('');

  useEffect(() => {
    const handleCallback = async () => {
      try {
        // 从URL参数获取code和state
        const code = searchParams.get('code');
        const state = searchParams.get('state');
        
        if (!code || !state) {
          throw new Error('缺少必要的认证参数');
        }

        console.log('Processing OIDC callback with code:', code.substring(0, 10) + '...');
        
        // 通过后端API处理OIDC回调
        const authResponse = await oidcAuth.handleCallback(code, state);
        
        console.log('OIDC login successful:', authResponse.user.email);
        
        // 🔥 关键：手动更新AuthContext的全局状态
        setUser(authResponse.user);
        setToken(authResponse.token);
        setAuthType('oidc');
        setIsAuthenticated(true);
        
        setStatus('success');
        
        // 登录成功后重定向到主页
        setTimeout(() => {
          router.push('/');
        }, 1000); // 减少延迟
        
      } catch (error) {
        console.error('OIDC callback failed:', error);
        setError(error instanceof Error ? error.message : '认证失败');
        setStatus('error');
        
        // 错误后延迟重定向到登录页面
        setTimeout(() => {
          router.push('/');
        }, 5000);
      }
    };

    handleCallback();
  }, [searchParams, router]);

  if (status === 'processing') {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50 dark:bg-gray-900">
        <div className="text-center">
          <div className="w-16 h-16 animate-spin rounded-full border-4 border-gray-300 border-t-indigo-600 mx-auto mb-4"></div>
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white mb-2">
            正在处理登录...
          </h2>
          <p className="text-gray-600 dark:text-gray-400">
            请稍等，我们正在验证您的身份
          </p>
        </div>
      </div>
    );
  }

  if (status === 'success') {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50 dark:bg-gray-900">
        <div className="text-center">
          <div className="w-16 h-16 bg-green-100 dark:bg-green-900 rounded-full flex items-center justify-center mx-auto mb-4">
            <svg className="w-8 h-8 text-green-600 dark:text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
            </svg>
          </div>
          <h2 className="text-xl font-semibold text-gray-900 dark:text-white mb-2">
            登录成功！
          </h2>
          <p className="text-gray-600 dark:text-gray-400">
            即将跳转到主页...
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 dark:bg-gray-900">
      <div className="text-center">
        <div className="w-16 h-16 bg-red-100 dark:bg-red-900 rounded-full flex items-center justify-center mx-auto mb-4">
          <svg className="w-8 h-8 text-red-600 dark:text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
          </svg>
        </div>
        <h2 className="text-xl font-semibold text-gray-900 dark:text-white mb-2">
          登录失败
        </h2>
        <p className="text-gray-600 dark:text-gray-400 mb-4">
          {error || '认证过程中发生错误'}
        </p>
        <p className="text-sm text-gray-500 dark:text-gray-500">
          将在几秒后返回首页...
        </p>
      </div>
    </div>
  );
}

// 加载状态组件
function LoadingFallback() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 dark:bg-gray-900">
      <div className="text-center">
        <div className="w-16 h-16 animate-spin rounded-full border-4 border-gray-300 border-t-indigo-600 mx-auto mb-4"></div>
        <h2 className="text-xl font-semibold text-gray-900 dark:text-white mb-2">
          正在加载...
        </h2>
      </div>
    </div>
  );
}

// 默认导出组件，用Suspense包装
export default function OIDCCallback() {
  return (
    <Suspense fallback={<LoadingFallback />}>
      <OIDCCallbackContent />
    </Suspense>
  );
}
