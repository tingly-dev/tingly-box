// Enterprise API service

import axios, { type AxiosInstance } from 'axios';
import type {
  AuditLogFilters,
  AuditLog,
  ChangePasswordRequest,
  CreateTokenRequest,
  CreateUserRequest,
  EnterpriseStats,
  LoginRequest,
  LoginResponse,
  RefreshTokenRequest,
  RefreshTokenResponse,
  TokenListResponse,
  TokenWithUserData,
  UpdateTokenRequest,
  UpdateUserRequest,
  User,
  UserListResponse,
} from '../../types/enterprise';

class EnterpriseAPI {
  private client: AxiosInstance;

  constructor(baseURL: string = '') {
    this.client = axios.create({
      baseURL: baseURL + '/enterprise/api/v1',
      headers: {
        'Content-Type': 'application/json',
      },
    });

    // Add request interceptor to include auth token
    this.client.interceptors.request.use((config) => {
      const token = localStorage.getItem('enterprise_access_token');
      if (token) {
        config.headers.Authorization = `Bearer ${token}`;
      }
      return config;
    });

    // Add response interceptor to handle token refresh
    this.client.interceptors.response.use(
      (response) => response,
      async (error) => {
        const originalRequest = error.config;

        if (error.response?.status === 401 && !originalRequest._retry) {
          originalRequest._retry = true;

          try {
            const refreshToken = localStorage.getItem('enterprise_refresh_token');
            if (refreshToken) {
              const response = await this.refreshToken({ refresh_token: refreshToken });
              localStorage.setItem('enterprise_access_token', response.access_token);
              originalRequest.headers.Authorization = `Bearer ${response.access_token}`;
              return this.client(originalRequest);
            }
          } catch (refreshError) {
            // Refresh failed, clear tokens and redirect to login
            localStorage.removeItem('enterprise_access_token');
            localStorage.removeItem('enterprise_refresh_token');
            window.location.href = '/enterprise/login';
            return Promise.reject(refreshError);
          }
        }

        return Promise.reject(error);
      }
    );
  }

  // Authentication endpoints
  async login(data: LoginRequest): Promise<LoginResponse> {
    const response = await this.client.post<LoginResponse>('/auth/login', data);
    return response.data;
  }

  async refreshToken(data: RefreshTokenRequest): Promise<RefreshTokenResponse> {
    const response = await this.client.post<RefreshTokenResponse>('/auth/refresh', data);
    return response.data;
  }

  async logout(): Promise<void> {
    await this.client.post('/auth/logout');
  }

  async getCurrentUser(): Promise<User> {
    const response = await this.client.get<User>('/auth/me');
    return response.data;
  }

  // User management endpoints
  async listUsers(page: number = 1, pageSize: number = 20): Promise<UserListResponse> {
    const response = await this.client.get<UserListResponse>('/users', {
      params: { page, page_size: pageSize },
    });
    return response.data;
  }

  async createUser(data: CreateUserRequest): Promise<User> {
    const response = await this.client.post<User>('/users', data);
    return response.data;
  }

  async getUser(id: number): Promise<User> {
    const response = await this.client.get<User>(`/users/${id}`);
    return response.data;
  }

  async updateUser(id: number, data: UpdateUserRequest): Promise<User> {
    const response = await this.client.put<User>(`/users/${id}`, data);
    return response.data;
  }

  async deleteUser(id: number): Promise<void> {
    await this.client.delete(`/users/${id}`);
  }

  async activateUser(id: number): Promise<void> {
    await this.client.post(`/users/${id}/activate`);
  }

  async deactivateUser(id: number): Promise<void> {
    await this.client.post(`/users/${id}/deactivate`);
  }

  async resetPassword(id: number): Promise<{ new_password: string; message: string; should_change: boolean }> {
    const response = await this.client.post<{ new_password: string; message: string; should_change: boolean }>(
      `/users/${id}/password`
    );
    return response.data;
  }

  // Token management endpoints
  async listTokens(page: number = 1, pageSize: number = 20): Promise<TokenListResponse> {
    const response = await this.client.get<TokenListResponse>('/tokens', {
      params: { page, page_size: pageSize },
    });
    return response.data;
  }

  async createToken(data: CreateTokenRequest): Promise<{
    token: string;
    token_prefix: string;
    token_id: string;
    name: string;
    scopes: string;
    expires_at?: string;
  }> {
    const response = await this.client.post('/tokens', data);
    return response.data;
  }

  async getToken(id: number): Promise<TokenWithUserData> {
    const response = await this.client.get<TokenWithUserData>(`/tokens/${id}`);
    return response.data;
  }

  async updateToken(id: number, data: UpdateTokenRequest): Promise<TokenWithUserData> {
    const response = await this.client.put<TokenWithUserData>(`/tokens/${id}`, data);
    return response.data;
  }

  async deleteToken(id: number): Promise<void> {
    await this.client.delete(`/tokens/${id}`);
  }

  // My tokens endpoints
  async listMyTokens(page: number = 1, pageSize: number = 20): Promise<TokenListResponse> {
    const response = await this.client.get<TokenListResponse>('/my-tokens', {
      params: { page, page_size: pageSize },
    });
    return response.data;
  }

  async createMyToken(data: CreateTokenRequest): Promise<{
    token: string;
    token_prefix: string;
    token_id: string;
    name: string;
    scopes: string;
    expires_at?: string;
  }> {
    const response = await this.client.post('/my-tokens', data);
    return response.data;
  }

  async deleteMyToken(uuid: string): Promise<void> {
    await this.client.delete(`/my-tokens/${uuid}`);
  }

  // Audit log endpoints
  async listAuditLogs(
    page: number = 1,
    pageSize: number = 20,
    filters?: AuditLogFilters
  ): Promise<{ logs: AuditLog[]; total: number; page: number; size: number }> {
    const response = await this.client.get('/audit', {
      params: { page, page_size: pageSize, ...filters },
    });
    return response.data;
  }

  async getAuditLog(id: number): Promise<AuditLog> {
    const response = await this.client.get<AuditLog>(`/audit/${id}`);
    return response.data;
  }

  // Stats endpoint
  async getStats(): Promise<EnterpriseStats> {
    const response = await this.client.get<EnterpriseStats>('/stats');
    return response.data;
  }

  // Change password
  async changePassword(data: ChangePasswordRequest): Promise<void> {
    await this.client.post('/auth/change-password', data);
  }
}

// Create singleton instance
export const enterpriseAPI = new EnterpriseAPI();

// Export class for testing
export { EnterpriseAPI };
