'use client';

import React, { createContext, useContext, useEffect, useState, ReactNode } from 'react';
import { OIDCUser, AuthContextType } from '../types';
import { oidcAuth } from '../utils/oidc-auth';
import { getApiKey, validateApiKey } from '../utils/auth';

interface AuthProviderProps {
  children: ReactNode;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: AuthProviderProps) {
  const [user, setUser] = useState<OIDCUser | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [authType, setAuthType] = useState<'api_key' | 'oidc'>('oidc');
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [isLoading, setIsLoading] = useState(true);

  // 检测认证类型（基于环境变量或配置）
  const detectAuthType = (): 'api_key' | 'oidc' => {
    // 优先检查是否有OIDC token
    if (oidcAuth.isAuthenticated()) {
      return 'oidc';
    }
    
    // 如果没有OIDC认证，检查是否有API Key
    const apiKey = getApiKey();
    if (apiKey) {
      return 'api_key';
    }
    
    // 默认使用OIDC
    return 'oidc';
  };

  // 初始化认证状态
  const initializeAuth = async () => {
    setIsLoading(true);
    
    try {
      const detectedAuthType = detectAuthType();
      setAuthType(detectedAuthType);

      if (detectedAuthType === 'oidc') {
        // OIDC认证
        if (oidcAuth.isAuthenticated()) {
          const isValid = await oidcAuth.validateAuthentication();
          if (isValid) {
            const currentUser = oidcAuth.getUser();
            const currentToken = oidcAuth.getToken();
            
            setUser(currentUser);
            setToken(currentToken);
            setIsAuthenticated(true);
          } else {
            // 认证失效，清除状态
            await logout();
          }
        }
      } else {
        // API Key认证（向后兼容）
        const apiKey = getApiKey();
        if (apiKey) {
          const isValid = await validateApiKey(apiKey);
          if (isValid) {
            // 为API Key用户创建虚拟用户对象
            const apiUser: OIDCUser = {
              id: 'api_key_user',
              email: 'api@imageflow.local',
              name: 'API Key User',
              provider: 'api_key',
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
              last_login: new Date().toISOString(),
              is_active: true,
            };
            
            setUser(apiUser);
            setToken(apiKey);
            setIsAuthenticated(true);
          }
        }
      }
    } catch (error) {
      console.error('Auth initialization failed:', error);
      await logout();
    } finally {
      setIsLoading(false);
    }
  };

  // 处理OIDC回调
  const handleOIDCCallback = async (code: string, state: string) => {
    try {
      const authResponse = await oidcAuth.handleCallback(code, state);
      
      setUser(authResponse.user);
      setToken(authResponse.token);
      setAuthType('oidc');
      setIsAuthenticated(true);
      
      // 清除URL中的查询参数
      window.history.replaceState({}, document.title, window.location.pathname);
      
      return true;
    } catch (error) {
      console.error('OIDC callback failed:', error);
      throw error;
    }
  };

  // 登录
  const login = async () => {
    if (authType === 'oidc') {
      await oidcAuth.initiateLogin();
    } else {
      // API Key认证需要通过modal处理
      throw new Error('API Key authentication should be handled by modal');
    }
  };

  // 登出
  const logout = async () => {
    try {
      if (authType === 'oidc') {
        await oidcAuth.logout();
      }
    } catch (error) {
      console.error('Logout failed:', error);
    } finally {
      setUser(null);
      setToken(null);
      setIsAuthenticated(false);
    }
  };

  // 刷新Token
  const refreshToken = async () => {
    if (authType === 'oidc') {
      const isValid = await oidcAuth.refreshTokenIfNeeded();
      if (!isValid) {
        await logout();
      }
    }
  };

  // 处理URL中的OIDC回调
  useEffect(() => {
    const handleURLCallback = async () => {
      const urlParams = new URLSearchParams(window.location.search);
      const code = urlParams.get('code');
      const state = urlParams.get('state');
      
      if (code && state) {
        try {
          setIsLoading(true);
          await handleOIDCCallback(code, state);
        } catch (error) {
          console.error('URL callback handling failed:', error);
          // 可以在这里显示错误消息
        } finally {
          setIsLoading(false);
        }
        return;
      }
      
      // 如果不是回调，执行正常的初始化
      await initializeAuth();
    };

    handleURLCallback();
  }, []);

  // 定期检查token有效性
  useEffect(() => {
    if (!isAuthenticated || authType !== 'oidc') return;

    const interval = setInterval(async () => {
      if (oidcAuth.isTokenExpired()) {
        await logout();
      }
    }, 60000); // 每分钟检查一次

    return () => clearInterval(interval);
  }, [isAuthenticated, authType]);

  const contextValue: AuthContextType = {
    user,
    token,
    authType,
    isAuthenticated,
    isLoading,
    login,
    logout,
    refreshToken,
  };

  return (
    <AuthContext.Provider value={contextValue}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextType {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}

export default AuthContext;
