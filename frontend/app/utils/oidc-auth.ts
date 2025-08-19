import { OIDCUser, AuthResponse, OIDCLoginResponse } from "../types";

const TOKEN_KEY = "imageflow_jwt_token";
const USER_KEY = "imageflow_user";
const EXPIRY_KEY = "imageflow_token_expiry";
const BASE_URL = process.env.NEXT_PUBLIC_API_URL || "";

export class OIDCAuthManager {
  private static instance: OIDCAuthManager;

  private constructor() {}

  public static getInstance(): OIDCAuthManager {
    if (!OIDCAuthManager.instance) {
      OIDCAuthManager.instance = new OIDCAuthManager();
    }
    return OIDCAuthManager.instance;
  }

  // JWT Token管理
  public getToken(): string | null {
    if (typeof window === "undefined") return null;
    return localStorage.getItem(TOKEN_KEY);
  }

  public setToken(token: string, expiresAt: number): void {
    if (typeof window === "undefined") return;
    localStorage.setItem(TOKEN_KEY, token);
    localStorage.setItem(EXPIRY_KEY, expiresAt.toString());
  }

  public removeToken(): void {
    if (typeof window === "undefined") return;
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(EXPIRY_KEY);
  }

  // 用户信息管理
  public getUser(): OIDCUser | null {
    if (typeof window === "undefined") return null;
    const userStr = localStorage.getItem(USER_KEY);
    if (!userStr) return null;
    
    try {
      return JSON.parse(userStr);
    } catch {
      return null;
    }
  }

  public setUser(user: OIDCUser): void {
    if (typeof window === "undefined") return;
    localStorage.setItem(USER_KEY, JSON.stringify(user));
  }

  public removeUser(): void {
    if (typeof window === "undefined") return;
    localStorage.removeItem(USER_KEY);
  }

  // Token有效性检查
  public isTokenExpired(): boolean {
    if (typeof window === "undefined") return true;
    const expiryStr = localStorage.getItem(EXPIRY_KEY);
    if (!expiryStr) return true;
    
    const expiryTime = parseInt(expiryStr, 10);
    return Date.now() >= expiryTime * 1000; // 转换为毫秒
  }

  public isAuthenticated(): boolean {
    const token = this.getToken();
    const user = this.getUser();
    return !!(token && user && !this.isTokenExpired());
  }

  // OIDC登录流程
  public async initiateLogin(): Promise<void> {
    try {
      const response = await fetch(`${BASE_URL}/api/auth/login`, {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
        },
        credentials: 'include', // 确保可以接收和发送cookie
      });

      if (!response.ok) {
        throw new Error('Failed to initiate OIDC login');
      }

      const data: OIDCLoginResponse = await response.json();
      
      // state已由后端通过HttpOnly cookie保存，前端无需存储
      // 直接重定向到OIDC提供者
      window.location.href = data.auth_url;
    } catch (error) {
      console.error('OIDC login initiation failed:', error);
      throw new Error('登录初始化失败，请重试');
    }
  }

  // 处理OIDC回调
  public async handleCallback(code: string, state: string): Promise<AuthResponse> {
    // 后端会验证state参数（通过HttpOnly cookie），前端只需要发送即可
    // 移除前端localStorage验证，避免与后端cookie验证冲突
    
    try {
      // 通过POST API发送code和state给后端处理，包含cookies用于state验证
      const response = await fetch(`${BASE_URL}/api/auth/callback`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        credentials: 'include', // 确保包含cookie
        body: JSON.stringify({
          code: code,
          state: state,
        }),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Authentication failed: ${response.status} ${errorText}`);
      }

      const authData: AuthResponse = await response.json();
      
      // 保存认证信息
      this.setToken(authData.token, authData.expires_at);
      this.setUser(authData.user);
      
      return authData;
    } catch (error) {
      console.error('OIDC callback handling failed:', error);
      throw new Error('认证回调处理失败，请重试');
    }
  }

  // 登出
  public async logout(): Promise<void> {
    try {
      const token = this.getToken();
      if (token) {
        // 调用后端登出接口
        await fetch(`${BASE_URL}/api/auth/logout`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`,
          },
        });
      }
    } catch (error) {
      console.error('Logout API call failed:', error);
      // 即使API调用失败，也要清除本地数据
    } finally {
      // 清除本地认证数据
      this.removeToken();
      this.removeUser();
    }
  }

  // 获取用户配置
  public async getUserProfile(): Promise<OIDCUser> {
    const token = this.getToken();
    if (!token) {
      throw new Error('No authentication token');
    }

    try {
      const response = await fetch(`${BASE_URL}/api/auth/profile`, {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
        },
      });

      if (!response.ok) {
        if (response.status === 401) {
          // Token过期或无效，清除认证数据
          this.removeToken();
          this.removeUser();
          throw new Error('Authentication expired');
        }
        throw new Error(`Failed to fetch user profile: ${response.status}`);
      }

      const user: OIDCUser = await response.json();
      
      // 更新本地用户信息
      this.setUser(user);
      
      return user;
    } catch (error) {
      console.error('Failed to fetch user profile:', error);
      throw error;
    }
  }

  // 获取认证头
  public getAuthHeader(): Record<string, string> {
    const token = this.getToken();
    if (!token) {
      throw new Error('No authentication token');
    }

    return {
      'Authorization': `Bearer ${token}`,
    };
  }

  // Token刷新（如果需要）
  public async refreshTokenIfNeeded(): Promise<boolean> {
    if (!this.isTokenExpired()) {
      return true; // Token仍然有效
    }

    // JWT通常不支持刷新，需要重新登录
    this.removeToken();
    this.removeUser();
    return false;
  }

  // 检查认证状态并自动刷新
  public async validateAuthentication(): Promise<boolean> {
    if (!this.isAuthenticated()) {
      return false;
    }

    try {
      // 尝试获取用户配置来验证token有效性
      await this.getUserProfile();
      return true;
    } catch (error) {
      console.error('Authentication validation failed:', error);
      return false;
    }
  }
}

// 导出单例实例
export const oidcAuth = OIDCAuthManager.getInstance();

// 向后兼容的导出
export const getToken = () => oidcAuth.getToken();
export const getUser = () => oidcAuth.getUser();
export const isAuthenticated = () => oidcAuth.isAuthenticated();
export const logout = () => oidcAuth.logout();
export const login = () => oidcAuth.initiateLogin();
