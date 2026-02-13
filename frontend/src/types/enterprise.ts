// Enterprise types and interfaces

export type Role = 'admin' | 'user' | 'readonly';

export interface User {
  id: number;
  uuid: string;
  username: string;
  email: string;
  role: Role;
  full_name: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  last_login_at?: string;
}

export type Scope =
  | 'read:providers'
  | 'write:providers'
  | 'read:rules'
  | 'write:rules'
  | 'read:usage'
  | 'read:users'
  | 'write:users'
  | 'read:tokens'
  | 'write:tokens'
  | 'admin:all';

export interface APIToken {
  id: number;
  uuid: string;
  user_id: number;
  token_prefix: string;
  name: string;
  scopes: string; // JSON string
  expires_at?: string;
  last_used_at?: string;
  is_active: boolean;
  created_at: string;
  username?: string;
  email?: string;
}

export interface TokenWithUserData extends APIToken {
  username: string;
  email: string;
}

export interface Session {
  id: number;
  uuid: string;
  user_id: number;
  expires_at: string;
  created_at: string;
}

export interface AuditLog {
  id: number;
  user_id?: number;
  action: string;
  resource_type: string;
  resource_id: string;
  details: string;
  ip_address: string;
  user_agent: string;
  status: string;
  created_at: string;
  user?: User;
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  access_token: string;
  refresh_token: string;
  expires_at: string;
  user: User;
}

export interface RefreshTokenRequest {
  refresh_token: string;
}

export interface RefreshTokenResponse {
  access_token: string;
  expires_at: string;
}

export interface CreateUserRequest {
  username: string;
  email: string;
  password: string;
  full_name?: string;
  role: Role;
}

export interface UpdateUserRequest {
  full_name?: string;
  role?: Role;
}

export interface ChangePasswordRequest {
  current_password: string;
  new_password: string;
}

export interface CreateTokenRequest {
  name: string;
  scopes: Scope[];
  expires_at?: string;
  user_id?: number;
}

export interface UpdateTokenRequest {
  name?: string;
  scopes?: Scope[];
  expires_at?: string;
}

export interface TokenListResponse {
  tokens: TokenWithUserData[];
  total: number;
  page: number;
  size: number;
}

export interface UserListResponse {
  users: User[];
  total: number;
  page: number;
  size: number;
}

export interface AuditLogFilters {
  user_id?: number;
  action?: string;
  resource_type?: string;
  status?: string;
  start_date?: string;
  end_date?: string;
}

export interface EnterpriseStats {
  total_users: number;
  active_users: number;
  total_tokens: number;
  active_tokens: number;
  total_audit_logs: number;
  enterprise_enabled: boolean;
}
