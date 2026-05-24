import {
    Box,
    Divider,
    IconButton,
    InputAdornment,
    Stack,
    Switch,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import { InfoOutlined as InfoOutlinedIcon } from '@/components/icons';
import { RestartAlt as RestartAltIcon } from '@/components/icons';
import React from 'react';
import { useTranslation } from 'react-i18next';

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

    DISABLE_TELEMETRY?: string;
    DISABLE_ERROR_REPORTING?: string;
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC?: string;
    DISABLE_AUTOUPDATER?: string;
    USE_BUILTIN_RIPGREP?: string;
}

type PrefsKey = keyof ClaudeCodePrefs;
type Group = 'model' | 'limits' | 'switches';
type Kind = 'model' | 'int' | 'bool';
type Lang = 'zh' | 'en';

// ── Field structure (language-agnostic) ────────────────────────────────
// Adding a new env: append a row here AND add entries in FIELDS_TEXT_ZH /
// FIELDS_TEXT_EN below (TS will flag the missing keys).

interface FieldStruct {
    envName: PrefsKey;
    group: Group;
    kind: Kind;
    unit?: string;
}

const FIELD_STRUCT: FieldStruct[] = [
    // Models
    { envName: 'ANTHROPIC_MODEL', group: 'model', kind: 'model' },
    { envName: 'ANTHROPIC_DEFAULT_HAIKU_MODEL', group: 'model', kind: 'model' },
    { envName: 'ANTHROPIC_DEFAULT_SONNET_MODEL', group: 'model', kind: 'model' },
    { envName: 'ANTHROPIC_DEFAULT_OPUS_MODEL', group: 'model', kind: 'model' },
    { envName: 'CLAUDE_CODE_SUBAGENT_MODEL', group: 'model', kind: 'model' },
    // Limits
    { envName: 'API_TIMEOUT_MS', group: 'limits', kind: 'int', unit: 'ms' },
    { envName: 'CLAUDE_CODE_MAX_OUTPUT_TOKENS', group: 'limits', kind: 'int', unit: 'tokens' },
    { envName: 'MAX_THINKING_TOKENS', group: 'limits', kind: 'int', unit: 'tokens' },
    { envName: 'BASH_DEFAULT_TIMEOUT_MS', group: 'limits', kind: 'int', unit: 'ms' },
    { envName: 'BASH_MAX_TIMEOUT_MS', group: 'limits', kind: 'int', unit: 'ms' },
    { envName: 'MCP_TIMEOUT', group: 'limits', kind: 'int', unit: 'ms' },
    { envName: 'MCP_TOOL_TIMEOUT', group: 'limits', kind: 'int', unit: 'ms' },
    { envName: 'MAX_MCP_OUTPUT_TOKENS', group: 'limits', kind: 'int', unit: 'tokens' },
    // Switches
    { envName: 'DISABLE_TELEMETRY', group: 'switches', kind: 'bool' },
    { envName: 'DISABLE_ERROR_REPORTING', group: 'switches', kind: 'bool' },
    { envName: 'CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC', group: 'switches', kind: 'bool' },
    { envName: 'DISABLE_AUTOUPDATER', group: 'switches', kind: 'bool' },
    { envName: 'USE_BUILTIN_RIPGREP', group: 'switches', kind: 'bool' },
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
};

interface SectionText { title: string; hint: string }
type SectionTextMap = Record<Group, SectionText>;

const SECTION_TEXT_ZH: SectionTextMap = {
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
};

const SECTION_TEXT_EN: SectionTextMap = {
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
};

interface UIText {
    panelHeader: string;
    resetTooltip: string;
    oneMTooltip: string;
}

const UI_TEXT_ZH: UIText = {
    panelHeader: '每行对应一个 Claude Code 环境变量。hover 信息图标查看含义；留空 / 关闭 = 不写入。',
    resetTooltip: '重置为 tb 推荐默认值',
    oneMTooltip: '启用 1M 上下文窗口（在模型 ID 末尾追加 [1m]，需路由的目标模型支持）',
};

const UI_TEXT_EN: UIText = {
    panelHeader: 'Each row is one Claude Code env var. Hover the info icon for details. Blank / off = the env is not written.',
    resetTooltip: 'Reset to tb-recommended defaults',
    oneMTooltip: 'Enable the 1M context window (appends [1m] to the model ID; the routed target model must support it).',
};

