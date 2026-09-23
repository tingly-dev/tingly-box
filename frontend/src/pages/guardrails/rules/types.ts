// Shared types + constants for the Guardrails rules page. Imported only by
// `pages/guardrails/**` (RulesPage / GroupsPage), never by eager code outside
// `pages/**` — see frontend/CLAUDE.md on nav-level exports.

export type PolicyGroup = {
    id: string;
    name?: string;
    severity?: string;
    enabled?: boolean;
};

export type GuardrailsPolicy = {
    id: string;
    name?: string;
    groups?: string[];
    kind: 'resource_access' | 'command_execution' | 'content' | 'operation';
    enabled?: boolean;
    scope?: {
        scenarios?: string[];
    };
    match?: {
        tool_names?: string[];
        actions?: { include?: string[]; exclude?: string[] };
        resources?: { type?: string; mode?: string; values?: string[] };
        terms?: string[];
        credential_refs?: string[];
        patterns?: string[];
        pattern_mode?: string;
        case_sensitive?: boolean;
    };
    verdict?: string;
    reason?: string;
};

export type DisplayPolicy = GuardrailsPolicy & {
    isBuiltin?: boolean;
    builtinSummary?: string;
};

export type RegistryPolicyEntry = {
    id: string;
    name?: string;
    reason?: string;
    path: string;
};

export type EditorState = {
    id: string;
    name: string;
    groups: string[];
    kind: 'resource_access' | 'command_execution' | 'content' | '';
    enabled: boolean;
    verdict: string;
    scenarios: string[];
    toolNames: string;
    actions: string[];
    commandTerms: string;
    resources: string;
    resourceMode: string;
    patterns: string;
    patternMode: string;
    caseSensitive: boolean;
    reason: string;
};

export type EditorListField = 'toolNames' | 'commandTerms' | 'resources' | 'patterns';

export type OversizedListField = {
    values: string[];
    preview: string[];
    total: number;
};

export type PreparedEditorState = {
    state: EditorState;
    oversized: Partial<Record<EditorListField, OversizedListField>>;
};

export const MAX_SUMMARY_VALUES = 2;
export const MAX_SUMMARY_CHARS = 140;
// Heuristic editability thresholds for large generated guardrail lists.
// These optimize for Rules page responsiveness, not persistence size: once a
// list grows into the hundreds of entries, joining it into one large string,
// repeatedly splitting it on render, and mounting one editable row per value
// starts to make the dialog sluggish. We keep normal hand-authored policies
// fully editable, but switch machine-generated blocklists into preview mode
// before they become expensive to render.
//
// The specific cutoffs are intentionally conservative and were chosen around
// the datasets introduced in this PR:
// - medium generated lists such as the NuGet and RubyGems malicious package
//   blocklists are already well above 400 entries and should use preview mode
// - very large PyPI and npm blocklists always use preview mode
// - smaller policies remain inline-editable
export const MAX_INLINE_LIST_ITEMS = 400;
// Secondary guardrail for cases where the item count is moderate but the total
// joined text is still large enough to hurt UI responsiveness.
export const MAX_INLINE_LIST_CHARS = 16000;
export const OVERSIZED_LIST_PREVIEW_ITEMS = 25;

export const resourceAccessActionOptions = [
    {
        value: 'read',
        label: 'Read',
        description: 'Inspect or list files, directories, and other protected paths.',
    },
    {
        value: 'write',
        label: 'Write',
        description: 'Create or modify files, directories, or configuration content.',
    },
    {
        value: 'delete',
        label: 'Delete',
        description: 'Remove files, directories, or other protected resources.',
    },
    {
        value: 'network',
        label: 'Network',
        description: 'Fetch from or send data to remote endpoints.',
    },
] as const;

export const commandExecutionActionOptions = [
    {
        value: 'execute',
        label: 'Execute',
        description: 'Match explicit command patterns such as rm -rf, curl | sh, or python -c.',
    },
    {
        value: 'install',
        label: 'Install',
        description: 'Match normalized package, tool, or extension install commands such as npm install or pip install.',
    },
] as const;

export const DEFAULT_GROUP_ID = 'default';
export const PENDING_REGISTRY_INSTALLS_STORAGE_KEY = 'guardrails.pendingRegistryInstalls';
