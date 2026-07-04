import {
    Box,
    Collapse,
    Divider,
    FormControl,
    MenuItem,
    Select,
    IconButton,
    InputAdornment,
    Stack,
    Switch,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import { InfoOutlined as InfoOutlinedIcon } from '@/components/icons';
import { ExpandMore as ExpandMoreIcon } from '@/components/icons';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { has1M, with1M } from '@/components/rule-card/modelNameUtils';

// ClaudeCodePrefs mirrors the Go struct in internal/agent/prefs.go.
// Keys are the literal Claude Code env var names so the object can be
// dropped straight into JSON.stringify({env: prefs}) and round-tripped
// through the backend without an intermediate mapping layer.
export interface ClaudeCodePrefs {
    ANTHROPIC_MODEL?: string;
    ANTHROPIC_DEFAULT_HAIKU_MODEL?: string;
    ANTHROPIC_DEFAULT_SONNET_MODEL?: string;
    ANTHROPIC_DEFAULT_OPUS_MODEL?: string;
    CLAUDE_CODE_SUBAGENT_MODEL?: string;

    API_TIMEOUT_MS?: string;
    CLAUDE_CODE_MAX_OUTPUT_TOKENS?: string;
    MAX_THINKING_TOKENS?: string;
    BASH_DEFAULT_TIMEOUT_MS?: string;
    BASH_MAX_TIMEOUT_MS?: string;
    MCP_TIMEOUT?: string;
    MCP_TOOL_TIMEOUT?: string;
    MAX_MCP_OUTPUT_TOKENS?: string;

    CLAUDE_CODE_AUTO_COMPACT_WINDOW?: string;
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE?: string;

    DISABLE_TELEMETRY?: string;
    DISABLE_ERROR_REPORTING?: string;
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC?: string;
    DISABLE_AUTOUPDATER?: string;
    USE_BUILTIN_RIPGREP?: string;

    HTTP_PROXY?: string;
    HTTPS_PROXY?: string;
    NO_PROXY?: string;
}

type PrefsKey = keyof ClaudeCodePrefs;
export type ClaudeCodeDefaultMode = 'acceptEdits' | 'bypassPermissions' | 'default' | 'delegate' | 'dontAsk' | 'plan' | 'auto';
type Group = 'behavior' | 'model' | 'limits' | 'switches' | 'network';
type Kind = 'model' | 'int' | 'text' | 'bool';
type Lang = 'zh' | 'en';

// ── Field structure (language-agnostic) ────────────────────────────────
// Adding a new env: append a row here AND add entries in FIELDS_TEXT_ZH /
// FIELDS_TEXT_EN below (TS will flag the missing keys).

interface FieldStruct {
    envName: PrefsKey;
    group: Group;
    kind: Kind;
    unit?: string;
    advanced?: boolean; // Mark advanced fields that should be collapsed by default
}

const FIELD_STRUCT: FieldStruct[] = [
    // Models (always visible - most commonly adjusted)
    { envName: 'ANTHROPIC_MODEL', group: 'model', kind: 'model', advanced: false },
    { envName: 'ANTHROPIC_DEFAULT_HAIKU_MODEL', group: 'model', kind: 'model', advanced: false },
    { envName: 'ANTHROPIC_DEFAULT_SONNET_MODEL', group: 'model', kind: 'model', advanced: false },
    { envName: 'ANTHROPIC_DEFAULT_OPUS_MODEL', group: 'model', kind: 'model', advanced: false },
    { envName: 'CLAUDE_CODE_SUBAGENT_MODEL', group: 'model', kind: 'model', advanced: false },
    // Limits (advanced - rarely changed)
    { envName: 'API_TIMEOUT_MS', group: 'limits', kind: 'int', unit: 'ms', advanced: true },
    { envName: 'CLAUDE_CODE_MAX_OUTPUT_TOKENS', group: 'limits', kind: 'int', unit: 'tokens', advanced: true },
    { envName: 'MAX_THINKING_TOKENS', group: 'limits', kind: 'int', unit: 'tokens', advanced: true },
    { envName: 'BASH_DEFAULT_TIMEOUT_MS', group: 'limits', kind: 'int', unit: 'ms', advanced: true },
    { envName: 'BASH_MAX_TIMEOUT_MS', group: 'limits', kind: 'int', unit: 'ms', advanced: true },
    { envName: 'MCP_TIMEOUT', group: 'limits', kind: 'int', unit: 'ms', advanced: true },
    { envName: 'MCP_TOOL_TIMEOUT', group: 'limits', kind: 'int', unit: 'ms', advanced: true },
    { envName: 'MAX_MCP_OUTPUT_TOKENS', group: 'limits', kind: 'int', unit: 'tokens', advanced: true },
    // Auto-compact (commonly adjusted - not advanced)
    { envName: 'CLAUDE_CODE_AUTO_COMPACT_WINDOW', group: 'model', kind: 'int', unit: 'tokens', advanced: false },
    { envName: 'CLAUDE_AUTOCOMPACT_PCT_OVERRIDE', group: 'model', kind: 'int', unit: '%', advanced: false },
    // Switches (advanced - usually don't need to change)
    { envName: 'DISABLE_TELEMETRY', group: 'switches', kind: 'bool', advanced: true },
    { envName: 'DISABLE_ERROR_REPORTING', group: 'switches', kind: 'bool', advanced: true },
    { envName: 'CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC', group: 'switches', kind: 'bool', advanced: true },
    { envName: 'DISABLE_AUTOUPDATER', group: 'switches', kind: 'bool', advanced: true },
    { envName: 'USE_BUILTIN_RIPGREP', group: 'switches', kind: 'bool', advanced: true },
    // Network proxy (advanced - rarely needed)
    { envName: 'HTTP_PROXY', group: 'network', kind: 'text', advanced: true },
    { envName: 'HTTPS_PROXY', group: 'network', kind: 'text', advanced: true },
    { envName: 'NO_PROXY', group: 'network', kind: 'text', advanced: true },
];

// ── Localized text bundles ─────────────────────────────────────────────
// Kept inline rather than in i18n/locales/* — the strings are dense, dev-
// facing, and likely to churn as we tune the wording. Two parallel maps
// avoids the i18n locale file becoming a junk drawer.

interface FieldText {
    label: string;
    purpose: string;
    tooltip: string;
    placeholder?: string;
}

type FieldTextMap = Record<PrefsKey, FieldText>;

const FIELDS_TEXT_ZH: FieldTextMap = {
    ANTHROPIC_MODEL: {
        label: '默认模型',
        purpose: '未指定具体场景时使用的兜底模型',
        tooltip: 'Claude Code 在没有专门路由时回退到这个模型。tb 通常映射到 tingly/cc 或 tingly/cc-default。',
        placeholder: 'tingly/cc',
    },
    ANTHROPIC_DEFAULT_HAIKU_MODEL: {
        label: 'Haiku 槽位',
        purpose: '轻量调用（如生成 commit message、文件摘要）使用的模型',
        tooltip: 'Claude Code 内部对一些便宜的辅助调用走 haiku 槽位。tb 把它路由到 tingly/cc-haiku。',
        placeholder: 'tingly/cc-haiku',
    },
    ANTHROPIC_DEFAULT_SONNET_MODEL: {
        label: 'Sonnet 槽位',
        purpose: '主力槽位 — 大部分对话和代码生成走这里',
        tooltip: 'Claude Code 的默认主力。除非显式选其他模型，正常会话都用 sonnet 槽位。',
        placeholder: 'tingly/cc-sonnet',
    },
    ANTHROPIC_DEFAULT_OPUS_MODEL: {
        label: 'Opus 槽位',
        purpose: '复杂推理（如 plan 模式、深度分析）使用的模型',
        tooltip: '相对昂贵但更强的推理模型。Claude Code 在显式调用 opus 时使用。',
        placeholder: 'tingly/cc-opus',
    },
    CLAUDE_CODE_SUBAGENT_MODEL: {
        label: '子 Agent 模型',
        purpose: '通过 Task 工具派生的子 Agent 使用的模型',
        tooltip: '子 Agent 用于并发研究、独立子任务。可以单独指定一个更便宜或更强的模型。',
        placeholder: 'tingly/cc-subagent',
    },
    API_TIMEOUT_MS: {
        label: 'API 请求超时',
        purpose: '单次 API 请求最长等待时间',
        tooltip: '官方默认 120000 (2 分钟)。tb 走代理常有长任务，推荐拉到 3000000 (50 分钟)。',
        placeholder: '3000000',
    },
    CLAUDE_CODE_MAX_OUTPUT_TOKENS: {
        label: '最大输出 token',
        purpose: '单条回复输出的 token 上限',
        tooltip: '太小可能被截断，太大会浪费配额。tb 推荐 32000。',
        placeholder: '32000',
    },
    MAX_THINKING_TOKENS: {
        label: '思考 token 预算',
        purpose: 'Extended Thinking 模式下的思考 token 上限',
        tooltip: '留空表示用模型默认值。仅对支持 thinking 的模型有效。',
        placeholder: '(空 = 模型默认)',
    },
    BASH_DEFAULT_TIMEOUT_MS: {
        label: 'Bash 默认超时',
        purpose: 'Bash 工具单次执行的默认超时',
        tooltip: '官方默认 120000。长跑脚本（如 npm install）若超时可以调高。',
        placeholder: '120000',
    },
    BASH_MAX_TIMEOUT_MS: {
        label: 'Bash 最大超时',
        purpose: 'Bash 工具允许指定的最长超时',
        tooltip: 'Claude 自己设置 timeout 时的上限。',
        placeholder: '600000',
    },
    MCP_TIMEOUT: {
        label: 'MCP 连接超时',
        purpose: 'MCP server 启动/响应的超时',
        tooltip: '官方默认 30000。MCP server 启动慢可以调高。',
        placeholder: '30000',
    },
    MCP_TOOL_TIMEOUT: {
        label: 'MCP 工具超时',
        purpose: '单次 MCP 工具调用的超时',
        tooltip: '官方默认 10000。',
        placeholder: '10000',
    },
    MAX_MCP_OUTPUT_TOKENS: {
        label: 'MCP 输出上限',
        purpose: 'MCP 工具单次返回内容的 token 上限',
        tooltip: '官方默认 8192。超过会被截断。',
        placeholder: '8192',
    },
    CLAUDE_CODE_AUTO_COMPACT_WINDOW: {
        label: '自动压缩窗口',
        purpose: '上下文自动压缩的目标窗口大小',
        tooltip: 'tb 默认 200000（1M 模型自动调整为 1000000）。当触发自动压缩时，会保留最近的 N 个 token。调高可以保留更多上下文，但会占用更多配额。',
        placeholder: '200000',
    },
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: {
        label: '自动压缩阈值',
        purpose: '上下文自动压缩的触发百分比',
        tooltip: 'tb 默认 80。当上下文使用率达到该百分比时触发自动压缩。调低则更早压缩，调高则更晚。设为 0 禁用。',
        placeholder: '80',
    },
    DISABLE_TELEMETRY: {
        label: '禁用遥测',
        purpose: '关闭 Anthropic 官方遥测上报',
        tooltip: 'tb 默认开启此项以保护内网/隐私环境。',
    },
    DISABLE_ERROR_REPORTING: {
        label: '禁用错误上报',
        purpose: '关闭异常自动上报到 Anthropic',
        tooltip: 'tb 默认开启此项。',
    },
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: {
        label: '禁用非必要流量',
        purpose: '关闭所有非业务请求（更新检查、提示、统计等）',
        tooltip: '最干净的模式，只保留模型调用本身。tb 默认开启。',
    },
    DISABLE_AUTOUPDATER: {
        label: '禁用自动更新',
        purpose: 'Claude Code 不再自动检查/下载新版本',
        tooltip: '通常用于固定版本的部署环境。',
    },
    USE_BUILTIN_RIPGREP: {
        label: '使用内置 ripgrep',
        purpose: 'Claude Code 自带的 ripgrep 优先于系统 PATH',
        tooltip: '官方默认开启。仅在需要用系统自定义 ripgrep 时关闭。',
    },
    HTTP_PROXY: {
        label: 'HTTP 代理',
        purpose: 'Claude Code 发出 HTTP 请求时使用的代理地址',
        tooltip: '格式：http://host:port。留空则继承系统代理设置。注意：系统代理不会自动排除 localhost，可能导致向本地网关发起的请求被代理拦截而 502。',
        placeholder: 'http://proxy.example.com:8080',
    },
    HTTPS_PROXY: {
        label: 'HTTPS 代理',
        purpose: 'Claude Code 发出 HTTPS 请求时使用的代理地址',
        tooltip: '格式：http://host:port 或 https://host:port。留空则继承系统代理设置。',
        placeholder: 'http://proxy.example.com:8080',
    },
    NO_PROXY: {
        label: '代理排除列表',
        purpose: '不走代理的主机/域名列表',
        tooltip: '逗号分隔，如 "localhost,127.0.0.1,::1"。tb 启动时会自动把 localhost/127.0.0.1/::1 追加进来，即使此处留空也会生效。建议留空（由 tb 自动管理），仅在需要额外排除内网域名时填写。',
        placeholder: 'localhost,127.0.0.1,::1',
    },
};

const FIELDS_TEXT_EN: FieldTextMap = {
    ANTHROPIC_MODEL: {
        label: 'Default model',
        purpose: 'Fallback model used when no specific slot applies',
        tooltip: 'What Claude Code reaches for when no specialized routing matches. tb typically maps this to tingly/cc or tingly/cc-default.',
        placeholder: 'tingly/cc',
    },
    ANTHROPIC_DEFAULT_HAIKU_MODEL: {
        label: 'Haiku slot',
        purpose: 'Lightweight tasks like commit messages and summaries',
        tooltip: 'Claude Code routes cheap auxiliary calls to the haiku slot. tb points it at tingly/cc-haiku.',
        placeholder: 'tingly/cc-haiku',
    },
    ANTHROPIC_DEFAULT_SONNET_MODEL: {
        label: 'Sonnet slot',
        purpose: 'Workhorse slot — most chat and code generation lands here',
        tooltip: "Claude Code's default. Unless you pick another model explicitly, normal sessions use the sonnet slot.",
        placeholder: 'tingly/cc-sonnet',
    },
    ANTHROPIC_DEFAULT_OPUS_MODEL: {
        label: 'Opus slot',
        purpose: 'Heavier reasoning (plan mode, deep analysis)',
        tooltip: 'More expensive but stronger model. Claude Code uses it when opus is explicitly requested.',
        placeholder: 'tingly/cc-opus',
    },
    CLAUDE_CODE_SUBAGENT_MODEL: {
        label: 'Sub-agent model',
        purpose: 'Model used by sub-agents spawned via the Task tool',
        tooltip: 'Sub-agents handle parallel research and independent subtasks. You can give them a cheaper or stronger model.',
        placeholder: 'tingly/cc-subagent',
    },
    API_TIMEOUT_MS: {
        label: 'API request timeout',
        purpose: 'Maximum time to wait for a single API response',
        tooltip: 'Anthropic default is 120000 (2 min). Long-running proxied tasks under tb usually want this bumped to 3000000 (50 min).',
        placeholder: '3000000',
    },
    CLAUDE_CODE_MAX_OUTPUT_TOKENS: {
        label: 'Max output tokens',
        purpose: 'Upper bound on tokens in a single response',
        tooltip: 'Too small truncates; too large wastes quota. tb recommends 32000.',
        placeholder: '32000',
    },
    MAX_THINKING_TOKENS: {
        label: 'Thinking token budget',
        purpose: 'Token budget for Extended Thinking',
        tooltip: 'Leave blank to use the model default. Only meaningful for thinking-capable models.',
        placeholder: '(blank = model default)',
    },
    BASH_DEFAULT_TIMEOUT_MS: {
        label: 'Bash default timeout',
        purpose: 'Default timeout for a single Bash tool call',
        tooltip: 'Anthropic default is 120000. Raise it if long scripts (e.g. npm install) tend to time out.',
        placeholder: '120000',
    },
    BASH_MAX_TIMEOUT_MS: {
        label: 'Bash max timeout',
        purpose: 'Ceiling for any Bash timeout Claude requests',
        tooltip: 'The upper limit when Claude sets its own timeout on a Bash call.',
        placeholder: '600000',
    },
    MCP_TIMEOUT: {
        label: 'MCP connect timeout',
        purpose: 'Timeout for MCP server startup and responses',
        tooltip: 'Anthropic default is 30000. Raise it for slow-starting MCP servers.',
        placeholder: '30000',
    },
    MCP_TOOL_TIMEOUT: {
        label: 'MCP tool timeout',
        purpose: 'Timeout for a single MCP tool invocation',
        tooltip: 'Anthropic default is 10000.',
        placeholder: '10000',
    },
    MAX_MCP_OUTPUT_TOKENS: {
        label: 'MCP output cap',
        purpose: 'Max tokens returned from one MCP tool call',
        tooltip: 'Anthropic default is 8192. Anything larger is truncated.',
        placeholder: '8192',
    },
    CLAUDE_CODE_AUTO_COMPACT_WINDOW: {
        label: 'Auto-compact window',
        purpose: 'Target window size for context auto-compaction',
        tooltip: 'tb default is 200000 (auto-adjusted to 1000000 for 1M models). When auto-compaction triggers, keeps the most recent N tokens. Higher values preserve more context but consume more quota.',
        placeholder: '200000',
    },
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: {
        label: 'Auto-compact threshold',
        purpose: 'Context auto-compact trigger percentage',
        tooltip: 'tb default is 80. Triggers auto-compaction when context usage reaches this %. Lower = earlier compaction, higher = later. Set to 0 to disable.',
        placeholder: '80',
    },
    DISABLE_TELEMETRY: {
        label: 'Disable telemetry',
        purpose: 'Turn off Anthropic-side telemetry reporting',
        tooltip: 'tb enables this by default to keep internal/private deployments quiet.',
    },
    DISABLE_ERROR_REPORTING: {
        label: 'Disable error reporting',
        purpose: 'Turn off automatic crash uploads to Anthropic',
        tooltip: 'tb enables this by default.',
    },
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: {
        label: 'Disable non-essential traffic',
        purpose: 'Suppress update checks, surveys, and other ambient calls',
        tooltip: 'Cleanest mode — only the model calls themselves go out. tb enables this by default.',
    },
    DISABLE_AUTOUPDATER: {
        label: 'Disable auto-updater',
        purpose: 'Stop Claude Code from checking for new versions',
        tooltip: 'Typical for pinned-version deployments.',
    },
    USE_BUILTIN_RIPGREP: {
        label: 'Use built-in ripgrep',
        purpose: "Prefer Claude Code's bundled ripgrep over system PATH",
        tooltip: "On by default. Disable only if you need to use a custom system ripgrep.",
    },
    HTTP_PROXY: {
        label: 'HTTP proxy',
        purpose: 'Proxy used for outbound HTTP requests from Claude Code',
        tooltip: 'Format: http://host:port. Leave blank to inherit system proxy settings. Note: system proxies do not automatically bypass localhost, which can cause 502s when proxying requests to the local tb gateway.',
        placeholder: 'http://proxy.example.com:8080',
    },
    HTTPS_PROXY: {
        label: 'HTTPS proxy',
        purpose: 'Proxy used for outbound HTTPS requests from Claude Code',
        tooltip: 'Format: http://host:port or https://host:port. Leave blank to inherit system proxy settings.',
        placeholder: 'http://proxy.example.com:8080',
    },
    NO_PROXY: {
        label: 'No-proxy list',
        purpose: 'Comma-separated hosts that bypass the proxy',
        tooltip: 'e.g. "localhost,127.0.0.1,::1". tb automatically appends localhost/127.0.0.1/::1 at startup even if left blank. Leave blank to let tb manage it automatically; only set this if you need to bypass additional internal hosts.',
        placeholder: 'localhost,127.0.0.1,::1',
    },
};

interface SectionText { title: string; hint: string }
type SectionTextMap = Record<Group, SectionText>;

interface DefaultModeOptionText {
    label: string;
    description: string;
}

const DEFAULT_MODE_OPTIONS: ClaudeCodeDefaultMode[] = ['acceptEdits', 'default', 'plan', 'auto', 'delegate', 'dontAsk', 'bypassPermissions'];

const DEFAULT_MODE_TEXT_ZH: Record<ClaudeCodeDefaultMode, DefaultModeOptionText> = {
    acceptEdits: { label: '接受编辑（推荐）', description: '自动接受文件编辑，其他高风险操作仍按 Claude Code 规则处理。' },
    default: { label: '默认', description: '使用 Claude Code 官方默认权限行为。' },
    plan: { label: '计划模式', description: '默认进入 plan mode，先规划再执行。' },
    auto: { label: '自动', description: '由 Claude Code 自动选择权限行为。' },
    delegate: { label: '委托', description: '把权限决策委托给 Claude Code 支持的外部流程。' },
    dontAsk: { label: '不询问', description: '避免交互式询问；适合无人值守场景。' },
    bypassPermissions: { label: '绕过权限', description: '跳过权限检查；仅在完全可信环境中使用。' },
};

const DEFAULT_MODE_TEXT_EN: Record<ClaudeCodeDefaultMode, DefaultModeOptionText> = {
    acceptEdits: { label: 'Accept edits (recommended)', description: 'Automatically accepts file edits while leaving riskier actions to Claude Code rules.' },
    default: { label: 'Default', description: 'Use Claude Code\'s built-in default permission behavior.' },
    plan: { label: 'Plan mode', description: 'Start in plan mode by default before implementation.' },
    auto: { label: 'Auto', description: 'Let Claude Code choose the permission behavior automatically.' },
    delegate: { label: 'Delegate', description: 'Delegate permission decisions to Claude Code\'s supported external flow.' },
    dontAsk: { label: 'Don\'t ask', description: 'Avoid interactive prompts; useful for unattended setups.' },
    bypassPermissions: { label: 'Bypass permissions', description: 'Skip permission checks; use only in fully trusted environments.' },
};

const DEFAULT_MODE_TEXT: Record<Lang, Record<ClaudeCodeDefaultMode, DefaultModeOptionText>> = {
    zh: DEFAULT_MODE_TEXT_ZH,
    en: DEFAULT_MODE_TEXT_EN,
};

const DEFAULT_MODE_SECTION_TEXT: Record<Lang, SectionText> = {
    zh: {
        title: '默认权限模式',
        hint: '写入 settings.json 的 defaultMode；tb 推荐 acceptEdits。',
    },
    en: {
        title: 'Default permission mode',
        hint: 'Writes defaultMode in settings.json; tb recommends acceptEdits.',
    },
};

const SECTION_TEXT_ZH: SectionTextMap = {
    behavior: DEFAULT_MODE_SECTION_TEXT.zh,
    model: {
        title: '模型路由',
        hint: '每个槽位对应 Claude Code 内部一个用途。只用一个模型时把 5 个槽位填成同一个值即可。',
    },
    limits: {
        title: '性能与限制',
        hint: '留空 = 不写这个 env，Claude Code 用自己的默认值。',
    },
    switches: {
        title: '隐私与行为',
        hint: '开启 = 设置为 "1"；关闭 = 不写入。',
    },
    network: {
        title: '网络代理',
        hint: '留空 = 不写入。tb 始终自动将 localhost/127.0.0.1/::1 追加到 NO_PROXY，无需手动设置。',
    },
};

const SECTION_TEXT_EN: SectionTextMap = {
    behavior: DEFAULT_MODE_SECTION_TEXT.en,
    model: {
        title: 'Model routing',
        hint: 'Each slot maps to one of Claude Code\'s internal uses. To use a single model, fill all 5 slots with the same value.',
    },
    limits: {
        title: 'Performance & limits',
        hint: 'Blank = the env is not written; Claude Code uses its own default.',
    },
    switches: {
        title: 'Privacy & behavior',
        hint: 'On = set to "1"; Off = not written.',
    },
    network: {
        title: 'Network proxy',
        hint: 'Blank = not written. tb always appends localhost/127.0.0.1/::1 to NO_PROXY automatically — no manual entry needed.',
    },
};

interface UIText {
    panelHeader: string;
    oneMTooltip: string;
}

const UI_TEXT_ZH: UIText = {
    panelHeader: '每行对应一个 Claude Code 环境变量。hover 信息图标查看含义；留空 / 关闭 = 不写入。',
    oneMTooltip: '启用 1M 上下文窗口（在模型 ID 末尾追加 [1m]，需路由的目标模型支持）',
};

const UI_TEXT_EN: UIText = {
    panelHeader: 'Each row is one Claude Code env var. Hover the info icon for details. Blank / off = the env is not written.',
    oneMTooltip: 'Enable the 1M context window (appends [1m] to the model ID; the routed target model must support it).',
};

const FIELDS_TEXT: Record<Lang, FieldTextMap> = { zh: FIELDS_TEXT_ZH, en: FIELDS_TEXT_EN };
const SECTION_TEXT: Record<Lang, SectionTextMap> = { zh: SECTION_TEXT_ZH, en: SECTION_TEXT_EN };
const UI_TEXT: Record<Lang, UIText> = { zh: UI_TEXT_ZH, en: UI_TEXT_EN };

const useLang = (): Lang => {
    const { i18n } = useTranslation();
    return i18n.language === 'zh' ? 'zh' : 'en';
};

// ── Default prefs derivation ───────────────────────────────────────────
// Build initial prefs from the active routing rules, mirroring how the
// legacy modal picks models per slot. Anything not derivable falls back
// to tb's canonical defaults.

interface DerivePrefsInput {
    rules: any[];
    mode: 'unified' | 'separate' | 'smart';
}

export const derivePrefsFromRules = ({ rules, mode }: DerivePrefsInput): ClaudeCodePrefs => {
    const modelForVariant = (variant: string, fallback: string): string => {
        if (mode === 'unified') return rules[0]?.request_model || fallback;
        const rule = rules.find((r: any) => r?.uuid === `builtin:claude_code:${variant}`);
        return rule?.request_model || fallback;
    };

    // Get the 1M context window flag from a specific rule. Rules here come
    // straight from the API (snake_case flags); accept the camelCase shape
    // too in case a converted rule object is passed in.
    const getContext1MStateForRule = (rule: any): boolean => {
        if (!rule || !rule.flags) return false;
        return rule.flags?.context_1m || rule.flags?.context1m || false;
    };

    // Get the 1M state for a specific variant (only used in separate mode)
    const getContext1MStateForVariant = (variant: string): boolean => {
        if (mode === 'unified') {
            // In unified mode, use the first rule's context1m state
            return getContext1MStateForRule(rules[0]);
        }
        // In separate mode, check the specific rule for this variant
        const rule = rules.find((r: any) => r?.uuid === `builtin:claude_code:${variant}`);
        return getContext1MStateForRule(rule);
    };

    const context1MEnabled = mode === 'unified'
        ? getContext1MStateForRule(rules[0])
        : getContext1MStateForRule(rules.find((r: any) => r?.uuid === 'builtin:claude_code:default'));


    const isUnified = mode !== 'separate';
    const defaultModel = isUnified ? 'tingly/cc' : 'tingly/cc-default';

    // Apply 1M suffix to models if their corresponding rule has context1m enabled
    const apply1MSuffix = (model: string, variant: string): string => {
        const variantContext1M = getContext1MStateForVariant(variant);
        return with1M(model, variantContext1M);
    };

    return {
        ANTHROPIC_MODEL: apply1MSuffix(modelForVariant('default', defaultModel), 'default'),
        ANTHROPIC_DEFAULT_HAIKU_MODEL: apply1MSuffix(modelForVariant('haiku', isUnified ? defaultModel : 'tingly/cc-haiku'), 'haiku'),
        ANTHROPIC_DEFAULT_SONNET_MODEL: apply1MSuffix(modelForVariant('sonnet', isUnified ? defaultModel : 'tingly/cc-sonnet'), 'sonnet'),
        ANTHROPIC_DEFAULT_OPUS_MODEL: apply1MSuffix(modelForVariant('opus', isUnified ? defaultModel : 'tingly/cc-opus'), 'opus'),
        CLAUDE_CODE_SUBAGENT_MODEL: apply1MSuffix(modelForVariant('subagent', isUnified ? defaultModel : 'tingly/cc-subagent'), 'subagent'),

        API_TIMEOUT_MS: '3000000',
        CLAUDE_CODE_MAX_OUTPUT_TOKENS: '32000',
        CLAUDE_CODE_AUTO_COMPACT_WINDOW: context1MEnabled ? '1000000' : '200000',
        CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: '80',

        DISABLE_TELEMETRY: '1',
        DISABLE_ERROR_REPORTING: '1',
        CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1',
    };
};

// ── Materialize prefs to the env map the backend will write ────────────
// Mirrors internal/agent/prefs.go ToEnv(): strip empties, inject the
// server-resolved base URL + auth token.
// appendNoProxy mirrors the Go appendNoProxy helper in internal/agent/prefs.go.
const appendNoProxy = (current: string, ...hosts: string[]): string => {
    const existing = new Set(current ? current.split(',').map(h => h.trim()) : []);
    let result = current;
    for (const h of hosts) {
        if (!existing.has(h)) {
            result = result ? result + ',' + h : h;
            existing.add(h);
        }
    }
    return result;
};

export const prefsToEnvPreview = (
    prefs: ClaudeCodePrefs,
    baseURL: string,
    token: string,
): Record<string, string> => {
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(prefs)) {
        if (v === undefined || v === '') continue;
        out[k] = v;
    }
    out.ANTHROPIC_BASE_URL = baseURL.replace(/\/$/, '') + '/tingly/claude_code';
    out.ANTHROPIC_AUTH_TOKEN = token;
    // Mirror Go's ToEnv(): always ensure localhost entries are in NO_PROXY
    out.NO_PROXY = appendNoProxy(out.NO_PROXY ?? '', 'localhost', '127.0.0.1', '::1');
    return out;
};