const FIELDS_TEXT: Record<Lang, FieldTextMap> = { zh: FIELDS_TEXT_ZH, en: FIELDS_TEXT_EN };
const SECTION_TEXT: Record<Lang, SectionTextMap> = { zh: SECTION_TEXT_ZH, en: SECTION_TEXT_EN };
const UI_TEXT: Record<Lang, UIText> = { zh: UI_TEXT_ZH, en: UI_TEXT_EN };

const useLang = (): Lang => {
    const { i18n } = useTranslation();
    return i18n.language === 'zh' ? 'zh' : 'en';
};

// ── 1M-context suffix helpers ──────────────────────────────────────────
// 1M is part of the model string itself; the suffix lives on the wire,
// not as a separate prefs field. The UI just toggles the trailing [1m].

const ONE_M_SUFFIX = '[1m]';
const has1M = (v: string | undefined) => !!v && v.endsWith(ONE_M_SUFFIX);
const with1M = (v: string | undefined, on: boolean): string => {
    const base = (v || '').replace(/\[1m\]$/, '');
    return on ? base + ONE_M_SUFFIX : base;
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
        const rule = rules.find((r: any) => r?.uuid === `built-in-cc-${variant}`);
        return rule?.request_model || fallback;
    };

    const isUnified = mode !== 'separate';
    const defaultModel = isUnified ? 'tingly/cc' : 'tingly/cc-default';

    return {
        ANTHROPIC_MODEL: modelForVariant('default', defaultModel),
        ANTHROPIC_DEFAULT_HAIKU_MODEL: modelForVariant('haiku', isUnified ? defaultModel : 'tingly/cc-haiku'),
        ANTHROPIC_DEFAULT_SONNET_MODEL: modelForVariant('sonnet', isUnified ? defaultModel : 'tingly/cc-sonnet'),
        ANTHROPIC_DEFAULT_OPUS_MODEL: modelForVariant('opus', isUnified ? defaultModel : 'tingly/cc-opus'),
        CLAUDE_CODE_SUBAGENT_MODEL: modelForVariant('subagent', isUnified ? defaultModel : 'tingly/cc-subagent'),

        API_TIMEOUT_MS: '3000000',
        CLAUDE_CODE_MAX_OUTPUT_TOKENS: '32000',

        DISABLE_TELEMETRY: '1',
        DISABLE_ERROR_REPORTING: '1',
        CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1',
    };
};

// ── Materialize prefs to the env map the backend will write ────────────
// Mirrors internal/agent/prefs.go ToEnv(): strip empties, inject the
// server-resolved base URL + auth token.
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
                {(field.kind === 'int' || field.kind === 'model') && (
                    <TextField
                        size="small"
                        value={field.kind === 'model' ? value.replace(/\[1m\]$/, '') : value}
                        onChange={(e) => {
                            const next = e.target.value;
                            setValue(field.kind === 'model' && has1M(value) ? next + ONE_M_SUFFIX : next);
                        }}
                        placeholder={text.placeholder}
                        sx={{ width: field.kind === 'model' ? 280 : 180 }}
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
                                onChange={(_, c) => setValue(with1M(value, c))}
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
    const meta = SECTION_TEXT[lang][group];
    const fieldsText = FIELDS_TEXT[lang];
    const oneMTooltip = UI_TEXT[lang].oneMTooltip;
    const fields = FIELD_STRUCT.filter(f => f.group === group);
    return (
        <Box>
            <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5, mb: 0.5 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>{meta.title}</Typography>
                <Typography variant="caption" color="text.secondary">{meta.hint}</Typography>
            </Box>
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
        </Box>
    );
};

// ── Main panel ─────────────────────────────────────────────────────────

interface QuickConfigPanelProps {
    prefs: ClaudeCodePrefs;
    setPrefs: (p: ClaudeCodePrefs) => void;
    onResetDefaults: () => void;
}

const ClaudeCodeQuickConfig: React.FC<QuickConfigPanelProps> = ({
    prefs,
    setPrefs,
    onResetDefaults,
}) => {
    const lang = useLang();
    const uiText = UI_TEXT[lang];

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2.5 }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="body2" color="text.secondary">{uiText.panelHeader}</Typography>
                <Tooltip title={uiText.resetTooltip} arrow>
                    <IconButton size="small" onClick={onResetDefaults}>
                        <RestartAltIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
            </Box>

            <Section group="model" lang={lang} prefs={prefs} setPrefs={setPrefs} />
            <Section group="limits" lang={lang} prefs={prefs} setPrefs={setPrefs} />
            <Section group="switches" lang={lang} prefs={prefs} setPrefs={setPrefs} />
        </Box>
    );
};

export default ClaudeCodeQuickConfig;
