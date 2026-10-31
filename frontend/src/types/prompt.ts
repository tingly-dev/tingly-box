// Prompt Feature Types

// Re-export skill-related types from codegen
import type {
    Skill,
    SkillLocation,
    GroupingStrategy,
    DiscoveryResult,
    ScanResult,
} from '@/client';

// Re-export for convenience
export type {
    Skill,
    SkillLocation,
    GroupingStrategy,
    DiscoveryResult,
    ScanResult,
};

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

export interface GroupingStrategyCustom {
    mode: GroupingMode;
    group_pattern?: string;
    min_files_for_split?: number;
}

export interface SkillFilter {
    searchQuery: string;
    ideSource?: IDESource;
}
