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
import { type AppLanguage, resolveLanguage } from '@/i18n';
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

export type PrefsKey = keyof ClaudeCodePrefs;
export type ClaudeCodeDefaultMode = 'acceptEdits' | 'bypassPermissions' | 'default' | 'delegate' | 'dontAsk' | 'manual' | 'plan' | 'auto';
export type Group = 'behavior' | 'model' | 'limits' | 'switches' | 'network';
export type Kind = 'model' | 'int' | 'text' | 'bool';

// ── Field structure (language-agnostic) ────────────────────────────────
// Adding a new env: append a row here AND add an entry in every FIELDS_TEXT_*
// bundle below (TS will flag the missing keys).

export interface FieldStruct {
    envName: PrefsKey;
    group: Group;
    kind: Kind;
    unit?: string;
    advanced?: boolean; // Mark advanced fields that should be collapsed by default
}

// Shared layout primitives for the main Auto Config form and profile
// overrides. Keeping the same label / key / control axes makes both surfaces
// read as one configuration system instead of two unrelated forms.
export const CLAUDE_CONFIG_ROW_COLUMNS = {
    xs: 'minmax(0, 1fr)',
    md: '180px minmax(220px, 320px) minmax(260px, 1fr)',
} as const;

export const CLAUDE_CONFIG_KEY_SX = {
    // Env var names are machine text — never mirrored (see index.css).
    direction: 'ltr',
    display: 'inline-flex',
    maxWidth: '100%',
    px: 0.75,
    py: 0.25,
    borderRadius: 0.75,
    bgcolor: 'action.hover',
    fontFamily: 'monospace',
    fontSize: '0.72rem',
    lineHeight: 1.5,
    color: 'text.secondary',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
} as const;

