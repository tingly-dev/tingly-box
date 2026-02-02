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
