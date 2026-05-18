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
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import RestartAltIcon from '@mui/icons-material/RestartAlt';
import React from 'react';

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

interface FieldDescriptor {
    envName: PrefsKey;
    group: Group;
    kind: Kind;
    label: string;
    purpose: string;
    tooltip: string;
    placeholder?: string;
    unit?: string;
    // supportsOneM=true renders a [1m] toggle next to the model input. Only
    // sonnet/opus families currently support the 1M context window per
    // Anthropic docs; haiku does not.
    supportsOneM?: boolean;
}

// Single source of truth for the form. Adding an env = adding a row here.
export const FIELDS: FieldDescriptor[] = [
    // ── Models ───────────────────────────────────────────────────────────
    {
        envName: 'ANTHROPIC_MODEL',
        group: 'model', kind: 'model',
        label: '默认模型',
        purpose: '未指定具体场景时使用的兜底模型',
        tooltip: 'Claude Code 在没有专门路由时回退到这个模型。tb 通常映射到 tingly/cc 或 tingly/cc-default。',
        placeholder: 'tingly/cc',
        supportsOneM: true,
    },
    {
        envName: 'ANTHROPIC_DEFAULT_HAIKU_MODEL',
        group: 'model', kind: 'model',
        label: 'Haiku 槽位',
        purpose: '轻量调用（如生成 commit message、文件摘要）使用的模型',
        tooltip: 'Claude Code 内部对一些便宜的辅助调用走 haiku 槽位。tb 把它路由到 tingly/cc-haiku。',
        placeholder: 'tingly/cc-haiku',
    },
    {
        envName: 'ANTHROPIC_DEFAULT_SONNET_MODEL',
        group: 'model', kind: 'model',
        label: 'Sonnet 槽位',
        purpose: '主力槽位 — 大部分对话和代码生成走这里',
        tooltip: 'Claude Code 的默认主力。除非显式选其他模型，正常会话都用 sonnet 槽位。',
        placeholder: 'tingly/cc-sonnet',
        supportsOneM: true,
    },
    {
        envName: 'ANTHROPIC_DEFAULT_OPUS_MODEL',
        group: 'model', kind: 'model',
        label: 'Opus 槽位',
        purpose: '复杂推理（如 plan 模式、深度分析）使用的模型',
        tooltip: '相对昂贵但更强的推理模型。Claude Code 在显式调用 opus 时使用。',
        placeholder: 'tingly/cc-opus',
        supportsOneM: true,
    },
    {
        envName: 'CLAUDE_CODE_SUBAGENT_MODEL',
        group: 'model', kind: 'model',
        label: '子 Agent 模型',
        purpose: '通过 Task 工具派生的子 Agent 使用的模型',
        tooltip: '子 Agent 用于并发研究、独立子任务。可以单独指定一个更便宜或更强的模型。',
        placeholder: 'tingly/cc-subagent',
    },

    // ── Limits ───────────────────────────────────────────────────────────
    {
        envName: 'API_TIMEOUT_MS',
        group: 'limits', kind: 'int', unit: 'ms',
        label: 'API 请求超时',
        purpose: '单次 API 请求最长等待时间',
        tooltip: '官方默认 120000 (2 分钟)。tb 走代理常有长任务，推荐拉到 3000000 (50 分钟)。',
        placeholder: '3000000',
    },
    {
        envName: 'CLAUDE_CODE_MAX_OUTPUT_TOKENS',
        group: 'limits', kind: 'int', unit: 'tokens',
        label: '最大输出 token',
        purpose: '单条回复输出的 token 上限',
        tooltip: '太小可能被截断，太大会浪费配额。tb 推荐 32000。',
        placeholder: '32000',
    },
    {
        envName: 'MAX_THINKING_TOKENS',
        group: 'limits', kind: 'int', unit: 'tokens',
        label: '思考 token 预算',
        purpose: 'Extended Thinking 模式下的思考 token 上限',
        tooltip: '留空表示用模型默认值。仅对支持 thinking 的模型有效。',
        placeholder: '(空 = 模型默认)',
    },
    {
        envName: 'BASH_DEFAULT_TIMEOUT_MS',
        group: 'limits', kind: 'int', unit: 'ms',
        label: 'Bash 默认超时',
        purpose: 'Bash 工具单次执行的默认超时',
        tooltip: '官方默认 120000。长跑脚本（如 npm install）若超时可以调高。',
        placeholder: '120000',
    },
    {
        envName: 'BASH_MAX_TIMEOUT_MS',
        group: 'limits', kind: 'int', unit: 'ms',
        label: 'Bash 最大超时',
        purpose: 'Bash 工具允许指定的最长超时',
        tooltip: 'Claude 自己设置 timeout 时的上限。',
        placeholder: '600000',
    },
    {
        envName: 'MCP_TIMEOUT',
        group: 'limits', kind: 'int', unit: 'ms',
        label: 'MCP 连接超时',
        purpose: 'MCP server 启动/响应的超时',
        tooltip: '官方默认 30000。MCP server 启动慢可以调高。',
        placeholder: '30000',
    },
    {
        envName: 'MCP_TOOL_TIMEOUT',
        group: 'limits', kind: 'int', unit: 'ms',
        label: 'MCP 工具超时',
        purpose: '单次 MCP 工具调用的超时',
        tooltip: '官方默认 10000。',
        placeholder: '10000',
    },
    {
        envName: 'MAX_MCP_OUTPUT_TOKENS',
        group: 'limits', kind: 'int', unit: 'tokens',
        label: 'MCP 输出上限',
        purpose: 'MCP 工具单次返回内容的 token 上限',
        tooltip: '官方默认 8192。超过会被截断。',
        placeholder: '8192',
    },

    // ── Switches ─────────────────────────────────────────────────────────
    {
        envName: 'DISABLE_TELEMETRY',
        group: 'switches', kind: 'bool',
        label: '禁用遥测',
        purpose: '关闭 Anthropic 官方遥测上报',
        tooltip: 'tb 默认开启此项以保护内网/隐私环境。',
    },
    {
        envName: 'DISABLE_ERROR_REPORTING',
        group: 'switches', kind: 'bool',
        label: '禁用错误上报',
        purpose: '关闭异常自动上报到 Anthropic',
        tooltip: 'tb 默认开启此项。',
    },
    {
        envName: 'CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC',
        group: 'switches', kind: 'bool',
        label: '禁用非必要流量',
        purpose: '关闭所有非业务请求（更新检查、提示、统计等）',
        tooltip: '最干净的模式，只保留模型调用本身。tb 默认开启。',
    },
    {
        envName: 'DISABLE_AUTOUPDATER',
        group: 'switches', kind: 'bool',
        label: '禁用自动更新',
        purpose: 'Claude Code 不再自动检查/下载新版本',
        tooltip: '通常用于固定版本的部署环境。',
    },
    {
        envName: 'USE_BUILTIN_RIPGREP',
        group: 'switches', kind: 'bool',
        label: '使用内置 ripgrep',
        purpose: 'Claude Code 自带的 ripgrep 优先于系统 PATH',
        tooltip: '官方默认开启。仅在需要用系统自定义 ripgrep 时关闭。',
    },
];

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
    field: FieldDescriptor;
    prefs: ClaudeCodePrefs;
    setPrefs: (next: ClaudeCodePrefs) => void;
}

