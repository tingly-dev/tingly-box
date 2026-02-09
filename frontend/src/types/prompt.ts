// Prompt Feature Types

export interface User {
  id: string;
  name: string;
  email?: string;
  avatar?: string;
}

export type RecordingType =
  | 'code-review'
  | 'debug'
  | 'refactor'
  | 'test'
  | 'custom';

export interface Recording {
  id: string;
  timestamp: Date;
  user: User;
  project: string;
  type: RecordingType;
  content: string;
  duration: number;
  model: string;
  summary?: string;
}

export interface RecordingCalendarDay {
  date: Date;
  count: number;
  hasRecordings: boolean;
}

export type IDESource =
  | 'claude_code'
  | 'opencode'
  | 'vscode'
  | 'cursor'
  | 'codex'
  | 'antigravity'
  | 'amp'
  | 'kilo_code'
  | 'roo_code'
  | 'goose'
  | 'gemini_cli'
  | 'github_copilot'
  | 'clawdbot'
  | 'droid'
  | 'windsurf'
  | 'custom';

export type GroupingMode = 'flat' | 'auto' | 'pattern';

export interface GroupingStrategy {
  mode: GroupingMode;
  group_pattern?: string;
  min_files_for_split?: number;
}

export interface SkillLocation {
  id: string;
  name: string;              // Display name
  path: string;              // Full file system path
  ide_source: IDESource;     // Backend uses snake_case
  skill_count: number;       // Backend uses snake_case
  icon?: string;
  is_auto_discovered?: boolean;  // Backend uses snake_case
  is_installed?: boolean;    // Backend uses snake_case
  last_scanned_at?: Date;    // Backend uses snake_case
  grouping_strategy?: GroupingStrategy;  // Backend uses snake_case
}

export interface Skill {
  id: string;
  name: string;              // From filename
  filename: string;          // Full filename with extension
  path: string;              // Full file path
  location_id: string;       // Backend uses snake_case
  file_type: string;         // Backend uses snake_case
  description?: string;
  content_hash?: string;     // Backend uses snake_case
  size?: number;
  modified_at?: Date;        // Backend uses snake_case
  content?: string;          // File content
}

export interface SkillFilter {
  searchQuery: string;
  ideSource?: IDESource;
}

export interface RecordingFilter {
  searchQuery: string;
  userFilter?: string;
  projectFilter?: string;
  typeFilter?: RecordingType;
}

// ============================================
// Session Aggregation Types
// ============================================

// Account information extracted from metadata
export interface AccountInfo {
  id: string;
  name?: string;
  // For claude_code: the hash part of user_id
  claudeCodeUserId?: string;
  // For other scenarios: anthropic_user_id or similar
  anthropicUserId?: string;
}

// A group of rounds belonging to the same account + session
export interface SessionGroup {
  // Unique key for this group (account_id + session_id)
  groupKey: string;
  // Account information
  account: AccountInfo;
  // Session ID
  sessionId: string;
  // Project ID (optional)
  projectId?: string;
  // All rounds in this session, sorted by created_at
  rounds: PromptRoundListItem[];
  // Statistics
  stats: {
    totalRounds: number;
    totalTokens: number;
    firstMessageTime: string;
    lastMessageTime: string;
    models: string[];
    scenario: string;
  };
}

// Map of group keys to SessionGroups
export type SessionGroupMap = Map<string, SessionGroup>;

// ============================================
// Prompt Recording Types (Database-based)
// ============================================

export type ProtocolType = 'anthropic' | 'openai' | 'google';

// Lightweight type for list items (reduces initial data transfer)
export interface PromptRoundListItem {
  id: number;
  scenario: string;
  provider_name: string;
  model: string;
  protocol: ProtocolType;
  created_at: string;
  is_streaming: boolean;
  has_tool_use: boolean;
  // Full user_input for search and preview (truncated in UI only)
  user_input: string;
}

export interface PromptRoundItem {
  id: number;
  scenario: string;
  provider_uuid: string;
  provider_name: string;
  model: string;
  protocol: ProtocolType;
  request_id?: string;
  project_id?: string;
  session_id?: string;
  metadata?: Record<string, unknown>;
  round_index: number;
  user_input: string;
  round_result?: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  created_at: string;
  is_streaming: boolean;
  has_tool_use: boolean;
}

export interface PromptRoundListResponse {
  success: boolean;
  data: {
    rounds: PromptRoundItem[];
    total: number;
  };
  error?: string;
}

export interface PromptRoundListData {
  rounds: PromptRoundItem[];
  total: number;
}

export interface PromptUserInputsResponse {
  success: boolean;
  data: PromptRoundItem[];
}

export interface PromptSearchResponse {
  success: boolean;
  data: PromptRoundItem[];
}

export interface PromptDeleteResponse {
  success: boolean;
  message: string;
  data: {
    deleted_count: number;
    cutoff_days: number;
  };
}

export interface PromptFilters {
  scenario?: string;
  protocol?: ProtocolType;
  searchQuery?: string;
  project_id?: string;
  session_id?: string;
  limit?: number;
  offset?: number;
}