// ── Field row (3-column, single line) ──────────────────────────────────
// Layout:  [ Label + (i) ]   [ ENV_NAME code badge ]   [ control · right ]
// Switches and inputs are right-aligned in column 3 — Android-style "row
// with trailing control" so the page reads as a compact list, not a form.

interface FieldRowProps {
    field: FieldStruct;
    text: FieldText;
    oneMTooltip: string;
    prefs: ClaudeCodePrefs;
    setPrefs: (next: ClaudeCodePrefs) => void;
}

const FieldRow: React.FC<FieldRowProps> = ({ field, text, oneMTooltip, prefs, setPrefs }) => {
    const value = prefs[field.envName] ?? '';
    const setValue = (next: string) => setPrefs({ ...prefs, [field.envName]: next });

    const richTooltip = (
        <Box sx={{ maxWidth: 280 }}>
            <Typography variant="caption" sx={{ display: 'block', mb: 0.5 }}>{text.purpose}</Typography>
            <Typography variant="caption" sx={{ display: 'block', opacity: 0.85 }}>{text.tooltip}</Typography>
        </Box>
    );

    return (
        <Box
            sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 2,
                py: 1,
                minHeight: 44,
            }}
        >
            {/* Col 1 — Label + info icon */}
            <Box sx={{ flex: '0 0 180px', display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                <Typography variant="body2" fontWeight={500} noWrap>{text.label}</Typography>
                <Tooltip placement="top" arrow title={richTooltip}>
                    <InfoOutlinedIcon sx={{ fontSize: 14, color: 'text.disabled', cursor: 'help' }} />
                </Tooltip>
            </Box>

            {/* Col 2 — env name as a subtle code badge */}
            <Box sx={{ flex: '0 0 320px', minWidth: 0 }}>
                <Box
                    component="span"
                    sx={{
                        px: 0.75,
                        py: 0.25,
                        borderRadius: 0.75,
                        bgcolor: 'action.hover',
                        fontFamily: 'monospace',
                        fontSize: '0.72rem',
                        color: 'text.secondary',
                        whiteSpace: 'nowrap',
                    }}
                >
                    {field.envName}
                </Box>
            </Box>

            {/* Col 3 — control, right-aligned */}
            <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 1.5 }}>
                {field.kind === 'bool' && (
                    <Switch
                        size="small"
                        checked={value === '1'}
                        onChange={(_, c) => setValue(c ? '1' : '')}
                    />
                )}
                {(field.kind === 'int' || field.kind === 'text' || field.kind === 'model') && (
                    <TextField
                        size="small"
                        value={field.kind === 'model' ? value.replace(/\[1m\]$/, '') : value}
                        onChange={(e) => {
                            const next = e.target.value;
                            setValue(field.kind === 'model' ? with1M(next, has1M(value)) : next);
                        }}
                        placeholder={text.placeholder}
                        sx={{ width: field.kind === 'model' ? 280 : field.kind === 'text' ? 320 : 180 }}
                        InputProps={{
                            endAdornment: field.unit
                                ? <InputAdornment position="end"><Typography variant="caption" color="text.disabled">{field.unit}</Typography></InputAdornment>
                                : undefined,
                            sx: { fontFamily: field.kind === 'model' ? 'monospace' : undefined, fontSize: '0.85rem' },
                        }}
                    />
                )}
                {field.kind === 'model' && (
                    <Tooltip title={oneMTooltip} arrow placement="top">
                        <Box sx={{ display: 'flex', alignItems: 'center', flexShrink: 0 }}>
                            <Typography variant="caption" sx={{ mr: 0.25, color: 'text.secondary', letterSpacing: 0.5 }}>1M</Typography>
                            <Switch
                                size="small"
                                checked={has1M(value)}
                                disabled={true}
                                sx={{
                                    '& .Mui-checked': {
                                        color: has1M(value) ? 'primary.main' : 'text.disabled',
                                    },
                                    '& .Mui-checked + .MuiSwitch-track': {
                                        backgroundColor: has1M(value) ? 'primary.main' : 'text.disabled',
                                    },
                                }}
                            />
                        </Box>
                    </Tooltip>
                )}
            </Box>
        </Box>
    );
};

