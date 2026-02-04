import axios from 'axios';
import type {
  AuditLogsResponse,
  StatsResponse,
  RateLimitStats,
  TokenInfo,
  TokenValidationResult,
} from '@/types';

const API_BASE = '/admin';

// Create axios instance with auth header
const createApi = (token: string) =>
  axios.create({
    baseURL: API_BASE,
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
  });

// Auth context for token
let authToken: string | null = null;

export const setAuthToken = (token: string) => {
  authToken = token;
};

export const getAuthToken = () => authToken;

export const clearAuthToken = () => {
  authToken = null;
};

const getApi = () => {
  if (!authToken) {
    throw new Error('No authentication token set');
  }
  return createApi(authToken);
};

// Audit logs API
export const getAuditLogs = async (
  page = 1,
  limit = 50,
  filters?: { action?: string; user_id?: string; start_date?: string; end_date?: string }
): Promise<AuditLogsResponse> => {
  const api = getApi();
  const params: Record<string, string> = {
    page: page.toString(),
    limit: limit.toString(),
  };
  if (filters?.action) params.action = filters.action;
  if (filters?.user_id) params.user_id = filters.user_id;
  if (filters?.start_date) params.start_date = filters.start_date;
  if (filters?.end_date) params.end_date = filters.end_date;

  const response = await api.get('/logs', { params });
  return response.data;
};

// Stats API
export const getStats = async (): Promise<StatsResponse> => {
  const api = getApi();
  const response = await api.get('/stats');
  return response.data;
};

// Rate limit stats API
export const getRateLimitStats = async (): Promise<RateLimitStats> => {
  const api = getApi();
  const response = await api.get('/ratelimit/stats');
  return response.data;
};

// Reset rate limit for IP
export const resetRateLimit = async (ip: string): Promise<{ status: string; message: string }> => {
  const api = getApi();
  const response = await api.post('/ratelimit/reset', { ip });
  return response.data;
};

// Generate token API
export const generateToken = async (
  clientId: string,
  expiryHours?: number
): Promise<{ token: TokenInfo; status: string; message: string }> => {
  const api = getApi();
  const response = await api.post('/tokens/generate', {
    client_id: clientId,
    expiry_hours: expiryHours,
  });
  return response.data;
};

// Validate token API
export const validateToken = async (
  token: string
): Promise<{ result: TokenValidationResult }> => {
  const api = getApi();
  const response = await api.post('/tokens/validate', { token });
  return response.data;
};

// Revoke token API
export const revokeToken = async (
  clientId: string
): Promise<{ status: string; message: string }> => {
  const api = getApi();
  const response = await api.post('/tokens/revoke', { client_id: clientId });
  return response.data;
};

// Login API
export const login = async (token: string): Promise<{ success: boolean; token: string }> => {
  const response = await axios.post(`${API_BASE.replace('/admin', '')}/opsx/handshake`, null, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });
  if (response.status === 200) {
    return { success: true, token };
  }
  return { success: false, token: '' };
};
