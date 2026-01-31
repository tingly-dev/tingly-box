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
  | 'claude-code'
  | 'opencode'
  | 'vscode'
  | 'cursor'
  | 'custom';

export interface SkillLocation {
  id: string;
  name: string;              // Display name
  path: string;              // Full file system path
  ideSource: IDESource;
  skillCount: number;
  icon?: string;
}

export interface Skill {
  id: string;
  name: string;              // From filename
  filename: string;          // Full filename with extension
  path: string;              // Full file path
  locationId: string;
  fileType: string;          // File extension
  description?: string;
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

// IDE Source Icon Mapping
export const IDE_SOURCE_ICONS: Record<IDESource, string> = {
  'claude-code': '🎨',
  'opencode': '💻',
  'vscode': '💡',
  'cursor': '🎯',
  'custom': '📂',
};

// Recording Type Labels
export const RECORDING_TYPE_LABELS: Record<RecordingType, string> = {
  'code-review': 'Code Review',
  'debug': 'Debug',
  'refactor': 'Refactor',
  'test': 'Test',
  'custom': 'Custom',
};