export const CLAUDE_CODE_FIELD_STRUCT: FieldStruct[] = [
    // Models (always visible - most commonly adjusted)
    { envName: 'ANTHROPIC_MODEL', group: 'model', kind: 'model', advanced: false },
    { envName: 'ANTHROPIC_DEFAULT_HAIKU_MODEL', group: 'model', kind: 'model', advanced: false },
    { envName: 'ANTHROPIC_DEFAULT_SONNET_MODEL', group: 'model', kind: 'model', advanced: false },
    { envName: 'ANTHROPIC_DEFAULT_OPUS_MODEL', group: 'model', kind: 'model', advanced: false },
    { envName: 'CLAUDE_CODE_SUBAGENT_MODEL', group: 'model', kind: 'model', advanced: false },
    { envName: 'CLAUDE_CODE_MAX_OUTPUT_TOKENS', group: 'model', kind: 'int', unit: 'tokens', advanced: false },
    // Limits (advanced - rarely changed)
    { envName: 'API_TIMEOUT_MS', group: 'limits', kind: 'int', unit: 'ms', advanced: true },
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
// facing, and likely to churn as we tune the wording. Parallel per-language
// maps avoid the i18n locale file becoming a junk drawer.

export interface FieldText {
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

const FIELDS_TEXT_RU: FieldTextMap = {
    ANTHROPIC_MODEL: {
        label: 'Модель по умолчанию',
        purpose: 'Запасная модель, когда ни один специальный слот не подходит',
        tooltip: 'К ней Claude Code обращается, если специализированная маршрутизация не сработала. В tb это обычно tingly/cc или tingly/cc-default.',
        placeholder: 'tingly/cc',
    },
    ANTHROPIC_DEFAULT_HAIKU_MODEL: {
        label: 'Слот Haiku',
        purpose: 'Лёгкие задачи: сообщения коммитов, краткие сводки',
        tooltip: 'Claude Code направляет дешёвые вспомогательные вызовы в слот haiku. tb указывает на tingly/cc-haiku.',
        placeholder: 'tingly/cc-haiku',
    },
    ANTHROPIC_DEFAULT_SONNET_MODEL: {
        label: 'Слот Sonnet',
        purpose: 'Основной слот — сюда идёт большая часть диалога и генерации кода',
        tooltip: 'Значение Claude Code по умолчанию. Если явно не выбрана другая модель, обычные сессии используют слот sonnet.',
        placeholder: 'tingly/cc-sonnet',
    },
    ANTHROPIC_DEFAULT_OPUS_MODEL: {
        label: 'Слот Opus',
        purpose: 'Сложные рассуждения (режим планирования, глубокий анализ)',
        tooltip: 'Более дорогая, но более сильная модель. Claude Code использует её, когда opus запрошен явно.',
        placeholder: 'tingly/cc-opus',
    },
    CLAUDE_CODE_SUBAGENT_MODEL: {
        label: 'Модель субагента',
        purpose: 'Модель для субагентов, запускаемых инструментом Task',
        tooltip: 'Субагенты ведут параллельные исследования и независимые подзадачи. Им можно назначить более дешёвую или более сильную модель.',
        placeholder: 'tingly/cc-subagent',
    },
    API_TIMEOUT_MS: {
        label: 'Таймаут API-запроса',
        purpose: 'Максимальное время ожидания одного ответа API',
        tooltip: 'Значение Anthropic по умолчанию — 120000 (2 мин). Для長их проксируемых задач в tb обычно поднимают до 3000000 (50 мин).',
        placeholder: '3000000',
    },
    CLAUDE_CODE_MAX_OUTPUT_TOKENS: {
        label: 'Макс. токенов вывода',
        purpose: 'Верхняя граница числа токенов в одном ответе',
        tooltip: 'Слишком мало — ответ обрежется; слишком много — расходуется квота. tb рекомендует 32000.',
        placeholder: '32000',
    },
    MAX_THINKING_TOKENS: {
        label: 'Бюджет рассуждений',
        purpose: 'Бюджет токенов для расширенных рассуждений',
        tooltip: 'Оставьте пустым, чтобы использовать значение модели по умолчанию. Имеет смысл только для моделей с поддержкой рассуждений.',
        placeholder: '(пусто = значение модели)',
    },
    BASH_DEFAULT_TIMEOUT_MS: {
        label: 'Таймаут Bash',
        purpose: 'Таймаут по умолчанию для одного вызова инструмента Bash',
        tooltip: 'Значение Anthropic по умолчанию — 120000. Поднимите, если длинные скрипты (например, npm install) не успевают завершиться.',
        placeholder: '120000',
    },
    BASH_MAX_TIMEOUT_MS: {
        label: 'Макс. таймаут Bash',
        purpose: 'Потолок для любого таймаута Bash, который запрашивает Claude',
        tooltip: 'Верхний предел, когда Claude сам задаёт таймаут для вызова Bash.',
        placeholder: '600000',
    },
    MCP_TIMEOUT: {
        label: 'Таймаут MCP',
        purpose: 'Таймаут запуска и ответов сервера MCP',
        tooltip: 'Значение Anthropic по умолчанию — 30000. Поднимите для медленно стартующих серверов MCP.',
        placeholder: '30000',
    },
    MCP_TOOL_TIMEOUT: {
        label: 'Таймаут инстр. MCP',
        purpose: 'Таймаут одного вызова инструмента MCP',
        tooltip: 'Значение Anthropic по умолчанию — 10000.',
        placeholder: '10000',
    },
    MAX_MCP_OUTPUT_TOKENS: {
        label: 'Лимит вывода MCP',
        purpose: 'Максимум токенов, возвращаемых одним вызовом инструмента MCP',
        tooltip: 'Значение Anthropic по умолчанию — 8192. Всё сверх этого обрезается.',
        placeholder: '8192',
    },
    CLAUDE_CODE_AUTO_COMPACT_WINDOW: {
        label: 'Окно автосжатия',
        purpose: 'Целевой размер окна при автоматическом сжатии контекста',
        tooltip: 'Значение tb по умолчанию — 200000 (для моделей на 1M автоматически повышается до 1000000). При срабатывании автосжатия сохраняются последние N токенов. Больше значение — больше сохранённого контекста, но и больше расход квоты.',
        placeholder: '200000',
    },
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: {
        label: 'Порог автосжатия',
        purpose: 'Процент заполнения контекста, при котором срабатывает автосжатие',
        tooltip: 'Значение tb по умолчанию — 80. Автосжатие срабатывает, когда заполнение контекста достигает этого процента. Меньше — сжатие раньше, больше — позже. 0 отключает автосжатие.',
        placeholder: '80',
    },
    DISABLE_TELEMETRY: {
        label: 'Отключить телеметрию',
        purpose: 'Выключить отправку телеметрии в Anthropic',
        tooltip: 'tb включает это по умолчанию, чтобы внутренние и приватные развёртывания не отправляли лишнего.',
    },
    DISABLE_ERROR_REPORTING: {
        label: 'Отключить отчёты',
        purpose: 'Выключить автоматическую отправку отчётов о сбоях в Anthropic',
        tooltip: 'tb включает это по умолчанию.',
    },
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: {
        label: 'Убрать лишний трафик',
        purpose: 'Подавить проверки обновлений, опросы и прочие фоновые запросы',
        tooltip: 'Самый «чистый» режим — наружу уходят только вызовы моделей. tb включает это по умолчанию.',
    },
    DISABLE_AUTOUPDATER: {
        label: 'Без автообновления',
        purpose: 'Запретить Claude Code проверять наличие новых версий',
        tooltip: 'Обычный выбор для развёртываний с зафиксированной версией.',
    },
    USE_BUILTIN_RIPGREP: {
        label: 'Встроенный ripgrep',
        purpose: 'Предпочитать ripgrep из поставки Claude Code, а не из системного PATH',
        tooltip: 'Включено по умолчанию. Отключайте, только если нужен собственный системный ripgrep.',
    },
    HTTP_PROXY: {
        label: 'HTTP-прокси',
        purpose: 'Прокси для исходящих HTTP-запросов Claude Code',
        tooltip: 'Формат: http://host:port. Оставьте пустым, чтобы унаследовать системные настройки прокси. Учтите: системные прокси не исключают localhost автоматически, из-за чего запросы к локальному шлюзу tb могут возвращать 502.',
        placeholder: 'http://proxy.example.com:8080',
    },
    HTTPS_PROXY: {
        label: 'HTTPS-прокси',
        purpose: 'Прокси для исходящих HTTPS-запросов Claude Code',
        tooltip: 'Формат: http://host:port или https://host:port. Оставьте пустым, чтобы унаследовать системные настройки прокси.',
        placeholder: 'http://proxy.example.com:8080',
    },
    NO_PROXY: {
        label: 'Исключения прокси',
        purpose: 'Хосты через запятую, которые идут в обход прокси',
        tooltip: 'Например, «localhost,127.0.0.1,::1». tb сам добавляет localhost/127.0.0.1/::1 при запуске, даже если поле пустое. Лучше оставить пустым (tb управляет этим сам) и заполнять, только если нужно исключить дополнительные внутренние хосты.',
        placeholder: 'localhost,127.0.0.1,::1',
    },
};

const FIELDS_TEXT_FA: FieldTextMap = {
    ANTHROPIC_MODEL: {
        label: 'مدل پیش‌فرض',
        purpose: 'مدل جایگزین وقتی هیچ اسلات ویژه‌ای مناسب نباشد',
        tooltip: 'وقتی مسیریابی تخصصی تطبیق نکند، Claude Code سراغ این مدل می‌رود. tb معمولاً آن را به tingly/cc یا tingly/cc-default نگاشت می‌کند.',
        placeholder: 'tingly/cc',
    },
    ANTHROPIC_DEFAULT_HAIKU_MODEL: {
        label: 'اسلات Haiku',
        purpose: 'کارهای سبک مانند پیام کامیت و خلاصه‌سازی',
        tooltip: 'Claude Code فراخوانی‌های کمکی ارزان را به اسلات haiku می‌فرستد. tb آن را به tingly/cc-haiku وصل می‌کند.',
        placeholder: 'tingly/cc-haiku',
    },
    ANTHROPIC_DEFAULT_SONNET_MODEL: {
        label: 'اسلات Sonnet',
        purpose: 'اسلات اصلی — بیشتر گفت‌وگو و تولید کد اینجا انجام می‌شود',
        tooltip: 'پیش‌فرض Claude Code. تا وقتی مدل دیگری را صریحاً انتخاب نکنید، نشست‌های عادی از اسلات sonnet استفاده می‌کنند.',
        placeholder: 'tingly/cc-sonnet',
    },
    ANTHROPIC_DEFAULT_OPUS_MODEL: {
        label: 'اسلات Opus',
        purpose: 'استدلال سنگین‌تر (حالت برنامه‌ریزی، تحلیل عمیق)',
        tooltip: 'مدلی گران‌تر اما قوی‌تر. Claude Code وقتی opus صریحاً خواسته شود از آن استفاده می‌کند.',
        placeholder: 'tingly/cc-opus',
    },
    CLAUDE_CODE_SUBAGENT_MODEL: {
        label: 'مدل زیرایجنت',
        purpose: 'مدل زیرایجنت‌هایی که با ابزار Task ساخته می‌شوند',
        tooltip: 'زیرایجنت‌ها پژوهش موازی و زیروظیفه‌های مستقل را انجام می‌دهند. می‌توانید مدلی ارزان‌تر یا قوی‌تر به آن‌ها بدهید.',
        placeholder: 'tingly/cc-subagent',
    },
    API_TIMEOUT_MS: {
        label: 'مهلت درخواست API',
        purpose: 'بیشترین زمان انتظار برای یک پاسخ API',
        tooltip: 'پیش‌فرض Anthropic ‏۱۲۰۰۰۰ (۲ دقیقه) است. برای کارهای طولانی پراکسی‌شده در tb معمولاً آن را تا ۳۰۰۰۰۰۰ (۵۰ دقیقه) بالا می‌برند.',
        placeholder: '3000000',
    },
    CLAUDE_CODE_MAX_OUTPUT_TOKENS: {
        label: 'بیشینهٔ توکن خروجی',
        purpose: 'سقف تعداد توکن در یک پاسخ',
        tooltip: 'خیلی کم باعث بریدن پاسخ می‌شود و خیلی زیاد سهمیه را هدر می‌دهد. tb مقدار ۳۲۰۰۰ را پیشنهاد می‌کند.',
        placeholder: '32000',
    },
    MAX_THINKING_TOKENS: {
        label: 'بودجهٔ توکن استدلال',
        purpose: 'بودجهٔ توکن برای استدلال گسترده',
        tooltip: 'برای استفاده از مقدار پیش‌فرض مدل خالی بگذارید. تنها برای مدل‌های دارای قابلیت استدلال معنا دارد.',
        placeholder: '(خالی = پیش‌فرض مدل)',
    },
    BASH_DEFAULT_TIMEOUT_MS: {
        label: 'مهلت پیش‌فرض Bash',
        purpose: 'مهلت پیش‌فرض یک فراخوانی ابزار Bash',
        tooltip: 'پیش‌فرض Anthropic ‏۱۲۰۰۰۰ است. اگر اسکریپت‌های طولانی (مثلاً npm install) به مهلت می‌خورند آن را بالا ببرید.',
        placeholder: '120000',
    },
    BASH_MAX_TIMEOUT_MS: {
        label: 'بیشینهٔ مهلت Bash',
        purpose: 'سقف هر مهلتی که Claude برای Bash درخواست می‌کند',
        tooltip: 'حد بالا وقتی Claude خودش مهلت یک فراخوانی Bash را تعیین می‌کند.',
        placeholder: '600000',
    },
    MCP_TIMEOUT: {
        label: 'مهلت اتصال MCP',
        purpose: 'مهلت راه‌اندازی و پاسخ سرور MCP',
        tooltip: 'پیش‌فرض Anthropic ‏۳۰۰۰۰ است. برای سرورهای MCP کندراه‌انداز آن را بالا ببرید.',
        placeholder: '30000',
    },
    MCP_TOOL_TIMEOUT: {
        label: 'مهلت ابزار MCP',
        purpose: 'مهلت یک فراخوانی ابزار MCP',
        tooltip: 'پیش‌فرض Anthropic ‏۱۰۰۰۰ است.',
        placeholder: '10000',
    },
    MAX_MCP_OUTPUT_TOKENS: {
        label: 'سقف خروجی MCP',
        purpose: 'بیشترین توکن بازگشتی از یک فراخوانی ابزار MCP',
        tooltip: 'پیش‌فرض Anthropic ‏۸۱۹۲ است. هر چه بیشتر از آن باشد بریده می‌شود.',
        placeholder: '8192',
    },
    CLAUDE_CODE_AUTO_COMPACT_WINDOW: {
        label: 'پنجرهٔ فشرده‌سازی خودکار',
        purpose: 'اندازهٔ هدف پنجره هنگام فشرده‌سازی خودکار زمینه',
        tooltip: 'پیش‌فرض tb ‏۲۰۰۰۰۰ است (برای مدل‌های ۱M خودکار به ۱۰۰۰۰۰۰ می‌رسد). هنگام فشرده‌سازی خودکار، N توکن آخر نگه داشته می‌شود. مقدار بیشتر یعنی زمینهٔ بیشتر ولی مصرف سهمیهٔ بالاتر.',
        placeholder: '200000',
    },
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: {
        label: 'آستانهٔ فشرده‌سازی خودکار',
        purpose: 'درصد پرشدن زمینه که فشرده‌سازی خودکار را فعال می‌کند',
        tooltip: 'پیش‌فرض tb ‏۸۰ است. وقتی میزان استفاده از زمینه به این درصد برسد فشرده‌سازی خودکار انجام می‌شود. عدد کمتر یعنی زودتر و عدد بیشتر یعنی دیرتر. ‏۰ آن را غیرفعال می‌کند.',
        placeholder: '80',
    },
    DISABLE_TELEMETRY: {
        label: 'غیرفعال کردن تله‌متری',
        purpose: 'خاموش کردن ارسال تله‌متری به Anthropic',
        tooltip: 'tb این گزینه را به‌طور پیش‌فرض روشن می‌کند تا استقرارهای داخلی و خصوصی چیزی بیرون نفرستند.',
    },
    DISABLE_ERROR_REPORTING: {
        label: 'غیرفعال کردن گزارش خطا',
        purpose: 'خاموش کردن ارسال خودکار گزارش خرابی به Anthropic',
        tooltip: 'tb این گزینه را به‌طور پیش‌فرض روشن می‌کند.',
    },
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: {
        label: 'قطع ترافیک غیرضروری',
        purpose: 'خاموش کردن بررسی به‌روزرسانی، نظرسنجی و دیگر درخواست‌های جانبی',
        tooltip: 'تمیزترین حالت — تنها فراخوانی خود مدل‌ها بیرون می‌رود. tb این گزینه را به‌طور پیش‌فرض روشن می‌کند.',
    },
    DISABLE_AUTOUPDATER: {
        label: 'بدون به‌روزرسانی خودکار',
        purpose: 'جلوگیری از بررسی نسخه‌های تازه توسط Claude Code',
        tooltip: 'انتخاب رایج برای استقرارهایی که نسخه در آن‌ها ثابت است.',
    },
    USE_BUILTIN_RIPGREP: {
        label: 'ripgrep درون‌ساخت',
        purpose: 'ترجیح ripgrep همراه Claude Code به‌جای PATH سیستم',
        tooltip: 'به‌طور پیش‌فرض روشن است. تنها وقتی خاموش کنید که به ripgrep سفارشی سیستم نیاز دارید.',
    },
    HTTP_PROXY: {
        label: 'پراکسی HTTP',
        purpose: 'پراکسی درخواست‌های خروجی HTTP از Claude Code',
        tooltip: 'قالب: ‏http://host:port. برای ارث‌بری از تنظیمات پراکسی سیستم خالی بگذارید. توجه: پراکسی سیستم به‌طور خودکار localhost را کنار نمی‌گذارد و همین می‌تواند باعث خطای ۵۰۲ در درخواست به دروازهٔ محلی tb شود.',
        placeholder: 'http://proxy.example.com:8080',
    },
    HTTPS_PROXY: {
        label: 'پراکسی HTTPS',
        purpose: 'پراکسی درخواست‌های خروجی HTTPS از Claude Code',
        tooltip: 'قالب: ‏http://host:port یا https://host:port. برای ارث‌بری از تنظیمات پراکسی سیستم خالی بگذارید.',
        placeholder: 'http://proxy.example.com:8080',
    },
    NO_PROXY: {
        label: 'فهرست بدون پراکسی',
        purpose: 'میزبان‌هایی که با کاما جدا شده‌اند و از پراکسی عبور نمی‌کنند',
        tooltip: 'مثلاً «localhost,127.0.0.1,::1». ‏tb هنگام اجرا خودش localhost/127.0.0.1/::1 را اضافه می‌کند، حتی اگر این فیلد خالی باشد. بهتر است خالی بماند تا tb خودش مدیریت کند و تنها وقتی پرش کنید که میزبان داخلی دیگری را باید کنار بگذارید.',
        placeholder: 'localhost,127.0.0.1,::1',
    },
};

const FIELDS_TEXT_AR: FieldTextMap = {
    ANTHROPIC_MODEL: {
        label: 'النموذج الافتراضي',
        purpose: 'النموذج البديل حين لا ينطبق أي مَشغَل مخصص',
        tooltip: 'إليه يلجأ Claude Code حين لا يطابق أي توجيه متخصص. ويناظره tb عادةً بـ tingly/cc أو tingly/cc-default.',
        placeholder: 'tingly/cc',
    },
    ANTHROPIC_DEFAULT_HAIKU_MODEL: {
        label: 'مَشغَل Haiku',
        purpose: 'المهام الخفيفة مثل رسائل الالتزام والملخصات',
        tooltip: 'يوجِّه Claude Code الاستدعاءات المساعِدة الرخيصة إلى مَشغَل haiku. ويوجِّهه tb إلى tingly/cc-haiku.',
        placeholder: 'tingly/cc-haiku',
    },
    ANTHROPIC_DEFAULT_SONNET_MODEL: {
        label: 'مَشغَل Sonnet',
        purpose: 'المَشغَل الأساسي — إليه يذهب معظم الحوار وتوليد الشفرة',
        tooltip: 'الافتراضي في Claude Code. وما لم تختر نموذجًا آخر صراحةً، تستخدم الجلسات العادية مَشغَل sonnet.',
        placeholder: 'tingly/cc-sonnet',
    },
    ANTHROPIC_DEFAULT_OPUS_MODEL: {
        label: 'مَشغَل Opus',
        purpose: 'الاستدلال الأثقل (وضع التخطيط والتحليل العميق)',
        tooltip: 'نموذج أغلى لكنه أقوى. ويستخدمه Claude Code حين يُطلب opus صراحةً.',
        placeholder: 'tingly/cc-opus',
    },
    CLAUDE_CODE_SUBAGENT_MODEL: {
        label: 'نموذج الوكيل الفرعي',
        purpose: 'النموذج الذي تستخدمه الوكلاء الفرعية المُنشأة عبر أداة Task',
        tooltip: 'تتولى الوكلاء الفرعية البحث المتوازي والمهام الفرعية المستقلة. ويمكنك منحها نموذجًا أرخص أو أقوى.',
        placeholder: 'tingly/cc-subagent',
    },
    API_TIMEOUT_MS: {
        label: 'مهلة طلب API',
        purpose: 'أقصى مدة انتظار لاستجابة API واحدة',
        tooltip: 'الافتراضي لدى Anthropic ‏١٢٠٠٠٠ (دقيقتان). وللمهام الطويلة المُمرَّرة عبر tb يُرفَع عادةً إلى ٣٠٠٠٠٠٠ (٥٠ دقيقة).',
        placeholder: '3000000',
    },
    CLAUDE_CODE_MAX_OUTPUT_TOKENS: {
        label: 'أقصى رموز الإخراج',
        purpose: 'الحد الأعلى لعدد الرموز في الرد الواحد',
        tooltip: 'القليل جدًا يبتر الرد، والكثير جدًا يهدر الحصة. ويوصي tb بالقيمة ٣٢٠٠٠.',
        placeholder: '32000',
    },
    MAX_THINKING_TOKENS: {
        label: 'ميزانية رموز الاستدلال',
        purpose: 'ميزانية الرموز للاستدلال الموسَّع',
        tooltip: 'اتركه فارغًا لاستخدام القيمة الافتراضية للنموذج. ولا معنى له إلا مع النماذج القادرة على الاستدلال.',
        placeholder: '(فارغ = افتراضي النموذج)',
    },
    BASH_DEFAULT_TIMEOUT_MS: {
        label: 'مهلة Bash الافتراضية',
        purpose: 'المهلة الافتراضية لاستدعاء واحد لأداة Bash',
        tooltip: 'الافتراضي لدى Anthropic ‏١٢٠٠٠٠. ارفعه إذا كانت السكربتات الطويلة (مثل npm install) تتجاوز المهلة.',
        placeholder: '120000',
    },
    BASH_MAX_TIMEOUT_MS: {
        label: 'أقصى مهلة Bash',
        purpose: 'السقف لأي مهلة يطلبها Claude لـ Bash',
        tooltip: 'الحد الأعلى حين يضبط Claude بنفسه مهلة استدعاء Bash.',
        placeholder: '600000',
    },
    MCP_TIMEOUT: {
        label: 'مهلة اتصال MCP',
        purpose: 'مهلة إقلاع خادم MCP واستجابته',
        tooltip: 'الافتراضي لدى Anthropic ‏٣٠٠٠٠. ارفعه لخوادم MCP بطيئة الإقلاع.',
        placeholder: '30000',
    },
    MCP_TOOL_TIMEOUT: {
        label: 'مهلة أداة MCP',
        purpose: 'مهلة استدعاء واحد لأداة MCP',
        tooltip: 'الافتراضي لدى Anthropic ‏١٠٠٠٠.',
        placeholder: '10000',
    },
    MAX_MCP_OUTPUT_TOKENS: {
        label: 'سقف إخراج MCP',
        purpose: 'أقصى عدد رموز يعيدها استدعاء واحد لأداة MCP',
        tooltip: 'الافتراضي لدى Anthropic ‏٨١٩٢. وما زاد عن ذلك يُبتر.',
        placeholder: '8192',
    },
    CLAUDE_CODE_AUTO_COMPACT_WINDOW: {
        label: 'نافذة الضغط التلقائي',
        purpose: 'الحجم المستهدف للنافذة عند الضغط التلقائي للسياق',
        tooltip: 'الافتراضي في tb هو ٢٠٠٠٠٠ (ويُرفَع تلقائيًا إلى ١٠٠٠٠٠٠ لنماذج 1M). وعند تفعُّل الضغط التلقائي يُحتفظ بآخر N من الرموز. والقيم الأعلى تحفظ سياقًا أكثر لكنها تستهلك حصة أكبر.',
        placeholder: '200000',
    },
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: {
        label: 'عتبة الضغط التلقائي',
        purpose: 'نسبة امتلاء السياق التي تُفعِّل الضغط التلقائي',
        tooltip: 'الافتراضي في tb هو ٨٠. ويُفعَّل الضغط التلقائي حين يبلغ استهلاك السياق هذه النسبة. والقيمة الأقل تعني ضغطًا أبكر والأعلى أمتأخِّر. والقيمة ٠ تعطِّله.',
        placeholder: '80',
    },
    DISABLE_TELEMETRY: {
        label: 'تعطيل القياس عن بُعد',
        purpose: 'إيقاف إرسال بيانات القياس إلى Anthropic',
        tooltip: 'يفعِّله tb افتراضيًا كي لا ترسل عمليات النشر الداخلية والخاصة شيئًا للخارج.',
    },
    DISABLE_ERROR_REPORTING: {
        label: 'تعطيل تقارير الأخطاء',
        purpose: 'إيقاف الرفع التلقائي لتقارير الأعطال إلى Anthropic',
        tooltip: 'يفعِّله tb افتراضيًا.',
    },
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: {
        label: 'إيقاف الحركة غير الضرورية',
        purpose: 'كتم فحوص التحديث والاستبيانات وسائر الاستدعاءات الجانبية',
        tooltip: 'أنظف وضع — فلا يخرج سوى استدعاءات النماذج نفسها. ويفعِّله tb افتراضيًا.',
    },
    DISABLE_AUTOUPDATER: {
        label: 'تعطيل التحديث التلقائي',
        purpose: 'منع Claude Code من البحث عن إصدارات جديدة',
        tooltip: 'خيار معتاد لعمليات النشر ذات الإصدار المثبَّت.',
    },
    USE_BUILTIN_RIPGREP: {
        label: 'ripgrep المدمج',
        purpose: 'تفضيل ripgrep المرفق مع Claude Code على مسار النظام',
        tooltip: 'مُفعَّل افتراضيًا. لا تعطِّله إلا إذا كنت تحتاج إلى ripgrep مخصص من النظام.',
    },
    HTTP_PROXY: {
        label: 'وكيل HTTP',
        purpose: 'الوكيل المستخدَم لطلبات HTTP الصادرة من Claude Code',
        tooltip: 'الصيغة: ‏http://host:port. اتركه فارغًا لوراثة إعدادات وكيل النظام. ملاحظة: لا تستثني وكلاء النظام العنوان localhost تلقائيًا، وقد يسبِّب ذلك أخطاء ٥٠٢ عند الطلبات الموجَّهة إلى بوابة tb المحلية.',
        placeholder: 'http://proxy.example.com:8080',
    },
    HTTPS_PROXY: {
        label: 'وكيل HTTPS',
        purpose: 'الوكيل المستخدَم لطلبات HTTPS الصادرة من Claude Code',
        tooltip: 'الصيغة: ‏http://host:port أو https://host:port. اتركه فارغًا لوراثة إعدادات وكيل النظام.',
        placeholder: 'http://proxy.example.com:8080',
    },
    NO_PROXY: {
        label: 'قائمة تجاوز الوكيل',
        purpose: 'مضيفون مفصولون بفواصل يتجاوزون الوكيل',
        tooltip: 'مثل «localhost,127.0.0.1,::1». ويضيف tb تلقائيًا عند الإقلاع localhost/127.0.0.1/::1 حتى لو تُرك الحقل فارغًا. والأفضل تركه فارغًا ليديره tb، ولا تملأه إلا إذا احتجت إلى تجاوز مضيفين داخليين إضافيين.',
        placeholder: 'localhost,127.0.0.1,::1',
    },
};

interface SectionText { title: string; hint: string }
type SectionTextMap = Record<Group, SectionText>;

interface DefaultModeOptionText {
    label: string;
    description: string;
}

export const CLAUDE_CODE_DEFAULT_MODE_OPTIONS: ClaudeCodeDefaultMode[] = ['acceptEdits', 'default', 'manual', 'plan', 'auto', 'delegate', 'dontAsk', 'bypassPermissions'];

// Mirrors agent.DefaultClaudeCodeShowThinkingSummaries in internal/agent/prefs.go.
export const CLAUDE_CODE_DEFAULT_SHOW_THINKING_SUMMARIES = true;

const DEFAULT_MODE_TEXT_ZH: Record<ClaudeCodeDefaultMode, DefaultModeOptionText> = {
    acceptEdits: { label: '接受编辑（推荐）', description: '自动接受文件编辑，其他高风险操作仍按 Claude Code 规则处理。' },
    default: { label: '默认', description: '使用 Claude Code 官方默认权限行为。' },
    manual: { label: '手动确认', description: '工具权限请求需要交互确认。' },
    plan: { label: '计划模式', description: '默认进入 plan mode，先规划再执行。' },
    auto: { label: '自动规则', description: '由 Claude Code 的规则分类器允许、软拒绝或硬拒绝工具调用。' },
    delegate: { label: '委托', description: '把权限决策委托给 Claude Code 支持的外部流程。' },
    dontAsk: { label: '不询问', description: '避免交互式询问；适合无人值守场景。' },
    bypassPermissions: { label: '绕过权限', description: '跳过权限检查；仅在完全可信环境中使用。' },
};

const DEFAULT_MODE_TEXT_EN: Record<ClaudeCodeDefaultMode, DefaultModeOptionText> = {
    acceptEdits: { label: 'Accept edits (recommended)', description: 'Automatically accepts file edits while leaving riskier actions to Claude Code rules.' },
    default: { label: 'Default', description: 'Use Claude Code\'s built-in default permission behavior.' },
    manual: { label: 'Manual approval', description: 'Require interactive approval for tool permission requests.' },
    plan: { label: 'Plan mode', description: 'Start in plan mode by default before implementation.' },
    auto: { label: 'Auto rules', description: 'Use Claude Code\'s rule classifier to allow, soft-deny, or hard-deny tool calls.' },
    delegate: { label: 'Delegate', description: 'Delegate permission decisions to Claude Code\'s supported external flow.' },
    dontAsk: { label: 'Don\'t ask', description: 'Avoid interactive prompts; useful for unattended setups.' },
    bypassPermissions: { label: 'Bypass permissions', description: 'Skip permission checks; use only in fully trusted environments.' },
};

const DEFAULT_MODE_TEXT_RU: Record<ClaudeCodeDefaultMode, DefaultModeOptionText> = {
    acceptEdits: { label: 'Принимать правки (рекомендуется)', description: 'Правки файлов принимаются автоматически, более рискованные действия остаются на усмотрение правил Claude Code.' },
    default: { label: 'По умолчанию', description: 'Встроенное поведение прав доступа Claude Code.' },
    manual: { label: 'Ручное подтверждение', description: 'Каждый запрос прав на инструмент требует интерактивного подтверждения.' },
    plan: { label: 'Режим планирования', description: 'Начинать в режиме планирования, до реализации.' },
    auto: { label: 'Автоправила', description: 'Классификатор правил Claude Code сам разрешает, мягко или жёстко отклоняет вызовы инструментов.' },
    delegate: { label: 'Делегирование', description: 'Передать решения о правах внешнему процессу, поддерживаемому Claude Code.' },
    dontAsk: { label: 'Не спрашивать', description: 'Избегать интерактивных запросов; удобно для работы без присмотра.' },
    bypassPermissions: { label: 'Обходить проверку прав', description: 'Пропускать проверки прав; только в полностью доверенном окружении.' },
};

const DEFAULT_MODE_TEXT_FA: Record<ClaudeCodeDefaultMode, DefaultModeOptionText> = {
    acceptEdits: { label: 'پذیرش ویرایش‌ها (پیشنهادی)', description: 'ویرایش فایل‌ها خودکار پذیرفته می‌شود و کارهای پرخطرتر به قواعد Claude Code سپرده می‌شود.' },
    default: { label: 'پیش‌فرض', description: 'رفتار پیش‌فرض درون‌ساخت Claude Code برای دسترسی‌ها.' },
    manual: { label: 'تأیید دستی', description: 'هر درخواست دسترسی ابزار به تأیید تعاملی نیاز دارد.' },
    plan: { label: 'حالت برنامه‌ریزی', description: 'به‌طور پیش‌فرض پیش از پیاده‌سازی، در حالت برنامه‌ریزی شروع می‌شود.' },
    auto: { label: 'قواعد خودکار', description: 'دسته‌بند قواعد Claude Code خودش فراخوانی ابزارها را مجاز، نرم یا سخت رد می‌کند.' },
    delegate: { label: 'واگذاری', description: 'تصمیم‌های دسترسی به فرایند بیرونی پشتیبانی‌شدهٔ Claude Code واگذار می‌شود.' },
    dontAsk: { label: 'بدون پرسش', description: 'از پرسش‌های تعاملی پرهیز می‌کند؛ مناسب اجرای بدون نظارت.' },
    bypassPermissions: { label: 'دور زدن دسترسی‌ها', description: 'بررسی دسترسی‌ها انجام نمی‌شود؛ تنها در محیط کاملاً مورد اعتماد.' },
};

const DEFAULT_MODE_TEXT_AR: Record<ClaudeCodeDefaultMode, DefaultModeOptionText> = {
    acceptEdits: { label: 'قبول التعديلات (موصى به)', description: 'تُقبل تعديلات الملفات تلقائيًا، وتُترك الإجراءات الأخطر لقواعد Claude Code.' },
    default: { label: 'افتراضي', description: 'سلوك الأذونات الافتراضي المدمج في Claude Code.' },
    manual: { label: 'موافقة يدوية', description: 'كل طلب إذن لأداة يتطلب موافقة تفاعلية.' },
    plan: { label: 'وضع التخطيط', description: 'يبدأ في وضع التخطيط افتراضيًا قبل التنفيذ.' },
    auto: { label: 'قواعد تلقائية', description: 'يتولى مصنِّف القواعد في Claude Code السماح أو الرفض الليّن أو الرفض القاطع لاستدعاءات الأدوات.' },
    delegate: { label: 'تفويض', description: 'تُفوَّض قرارات الأذونات إلى الإجراء الخارجي الذي يدعمه Claude Code.' },
    dontAsk: { label: 'بلا استئذان', description: 'يتجنَّب المطالبات التفاعلية؛ مفيد للتشغيل دون إشراف.' },
    bypassPermissions: { label: 'تجاوز الأذونات', description: 'يتخطى فحوص الأذونات؛ استخدمه في البيئات الموثوقة تمامًا فقط.' },
};

export const CLAUDE_CODE_DEFAULT_MODE_TEXT: Record<AppLanguage, Record<ClaudeCodeDefaultMode, DefaultModeOptionText>> = {
    zh: DEFAULT_MODE_TEXT_ZH,
    en: DEFAULT_MODE_TEXT_EN,
    ru: DEFAULT_MODE_TEXT_RU,
    fa: DEFAULT_MODE_TEXT_FA,
    ar: DEFAULT_MODE_TEXT_AR,
};

const DEFAULT_MODE_SECTION_TEXT: Record<AppLanguage, SectionText> = {
    zh: {
        title: '默认权限模式',
        hint: '写入 settings.json 的 defaultMode；tb 推荐 acceptEdits。',
    },
    en: {
        title: 'Default permission mode',
        hint: 'Writes defaultMode in settings.json; tb recommends acceptEdits.',
    },
    ru: {
        title: 'Режим прав по умолчанию',
        hint: 'Записывает defaultMode в settings.json; tb рекомендует acceptEdits.',
    },
    fa: {
        title: 'حالت دسترسی پیش‌فرض',
        hint: 'مقدار defaultMode را در settings.json می‌نویسد؛ tb گزینهٔ acceptEdits را پیشنهاد می‌کند.',
    },
    ar: {
        title: 'وضع الأذونات الافتراضي',
        hint: 'يكتب defaultMode في settings.json؛ ويوصي tb بـ acceptEdits.',
    },
};

interface ToggleSettingText {
    title: string;
    hint: string;
    label: string;
    tooltip: string;
}

const SHOW_THINKING_SUMMARIES_TEXT: Record<AppLanguage, ToggleSettingText> = {
    zh: {
        title: '思考摘要',
        hint: '写入 settings.json 的顶层 showThinkingSummaries；不是 env 变量。',
        label: '显示思考摘要',
        tooltip: '开启后 Claude Code 会在回复前展示模型的推理摘要（reasoning）。关闭仅隐藏摘要展示，不影响模型是否思考。',
    },
    en: {
        title: 'Thinking summaries',
        hint: 'Writes the top-level showThinkingSummaries in settings.json; not an env var.',
        label: 'Show thinking summaries',
        tooltip: "When on, Claude Code displays the model's reasoning summary before its reply. Turning it off only hides the summary — it doesn't affect whether the model thinks.",
    },
    ru: {
        title: 'Сводки рассуждений',
        hint: 'Записывает showThinkingSummaries на верхнем уровне settings.json; это не переменная окружения.',
        label: 'Показывать сводки',
        tooltip: 'Когда включено, Claude Code показывает сводку рассуждений модели перед ответом. Выключение лишь скрывает сводку — на то, рассуждает ли модель, это не влияет.',
    },
    fa: {
        title: 'خلاصهٔ استدلال',
        hint: 'مقدار showThinkingSummaries را در سطح بالای settings.json می‌نویسد؛ این یک متغیر محیطی نیست.',
        label: 'نمایش خلاصه',
        tooltip: 'وقتی روشن باشد، Claude Code پیش از پاسخ، خلاصهٔ استدلال مدل را نشان می‌دهد. خاموش کردن آن تنها خلاصه را پنهان می‌کند و بر استدلال کردن مدل اثری ندارد.',
    },
    ar: {
        title: 'ملخصات الاستدلال',
        hint: 'يكتب showThinkingSummaries في المستوى الأعلى من settings.json؛ وليس متغير بيئة.',
        label: 'إظهار الملخصات',
        tooltip: 'عند التفعيل يعرض Claude Code ملخص استدلال النموذج قبل ردِّه. وإيقافه يخفي الملخص فقط ولا يؤثر في كون النموذج يستدل أم لا.',
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

const SECTION_TEXT_RU: SectionTextMap = {
    behavior: DEFAULT_MODE_SECTION_TEXT.ru,
    model: {
        title: 'Маршрутизация моделей',
        hint: 'Каждый слот соответствует одному внутреннему назначению в Claude Code. Чтобы использовать одну модель, укажите одно и то же значение во всех 5 слотах.',
    },
    limits: {
        title: 'Производительность и ограничения',
        hint: 'Пусто — переменная не записывается, и Claude Code использует своё значение по умолчанию.',
    },
    switches: {
        title: 'Приватность и поведение',
        hint: 'Вкл — записывается «1»; выкл — не записывается.',
    },
    network: {
        title: 'Сетевой прокси',
        hint: 'Пусто — не записывается. tb всегда сам добавляет localhost/127.0.0.1/::1 в NO_PROXY, вручную указывать не нужно.',
    },
};

const SECTION_TEXT_FA: SectionTextMap = {
    behavior: DEFAULT_MODE_SECTION_TEXT.fa,
    model: {
        title: 'مسیریابی مدل‌ها',
        hint: 'هر اسلات به یکی از کاربردهای درونی Claude Code مربوط است. برای استفاده از یک مدل، هر ۵ اسلات را با همان مقدار پر کنید.',
    },
    limits: {
        title: 'کارایی و محدودیت‌ها',
        hint: 'خالی یعنی این متغیر نوشته نمی‌شود و Claude Code از مقدار پیش‌فرض خودش استفاده می‌کند.',
    },
    switches: {
        title: 'حریم خصوصی و رفتار',
        hint: 'روشن یعنی مقدار «1» نوشته می‌شود؛ خاموش یعنی نوشته نمی‌شود.',
    },
    network: {
        title: 'پراکسی شبکه',
        hint: 'خالی یعنی نوشته نمی‌شود. ‏tb همیشه خودش localhost/127.0.0.1/::1 را به NO_PROXY اضافه می‌کند و نیازی به وارد کردن دستی نیست.',
    },
};

const SECTION_TEXT_AR: SectionTextMap = {
    behavior: DEFAULT_MODE_SECTION_TEXT.ar,
    model: {
        title: 'توجيه النماذج',
        hint: 'يقابل كل مَشغَل أحد الاستخدامات الداخلية في Claude Code. ولاستخدام نموذج واحد، املأ المَشاغِل الخمسة بالقيمة نفسها.',
    },
    limits: {
        title: 'الأداء والحدود',
        hint: 'الفراغ يعني عدم كتابة المتغير، فيستخدم Claude Code قيمته الافتراضية.',
    },
    switches: {
        title: 'الخصوصية والسلوك',
        hint: 'التشغيل يعني كتابة «1»؛ والإيقاف يعني عدم الكتابة.',
    },
    network: {
        title: 'وكيل الشبكة',
        hint: 'الفراغ يعني عدم الكتابة. ويضيف tb دائمًا localhost/127.0.0.1/::1 إلى NO_PROXY تلقائيًا، فلا حاجة إلى إدخالها يدويًا.',
    },
};

interface UIText {
    oneMTooltip: string;
}

const UI_TEXT_ZH: UIText = {
    oneMTooltip: '启用 1M 上下文窗口（在模型 ID 末尾追加 [1m]，需路由的目标模型支持）',
};

const UI_TEXT_EN: UIText = {
    oneMTooltip: 'Enable the 1M context window (appends [1m] to the model ID; the routed target model must support it).',
};

const UI_TEXT_RU: UIText = {
    oneMTooltip: 'Включить контекстное окно 1M (к ID модели добавляется [1m]; целевая модель маршрута должна это поддерживать).',
};

const UI_TEXT_FA: UIText = {
    oneMTooltip: 'فعال کردن پنجرهٔ زمینهٔ ۱M (پسوند [1m] به شناسهٔ مدل افزوده می‌شود؛ مدل مقصد مسیر باید از آن پشتیبانی کند).',
};

const UI_TEXT_AR: UIText = {
    oneMTooltip: 'تفعيل نافذة السياق 1M (تُضاف اللاحقة [1m] إلى معرِّف النموذج؛ ويجب أن يدعمها النموذج الهدف في المسار).',
};

export const CLAUDE_CODE_FIELDS_TEXT: Record<AppLanguage, FieldTextMap> = { zh: FIELDS_TEXT_ZH, en: FIELDS_TEXT_EN, ru: FIELDS_TEXT_RU, fa: FIELDS_TEXT_FA, ar: FIELDS_TEXT_AR };
const SECTION_TEXT: Record<AppLanguage, SectionTextMap> = { zh: SECTION_TEXT_ZH, en: SECTION_TEXT_EN, ru: SECTION_TEXT_RU, fa: SECTION_TEXT_FA, ar: SECTION_TEXT_AR };
const UI_TEXT: Record<AppLanguage, UIText> = { zh: UI_TEXT_ZH, en: UI_TEXT_EN, ru: UI_TEXT_RU, fa: UI_TEXT_FA, ar: UI_TEXT_AR };

const useLang = (): AppLanguage => {
    const { i18n } = useTranslation();
    return resolveLanguage(i18n.language);
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
    const unifiedRule = rules.find((r: any) => r?.uuid === 'builtin:claude_code:cc');
    const modelForVariant = (variant: string, fallback: string): string => {
        if (mode === 'unified') return unifiedRule?.request_model || fallback;
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
            return getContext1MStateForRule(unifiedRule);
        }
        // In separate mode, check the specific rule for this variant
        const rule = rules.find((r: any) => r?.uuid === `builtin:claude_code:${variant}`);
        return getContext1MStateForRule(rule);
    };

    const context1MEnabled = mode === 'unified'
        ? getContext1MStateForRule(unifiedRule)
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
                display: 'grid',
                gridTemplateColumns: CLAUDE_CONFIG_ROW_COLUMNS,
                alignItems: 'center',
                columnGap: 2,
                rowGap: 1,
                px: 1.5,
                py: 1.1,
                minHeight: 56,
            }}
        >
            {/* Col 1 — Label + info icon */}
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                <Typography variant="body2" noWrap sx={{
                    fontWeight: 600,
                    color: 'text.primary',
                }}>{text.label}</Typography>
                <Tooltip placement="top" arrow title={richTooltip}>
                    <InfoOutlinedIcon sx={{ fontSize: 14, color: 'text.disabled', cursor: 'help' }} />
                </Tooltip>
            </Box>
            {/* Col 2 — env name as a subtle code badge */}
            <Box sx={{ minWidth: 0 }}>
                <Box component="span" sx={CLAUDE_CONFIG_KEY_SX}>
                    {field.envName}
                </Box>
            </Box>
            {/* Col 3 — control, right-aligned */}
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 1.5, minWidth: 0 }}>
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
                        slotProps={{
                            input: {
                                endAdornment: field.unit
                                    ? <InputAdornment position="end"><Typography variant="caption" sx={{
                                    color: "text.disabled"
                                }}>{field.unit}</Typography></InputAdornment>
                                    : undefined,
                                sx: { fontFamily: field.kind === 'model' ? 'monospace' : undefined, fontSize: '0.85rem' },
                            }
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
    lang: AppLanguage;
    prefs: ClaudeCodePrefs;
    setPrefs: (p: ClaudeCodePrefs) => void;
}

const Section: React.FC<SectionProps> = ({ group, lang, prefs, setPrefs }) => {
    const [expanded, setExpanded] = React.useState(group === 'model'); // Only model group expanded by default
    const meta = SECTION_TEXT[lang][group];
    const fieldsText = CLAUDE_CODE_FIELDS_TEXT[lang];
    const oneMTooltip = UI_TEXT[lang].oneMTooltip;
    const fields = CLAUDE_CODE_FIELD_STRUCT.filter(f => f.group === group);
    const hasAdvancedFields = fields.some(f => f.advanced);

    const toggleExpanded = () => setExpanded(!expanded);

    return (
        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1.5, overflow: 'hidden' }}>
            <Box
                onClick={hasAdvancedFields ? toggleExpanded : undefined}
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 1.5,
                    px: 1.5,
                    py: 1.15,
                    bgcolor: 'action.hover',
                    cursor: hasAdvancedFields ? 'pointer' : 'default',
                }}
            >
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography variant="subtitle1" sx={{ fontWeight: 700, color: 'text.primary', lineHeight: 1.35 }}>{meta.title}</Typography>
                    <Typography variant="body2" sx={{ mt: 0.25, color: 'text.secondary', lineHeight: 1.45 }}>{meta.hint}</Typography>
                </Box>
                {hasAdvancedFields && (
                    <IconButton
                        size="small"
                        onClick={(event) => {
                            event.stopPropagation();
                            toggleExpanded();
                        }}
                        aria-label={expanded ? `Collapse ${meta.title}` : `Expand ${meta.title}`}
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
                <Stack divider={<Divider flexItem sx={{ mx: 1.5 }} />} sx={{ borderTop: '1px solid', borderColor: 'divider' }}>
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

// Shared shell for a single "top-level settings.json key" row: a titled card
// with one label/key-badge/control row. Both DefaultModeSection (Select) and
// ShowThinkingSummariesSection (Switch) are peers of this — same JSON layer
// (not an env var, unlike the FieldRow-driven prefs groups below), just a
// different control in column 3.
const SettingsRowSection: React.FC<{
    title: string;
    hint: string;
    label: string;
    tooltip: string;
    settingsKey: string;
    control: React.ReactNode;
}> = ({ title, hint, label, tooltip, settingsKey, control }) => (
    <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1.5, overflow: 'hidden' }}>
        <Box sx={{ px: 1.5, py: 1.15, bgcolor: 'action.hover' }}>
            <Typography variant="subtitle1" sx={{ fontWeight: 700, color: 'text.primary', lineHeight: 1.35 }}>{title}</Typography>
            <Typography variant="body2" sx={{ mt: 0.25, color: 'text.secondary', lineHeight: 1.45 }}>{hint}</Typography>
        </Box>
        <Box sx={{ display: 'grid', gridTemplateColumns: CLAUDE_CONFIG_ROW_COLUMNS, alignItems: 'center', columnGap: 2, rowGap: 1, px: 1.5, py: 1.1, minHeight: 56, borderTop: '1px solid', borderColor: 'divider' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                <Typography variant="body2" noWrap sx={{
                    fontWeight: 600,
                    color: 'text.primary',
                }}>{label}</Typography>
                <Tooltip placement="top" arrow title={tooltip}>
                    <InfoOutlinedIcon sx={{ fontSize: 14, color: 'text.disabled', cursor: 'help' }} />
                </Tooltip>
            </Box>
            <Box sx={{ minWidth: 0 }}>
                <Box component="span" sx={CLAUDE_CONFIG_KEY_SX}>
                    {settingsKey}
                </Box>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', minWidth: 0 }}>
                {control}
            </Box>
        </Box>
    </Box>
);

const DefaultModeSection: React.FC<{
    lang: AppLanguage;
    defaultMode: ClaudeCodeDefaultMode;
    setDefaultMode: (mode: ClaudeCodeDefaultMode) => void;
}> = ({ lang, defaultMode, setDefaultMode }) => {
    // The row's own label comes from the locale files rather than the inline
    // bundles above: it names a Claude Code concept the rest of claudeCode.*
    // already covers, not one of this panel's dev-facing env descriptions.
    const { t } = useTranslation();
    const meta = DEFAULT_MODE_SECTION_TEXT[lang];
    const text = CLAUDE_CODE_DEFAULT_MODE_TEXT[lang];
    const selectedText = text[defaultMode];

    return (
        <SettingsRowSection
            title={meta.title}
            hint={meta.hint}
            label={t('claudeCode.defaultModeLabel')}
            tooltip={`${selectedText.label}: ${selectedText.description}`}
            settingsKey="defaultMode"
            control={
                <FormControl size="small" sx={{ width: 360 }}>
                    <Select
                        value={defaultMode}
                        onChange={(e) => setDefaultMode(e.target.value as ClaudeCodeDefaultMode)}
                        renderValue={(value) => {
                            const mode = value as ClaudeCodeDefaultMode;
                            return (
                                <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 2, width: '100%' }}>
                                    <Typography component="span" variant="body2">{text[mode].label}</Typography>
                                    <Typography
                                        component="span"
                                        variant="caption"
                                        sx={{
                                            color: "text.secondary",
                                            fontFamily: 'monospace'
                                        }}>{mode}</Typography>
                                </Box>
                            );
                        }}
                        MenuProps={{
                            slotProps: { paper: { sx: { maxHeight: 320, width: 360 } }, list: { sx: { py: 0.5 } } },
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
                        {CLAUDE_CODE_DEFAULT_MODE_OPTIONS.map((mode) => (
                            <MenuItem key={mode} value={mode} sx={{ minHeight: 40, py: 1 }}>
                                <Tooltip title={text[mode].description} arrow placement="left">
                                    <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 2, width: '100%' }}>
                                        <Typography variant="body2">{text[mode].label}</Typography>
                                        <Typography
                                            variant="caption"
                                            sx={{
                                                color: "text.secondary",
                                                fontFamily: 'monospace'
                                            }}>{mode}</Typography>
                                    </Box>
                                </Tooltip>
                            </MenuItem>
                        ))}
                    </Select>
                </FormControl>
            }
        />
    );
};