const FieldRow: React.FC<FieldRowProps> = ({ field, prefs, setPrefs }) => {
    const value = prefs[field.envName] ?? '';
    const setValue = (next: string) => setPrefs({ ...prefs, [field.envName]: next });

    const richTooltip = (
        <Box sx={{ maxWidth: 280 }}>
            <Typography variant="caption" sx={{ display: 'block', mb: 0.5 }}>{field.purpose}</Typography>
            <Typography variant="caption" sx={{ display: 'block', opacity: 0.85 }}>{field.tooltip}</Typography>
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
                <Typography variant="body2" fontWeight={500} noWrap>{field.label}</Typography>
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
                        placeholder={field.placeholder}
                        sx={{ width: field.kind === 'model' ? 280 : 180 }}
                        InputProps={{
                            endAdornment: field.unit
                                ? <InputAdornment position="end"><Typography variant="caption" color="text.disabled">{field.unit}</Typography></InputAdornment>
                                : undefined,
                            sx: { fontFamily: field.kind === 'model' ? 'monospace' : undefined, fontSize: '0.85rem' },
                        }}
                    />
                )}
                {field.kind === 'model' && field.supportsOneM && (
                    <Tooltip
                        title="启用 1M 上下文窗口（追加 [1m] 后缀，仅 Sonnet 4.5+ / Opus 4+ 支持）"
                        arrow
                        placement="top"
                    >
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

const SECTION_META: Record<Group, { title: string; hint: string }> = {
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

const Section: React.FC<{ group: Group; prefs: ClaudeCodePrefs; setPrefs: (p: ClaudeCodePrefs) => void }> = ({ group, prefs, setPrefs }) => {
    const meta = SECTION_META[group];
    const fields = FIELDS.filter(f => f.group === group);
    return (
        <Box>
            <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5, mb: 0.5 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>{meta.title}</Typography>
                <Typography variant="caption" color="text.secondary">{meta.hint}</Typography>
            </Box>
            <Divider />
            <Stack divider={<Divider flexItem />}>
                {fields.map(f => <FieldRow key={f.envName} field={f} prefs={prefs} setPrefs={setPrefs} />)}
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
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2.5 }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="body2" color="text.secondary">
                    每行对应一个 Claude Code 环境变量。hover 信息图标查看含义；留空 / 关闭 = 不写入。
                </Typography>
                <Tooltip title="重置为 tb 推荐默认值" arrow>
                    <IconButton size="small" onClick={onResetDefaults}>
                        <RestartAltIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
            </Box>

            <Section group="model" prefs={prefs} setPrefs={setPrefs} />
            <Section group="limits" prefs={prefs} setPrefs={setPrefs} />
            <Section group="switches" prefs={prefs} setPrefs={setPrefs} />
        </Box>
    );
};

export default ClaudeCodeQuickConfig;