// ── Section ────────────────────────────────────────────────────────────

interface SectionProps {
    group: Group;
    lang: Lang;
    prefs: ClaudeCodePrefs;
    setPrefs: (p: ClaudeCodePrefs) => void;
}

const Section: React.FC<SectionProps> = ({ group, lang, prefs, setPrefs }) => {
    const [expanded, setExpanded] = React.useState(group === 'model'); // Only model group expanded by default
    const meta = SECTION_TEXT[lang][group];
    const fieldsText = FIELDS_TEXT[lang];
    const oneMTooltip = UI_TEXT[lang].oneMTooltip;
    const fields = FIELD_STRUCT.filter(f => f.group === group);
    const hasAdvancedFields = fields.some(f => f.advanced);

    const toggleExpanded = () => setExpanded(!expanded);

    return (
        <Box>
            <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5, mb: 0.5 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, flex: 1 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>{meta.title}</Typography>
                    <Typography variant="caption" color="text.secondary">{meta.hint}</Typography>
                </Box>
                {hasAdvancedFields && (
                    <IconButton
                        size="small"
                        onClick={toggleExpanded}
                        sx={{
                            transition: 'transform 0.2s',
                            transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)',
                            p: 0.5,
                        }}
                    >
                        <ExpandMoreIcon fontSize="small" />
                    </IconButton>
                )}
            </Box>
            <Collapse in={expanded} timeout={300}>
                <Divider />
                <Stack divider={<Divider flexItem />}>
                    {fields.map(f => (
                        <FieldRow
                            key={f.envName}
                            field={f}
                            text={fieldsText[f.envName]}
                            oneMTooltip={oneMTooltip}
                            prefs={prefs}
                            setPrefs={setPrefs}
                        />
                    ))}
                </Stack>
            </Collapse>
        </Box>
    );
};

