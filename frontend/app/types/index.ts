// 通用类型
export interface ApiResponse<T = any> {
  success: boolean;
  data?: T;
  error?: string;
}

export interface PaginatedResponse<T> extends ApiResponse {
  data?: {
    items: T[];
    total: number;
    page: number;
    pageSize: number;
  };
}

// 图片相关类型
export interface ImageFile {
  id: string;
  filename: string;
  url: string;
  format: string;
  orientation: string;
  size: number;
  path: string;
  storageType: string;
  tags?: string[];
  width?: number;
  height?: number;
  urls?: {
    original: string;
    webp: string;
    avif: string;
  };
}

export interface ImageListResponse {
  images: ImageFile[];
  page: number;
  totalPages: number;
  total: number;
}

export interface ImageFilterState {
  format: string;
  orientation: string;
  tag: string;
}

// 组件 Props 类型
export interface ImageCardProps {
  image: ImageFile;
  onClick: () => void;
}

export interface ImageModalProps {
  image: ImageFile | null;
  isOpen: boolean;
  onClose: () => void;
  onDelete: (id: string) => Promise<void>;
}

export interface ApiKeyModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: (apiKey: string) => void;
}

export interface ImageFiltersProps {
  onFilterChange: (format: string, orientation: string, tag: string) => void;
}

// 上传结果类型定义
export interface UploadResult {
  filename: string;
  status: "success" | "error";
  message: string;
  format?: string;
  orientation?: string;
  expiryTime?: string; // 过期时间
  tags?: string[];
  urls?: {
    original: string;
    webp: string;
    avif: string;
  };
  id?: string;
  path?: string;
}

export interface UploadResponse {
  results: UploadResult[];
}

// 状态消息类型
export interface StatusMessage {
  type: "success" | "error" | "warning";
  message: string;
}

// 配置类型
export interface ConfigSettings {
  maxUploadCount: number;
  imageQuality: number;
  compressionEffort?: number;
  forceLossless?: boolean;
}

// OIDC用户类型
export interface OIDCUser {
  id: string;
  email: string;
  name: string;
  picture?: string;
  provider: string;
  created_at: string;
  updated_at: string;
  last_login: string;
  is_active: boolean;
}

// 认证响应类型
export interface AuthResponse {
  token: string;
  user: OIDCUser;
  expires_at: number;
}

// 认证状态类型
export type AuthType = "api_key" | "oidc";

// 认证上下文类型
export interface AuthContextType {
  user: OIDCUser | null;
  token: string | null;
  authType: AuthType;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: () => Promise<void>;
  logout: () => void;
  refreshToken: () => Promise<void>;
  // Setter functions for manual state updates (used in OIDC callback)
  setUser: (user: OIDCUser | null) => void;
  setToken: (token: string | null) => void;
  setAuthType: (type: AuthType) => void;
  setIsAuthenticated: (authenticated: boolean) => void;
}

// OIDC登录响应
export interface OIDCLoginResponse {
  auth_url: string;
  state: string;
}