const ShowThinkingSummariesSection: React.FC<{
    lang: AppLanguage;
    checked: boolean;
    onChange: (checked: boolean) => void;
}> = ({ lang, checked, onChange }) => {
    const text = SHOW_THINKING_SUMMARIES_TEXT[lang];

    return (
        <SettingsRowSection
            title={text.title}
            hint={text.hint}
            label={text.label}
            tooltip={text.tooltip}
            settingsKey="showThinkingSummaries"
            control={
                <Switch
                    size="small"
                    checked={checked}
                    onChange={(_, c) => onChange(c)}
                />
            }
        />
    );
};

interface QuickConfigPanelWithBehaviorProps extends QuickConfigPanelProps {
    showThinkingSummaries: boolean;
    setShowThinkingSummaries: (show: boolean) => void;
}

const ClaudeCodeQuickConfig: React.FC<QuickConfigPanelWithBehaviorProps> = ({
    prefs,
    setPrefs,
    defaultMode,
    setDefaultMode,
    showThinkingSummaries,
    setShowThinkingSummaries,
}) => {
    const lang = useLang();

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
            <DefaultModeSection lang={lang} defaultMode={defaultMode} setDefaultMode={setDefaultMode} />
            <ShowThinkingSummariesSection lang={lang} checked={showThinkingSummaries} onChange={setShowThinkingSummaries} />
            <Section group="model" lang={lang} prefs={prefs} setPrefs={setPrefs} />
            <Section group="limits" lang={lang} prefs={prefs} setPrefs={setPrefs} />
            <Section group="switches" lang={lang} prefs={prefs} setPrefs={setPrefs} />
            <Section group="network" lang={lang} prefs={prefs} setPrefs={setPrefs} />
        </Box>
    );
};

export default ClaudeCodeQuickConfig;
