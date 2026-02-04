// Audit log types
export interface AuditLogEntry {
  timestamp: string;
  level: string;
  action: string;
  user_id: string;
  client_ip: string;
  session_id: string;
  request_id: string;
  success: boolean;
  duration_ms: number;
  details?: Record<string, unknown>;
}

export interface AuditLogsResponse {
  logs: AuditLogEntry[];
  page: number;
  limit: number;
  total: number;
  total_pages: number;
}

// Stats types
export interface StatsResponse {
  total_sessions: number;
  active_sessions: number;
  completed_sessions: number;
  failed_sessions: number;
  closed_sessions: number;
  recent_actions: Record<string, number>;
  uptime: string;
  rate_limit_stats: Record<string, unknown>;
}

// Rate limit types
export interface RateLimitStats {
  stats: {
    total_ips_tracked: number;
    currently_blocked: number;
    max_attempts: number;
    window_size: string;
    block_duration: string;
  };
}

// Token types
export interface TokenInfo {
  token: string;
  client_id: string;
  expires_at: string;
  created_at: string;
}

export interface GenerateTokenRequest {
  client_id: string;
  description?: string;
  expiry_hours?: number;
}

export interface ResetRateLimitRequest {
  ip: string;
}

export interface TokenValidationResult {
  valid: boolean;
  client_id?: string;
  message?: string;
}