// ── Main panel ─────────────────────────────────────────────────────────

interface QuickConfigPanelProps {
    prefs: ClaudeCodePrefs;
    setPrefs: (p: ClaudeCodePrefs) => void;
    defaultMode: ClaudeCodeDefaultMode;
    setDefaultMode: (mode: ClaudeCodeDefaultMode) => void;
}

const DefaultModeSection: React.FC<{
    lang: Lang;
    defaultMode: ClaudeCodeDefaultMode;
    setDefaultMode: (mode: ClaudeCodeDefaultMode) => void;
}> = ({ lang, defaultMode, setDefaultMode }) => {
    const meta = DEFAULT_MODE_SECTION_TEXT[lang];
    const text = DEFAULT_MODE_TEXT[lang];
    const selectedText = text[defaultMode];

    return (
        <Box>
            <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5, mb: 0.5 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>{meta.title}</Typography>
                <Typography variant="caption" color="text.secondary">{meta.hint}</Typography>
            </Box>
            <Divider />
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, py: 1, minHeight: 52 }}>
                <Box sx={{ flex: '0 0 180px', display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                    <Typography variant="body2" fontWeight={500} noWrap>Default Mode</Typography>
                    <Tooltip placement="top" arrow title={`${selectedText.label}: ${selectedText.description}`}>
                        <InfoOutlinedIcon sx={{ fontSize: 14, color: 'text.disabled', cursor: 'help' }} />
                    </Tooltip>
                </Box>
                <Box sx={{ flex: '0 0 320px', minWidth: 0 }}>
                    <Box
                        component="span"
                        sx={{
                            px: 0.75,
                            py: 0.25,
                            borderRadius: 0.75,
                            bgcolor: 'action.hover',
                            fontFamily: 'monospace',
                            fontSize: '0.72rem',
                            color: 'text.secondary',
                            whiteSpace: 'nowrap',
                        }}
                    >
                        defaultMode
                    </Box>
                </Box>
                <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'flex-end' }}>
                    <FormControl size="small" sx={{ width: 360 }}>
                        <Select
                            value={defaultMode}
                            onChange={(e) => setDefaultMode(e.target.value as ClaudeCodeDefaultMode)}
                            renderValue={(value) => {
                                const mode = value as ClaudeCodeDefaultMode;
                                return (
                                    <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 2, width: '100%' }}>
                                        <Typography component="span" variant="body2">{text[mode].label}</Typography>
                                        <Typography component="span" variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>{mode}</Typography>
                                    </Box>
                                );
                            }}
                            MenuProps={{
                                PaperProps: { sx: { maxHeight: 320, width: 360 } },
                                MenuListProps: { sx: { py: 0.5 } },
                            }}
                            sx={{
                                height: 40,
                                '& .MuiSelect-select': {
                                    display: 'flex',
                                    alignItems: 'center',
                                    py: 1,
                                },
                            }}
                        >
                            {DEFAULT_MODE_OPTIONS.map((mode) => (
                                <MenuItem key={mode} value={mode} sx={{ minHeight: 40, py: 1 }}>
                                    <Tooltip title={text[mode].description} arrow placement="left">
                                        <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 2, width: '100%' }}>
                                            <Typography variant="body2">{text[mode].label}</Typography>
                                            <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>{mode}</Typography>
                                        </Box>
                                    </Tooltip>
                                </MenuItem>
                            ))}
                        </Select>
                    </FormControl>
                </Box>
            </Box>
        </Box>
    );
};

const ClaudeCodeQuickConfig: React.FC<QuickConfigPanelProps> = ({
    prefs,
    setPrefs,
    defaultMode,
    setDefaultMode,
}) => {
    const lang = useLang();
    const uiText = UI_TEXT[lang];

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2.5 }}>
            <Typography variant="body2" color="text.secondary">{uiText.panelHeader}</Typography>

            <DefaultModeSection lang={lang} defaultMode={defaultMode} setDefaultMode={setDefaultMode} />
            <Section group="model" lang={lang} prefs={prefs} setPrefs={setPrefs} />
            <Section group="limits" lang={lang} prefs={prefs} setPrefs={setPrefs} />
            <Section group="switches" lang={lang} prefs={prefs} setPrefs={setPrefs} />
            <Section group="network" lang={lang} prefs={prefs} setPrefs={setPrefs} />
        </Box>
    );
};

export default ClaudeCodeQuickConfig;
