import {
    Alert,
    Box,
    Divider,
    MenuItem,
    Select,
    Stack,
    Switch,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import { InfoOutlined as InfoOutlinedIcon } from '@/components/icons';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { type AppLanguage, resolveLanguage } from '@/i18n';
import { fontMono, fontSizes } from '@/theme/fonts';
import { CODEX_COMPACT, planCodexCompact } from '@/components/rule-card/codexCompact';
import { radius } from '@tingly/ui/theme/radius';

// CodexPrefs mirrors the Go struct in internal/config (CodexPrefs).
// Keys are the literal Codex config.toml keys so the object round-trips
// through the backend without an intermediate mapping layer. All values are
// strings; "" means "omit this key, let Codex use its own default".
export interface CodexPrefs {
    model_reasoning_effort?: string;
    model_reasoning_summary?: string;
    model_verbosity?: string;
    supports_reasoning_summaries?: string; // "true" | "" — catalog-only
    // Catalog-only (tingly-model-catalog.json, not config.toml keys): decimal
    // percent strings, "" = tingly-box default.
    effective_context_window_percent?: string;
    auto_compact_percent?: string;
}

// Mirrors codexEffectiveContextWindowPercent / codexDefaultAutoCompactTokenLimitPct
// and the min/max bounds in internal/config/apply_config_codex.go.
const CONTEXT_PERCENT_DEFAULTS = { effective_context_window_percent: 82, auto_compact_percent: 75 } as const;
const CONTEXT_PERCENT_MIN = 50;
const CONTEXT_PERCENT_MAX = 100;

export function defaultCodexPrefs(): CodexPrefs {
    // Position reasoning effort to a concrete default ("medium") instead of
    // leaving it unset — keep this in sync with Go's DefaultCodexPrefs.
    return { model_reasoning_effort: 'medium' };
}

// CODEX_PREF_KEYS is the single source of truth for the durable Codex pref
// keys on the frontend (mirrors MODEL_SLOT_KEYS in claudeCodePrefsState.ts and
// the backend CodexPrefs struct). Adding a key is one entry here, typed against
// keyof CodexPrefs so an omission or typo is a compile error.
const CODEX_PREF_KEYS = [
    'model_reasoning_effort',
    'model_reasoning_summary',
    'model_verbosity',
    'supports_reasoning_summaries',
    'effective_context_window_percent',
    'auto_compact_percent',
] as const satisfies readonly (keyof CodexPrefs)[];

// Merge a previously-applied prefs object over the current defaults so
// reopening the Codex config modal restores durable user choices rather than
// resetting to defaults every time. Empty values on `applied` defer to the
// default (Codex treats "" as "omit, use built-in default").
//
// Unlike Claude Code (where model slots must always come from routing rules),
// Codex has no routing-derived prefs, so a plain shallow merge is correct.
export function mergeSavedCodexPrefs(applied: CodexPrefs = {}): CodexPrefs {
    const merged: CodexPrefs = { ...defaultCodexPrefs() };
    for (const key of CODEX_PREF_KEYS) {
        const v = applied[key];
        if (v !== undefined && v !== '') {
            merged[key] = v;
        }
    }
    return merged;
}

type PrefsKey = keyof CodexPrefs;
type Kind = 'enum' | 'bool' | 'percent';

// ── Field structure (language-agnostic) ────────────────────────────────
// Keep in sync with codexPrefSpec in internal/config/apply_config.go.
// Adding a key: append here AND add entries in FIELDS_TEXT_ZH / FIELDS_TEXT_EN /
// FIELDS_TEXT_RU.

interface FieldStruct {
    key: PrefsKey;
    kind: Kind;
    enumValues?: string[]; // first entry is the implicit "empty" sentinel below
}

// Sentinel rendered in the Select for "leave unset (use Codex default)".
const UNSET = '';

const FIELD_STRUCT: FieldStruct[] = [
    { key: 'model_reasoning_effort', kind: 'enum', enumValues: ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra'] },
    { key: 'model_reasoning_summary', kind: 'enum', enumValues: ['auto', 'concise', 'detailed', 'none'] },
    { key: 'model_verbosity', kind: 'enum', enumValues: ['low', 'medium', 'high'] },
    { key: 'supports_reasoning_summaries', kind: 'bool' },
    { key: 'effective_context_window_percent', kind: 'percent' },
    { key: 'auto_compact_percent', kind: 'percent' },
];

// ── Localized text bundles ─────────────────────────────────────────────

interface FieldText {
    label: string;
    purpose: string;
    tooltip: string;
}

type FieldTextMap = Record<PrefsKey, FieldText>;

const FIELDS_TEXT_ZH: FieldTextMap = {
    model_reasoning_effort: {
        label: '推理强度',
        purpose: '控制模型思考的深度',
        tooltip: 'none/minimal 最快，high/xhigh 更深入但更慢更贵。留空则用 Codex 默认（medium）。',
    },
    model_reasoning_summary: {
        label: '推理摘要',
        purpose: '是否以及如何展示模型的思考过程',
        tooltip: 'auto 让 Codex 自行决定；concise/detailed 控制详略；none 隐藏。tb 默认 auto。',
    },
    model_verbosity: {
        label: '回答详略',
        purpose: '控制回复的啰嗦程度',
        tooltip: 'low 适合简洁的编码助手；high 会给更多解释。留空则用 Codex 默认（medium）。',
    },
    supports_reasoning_summaries: {
        label: '强制推理摘要',
        purpose: '在非 OpenAI 模型上强制开启推理摘要',
        tooltip: '经 tingly-box 转发的模型需要打开此项才能正常返回推理摘要。写入模型目录每个条目的 supports_reasoning_summaries（Codex 的 config.toml 已无对应键），需开启“写入模型目录”才生效。',
    },
    effective_context_window_percent: {
        label: '可用窗口比例 (%)',
        purpose: '上下文窗口里留给输入的比例，其余预留给系统提示、工具开销和输出',
        tooltip: '写入模型目录的 effective_context_window_percent，范围 50–100。越低越保守。留空用 tb 默认（82）。',
    },
    auto_compact_percent: {
        label: '自动压缩阈值 (%)',
        purpose: '上下文用到窗口的这个比例时触发自动压缩',
        tooltip: '写入模型目录的 auto_compact_token_limit（窗口 × 比例），范围 50–100，且不会高于可用窗口比例。留空用 tb 默认（75）。这是所有模型的默认值，单个模型可在规则行的“压缩”芯片里覆盖。',
    },
};

const FIELDS_TEXT_EN: FieldTextMap = {
    model_reasoning_effort: {
        label: 'Reasoning effort',
        purpose: 'How deeply the model thinks',
        tooltip: 'none/minimal are fastest; high/xhigh reason more but are slower and pricier. Empty = Codex default (medium).',
    },
    model_reasoning_summary: {
        label: 'Reasoning summary',
        purpose: 'Whether and how the thinking is surfaced',
        tooltip: 'auto lets Codex decide; concise/detailed control verbosity; none hides it. tb defaults to auto.',
    },
    model_verbosity: {
        label: 'Verbosity',
        purpose: 'How chatty the reply is',
        tooltip: 'low suits a concise coding assistant; high gives more explanation. Empty = Codex default (medium).',
    },
    supports_reasoning_summaries: {
        label: 'Force reasoning summaries',
        purpose: 'Force reasoning summaries on non-OpenAI models',
        tooltip: 'Models proxied through tingly-box need this on to return reasoning summaries. Written to each model catalog entry as supports_reasoning_summaries (Codex\'s config.toml no longer has a matching key), so it only takes effect with "Write model catalog" on.',
    },
    effective_context_window_percent: {
        label: 'Usable window (%)',
        purpose: 'Share of the context window usable for input; the rest is reserved for system prompt, tool overhead and output',
        tooltip: 'Written to the catalog as effective_context_window_percent, range 50–100. Lower is more conservative. Empty = tb default (82).',
    },
    auto_compact_percent: {
        label: 'Auto-compact at (%)',
        purpose: 'Compact the conversation once context reaches this share of the window',
        tooltip: 'Written to the catalog as auto_compact_token_limit (window × percent), range 50–100, never above the usable window Empty = tb default (75). This is the default for every model; a single model can override it with the Compact chip on its rule row.',
    },
};

const FIELDS_TEXT_RU: FieldTextMap = {
    model_reasoning_effort: {
        label: 'Глубина рассуждений',
        purpose: 'Насколько глубоко модель обдумывает ответ',
        tooltip: 'none/minimal — самые быстрые; high/xhigh рассуждают глубже, но медленнее и дороже. Пусто — значение Codex по умолчанию (medium).',
    },
    model_reasoning_summary: {
        label: 'Сводка рассуждений',
        purpose: 'Показывать ли ход рассуждений и насколько подробно',
        tooltip: 'auto оставляет решение за Codex; concise/detailed задают подробность; none скрывает сводку. По умолчанию tb использует auto.',
    },
    model_verbosity: {
        label: 'Подробность ответа',
        purpose: 'Насколько развёрнуто отвечает модель',
        tooltip: 'low подходит для лаконичного помощника по коду; high даёт больше пояснений. Пусто — значение Codex по умолчанию (medium).',
    },
    supports_reasoning_summaries: {
        label: 'Сводки принудительно',
        purpose: 'Включить сводки рассуждений для моделей не от OpenAI',
        tooltip: 'Моделям, проксируемым через tingly-box, эта опция нужна, чтобы возвращать сводки рассуждений. Записывается в каждую запись каталога как supports_reasoning_summaries (в config.toml Codex такого ключа больше нет), поэтому работает только при включённом «Каталоге моделей».',
    },
    effective_context_window_percent: {
        label: 'Доступное окно (%)',
        purpose: 'Доля контекстного окна под ввод; остальное резервируется под системный промпт, инструменты и вывод',
        tooltip: 'Записывается в каталог как effective_context_window_percent, диапазон 50–100. Чем ниже, тем осторожнее. Пусто — значение tb по умолчанию (82).',
    },
    auto_compact_percent: {
        label: 'Автосжатие при (%)',
        purpose: 'Сжимать диалог, когда контекст достигает этой доли окна',
        tooltip: 'Записывается в каталог как auto_compact_token_limit (окно × доля), диапазон 50–100, не выше доступного окна Пусто — значение tb по умолчанию (75). Это значение по умолчанию для всех моделей; для отдельной модели его можно переопределить чипом «Сжатие» в строке правила.',
    },
};

const FIELDS_TEXT: Record<AppLanguage, FieldTextMap> = { zh: FIELDS_TEXT_ZH, en: FIELDS_TEXT_EN, ru: FIELDS_TEXT_RU };

const UI_TEXT: Record<AppLanguage, { panelHeader: string; sectionTitle: string; sectionHint: string; unsetLabel: string }> = {
    zh: {
        panelHeader: '这些项写入 ~/.codex/config.toml 的顶层与每个 tingly profile',
        sectionTitle: '模型与推理',
        sectionHint: '留空表示用 Codex 自带默认',
        unsetLabel: '（默认）',
    },
    en: {
        panelHeader: 'These are written to the top level of ~/.codex/config.toml and into each tingly profile',
        sectionTitle: 'Model & reasoning',
        sectionHint: 'Empty = use Codex built-in default',
        unsetLabel: '(default)',
    },
    ru: {
        panelHeader: 'Эти значения записываются на верхний уровень ~/.codex/config.toml и в каждый профиль tingly',
        sectionTitle: 'Модель и рассуждения',
        sectionHint: 'Пусто — встроенное значение Codex по умолчанию',
        unsetLabel: '(по умолчанию)',
    },
};

function useLang(): AppLanguage {
    const { i18n } = useTranslation();
    return resolveLanguage(i18n.language);
}

// ── Auto-compact warnings ──────────────────────────────────────────────
// The shared default is judged against the default 200K window; a rule that
// sets the 1M flag has far more room (the Compact chip on its header judges it
// against its own window).

const COMPACT_WARN_TEXT: Record<AppLanguage, { low: string; limited: string }> = {
    zh: {
        low: '按 200K 窗口估算，压缩时仅剩约 {remain} tokens（建议不低于 {min}）。长工具输出可能撞上窗口上限，导致压缩失败。',
        limited: '高于「可用窗口 {effective}%」，实际按 {applied}% 生效。要更晚压缩，请先调高可用窗口。',
    },
    en: {
        low: 'On a 200K window only about {remain} tokens would be free when compacting (at least {min} recommended). A long tool output could overflow the window and make compaction fail.',
        limited: 'Higher than the usable window ({effective}%), so {applied}% is what takes effect. To compact later, raise the usable window first.',
    },
    ru: {
        low: 'На окне 200K при сжатии останется около {remain} токенов (рекомендуется не менее {min}). Длинный вывод инструмента может переполнить окно, и сжатие не удастся.',
        limited: 'Выше доступного окна ({effective}%), поэтому действует {applied}%. Чтобы сжимать позже, сначала увеличьте доступное окно.',
    },
};

const parseCompactPercent = (raw: string | undefined, fallback: number): number => {
    const n = Number.parseInt(raw ?? '', 10);
    return Number.isFinite(n) && n >= CONTEXT_PERCENT_MIN && n <= CONTEXT_PERCENT_MAX ? n : fallback;
};

/** The warning for the shared auto-compact percent, or null when it is fine. */
export function compactDefaultWarning(
    prefs: CodexPrefs,
    text: { low: string; limited: string },
): { severity: 'warning' | 'info'; message: string } | null {
    const effective = parseCompactPercent(prefs.effective_context_window_percent, CONTEXT_PERCENT_DEFAULTS.effective_context_window_percent);
    const percent = parseCompactPercent(prefs.auto_compact_percent, CONTEXT_PERCENT_DEFAULTS.auto_compact_percent);
    const plan = planCodexCompact({ window: CODEX_COMPACT.window, percent, effectivePercent: effective });
    const fill = (tpl: string) => tpl
        .replace('{value}', String(percent))
        .replace('{applied}', String(plan.applied))
        .replace('{effective}', String(effective))
        .replace('{remain}', plan.remain.toLocaleString('en-US'))
        .replace('{min}', CODEX_COMPACT.minRemaining.toLocaleString('en-US'));
    switch (plan.status) {
        case 'limited': return { severity: 'info', message: fill(text.limited) };
        case 'low': return { severity: 'warning', message: fill(text.low) };
        default: return null;
    }
}

// ── Field row ──────────────────────────────────────────────────────────

interface FieldRowProps {
    field: FieldStruct;
    text: FieldText;
    unsetLabel: string;
    prefs: CodexPrefs;
    setPrefs: (next: CodexPrefs) => void;
}

const FieldRow: React.FC<FieldRowProps> = ({ field, text, unsetLabel, prefs, setPrefs }) => {
    const value = prefs[field.key] ?? '';
    const setValue = (next: string) => setPrefs({ ...prefs, [field.key]: next });

    const richTooltip = (
        <Box sx={{ maxWidth: 280 }}>
            <Typography variant="caption" sx={{ display: 'block', mb: 0.5 }}>{text.purpose}</Typography>
            <Typography variant="caption" sx={{ display: 'block', opacity: 0.85 }}>{text.tooltip}</Typography>
        </Box>
    );

    const lang = useLang();
    const warning = field.key === 'auto_compact_percent' ? compactDefaultWarning(prefs, COMPACT_WARN_TEXT[lang]) : null;

    return (
        <Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, py: 1, minHeight: 44 }}>
            {/* Col 1 — Label + info icon */}
            <Box sx={{ flex: '0 0 180px', display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                <Typography variant="body2" noWrap sx={{
                    fontWeight: 500
                }}>{text.label}</Typography>
                <Tooltip placement="top" arrow title={richTooltip}>
                    <InfoOutlinedIcon sx={{ fontSize: 14, color: 'text.disabled', cursor: 'help' }} />
                </Tooltip>
            </Box>
            {/* Col 2 — config.toml key as a subtle code badge */}
            <Box sx={{ flex: '0 0 320px', minWidth: 0 }}>
                <Box
                    component="span"
                    sx={{
                        px: 0.75,
                        py: 0.25,
                        borderRadius: radius.chip,
                        bgcolor: 'action.hover',
                        fontFamily: fontMono,
                        fontSize: fontSizes.micro,
                        color: 'text.secondary',
                        whiteSpace: 'nowrap',
                    }}
                >
                    {field.key}
                </Box>
            </Box>
            {/* Col 3 — control, right-aligned */}
            <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 1.5 }}>
                {field.kind === 'bool' && (
                    <Switch
                        size="small"
                        checked={value === 'true'}
                        onChange={(_, c) => setValue(c ? 'true' : '')}
                    />
                )}
                {field.kind === 'percent' && (
                    <TextField
                        size="small"
                        type="number"
                        value={value}
                        placeholder={String(CONTEXT_PERCENT_DEFAULTS[field.key as keyof typeof CONTEXT_PERCENT_DEFAULTS])}
                        onChange={(e) => setValue(e.target.value)}
                        slotProps={{ htmlInput: { min: CONTEXT_PERCENT_MIN, max: CONTEXT_PERCENT_MAX, step: 1 } }}
                        sx={{ width: 160, '& input': { fontSize: fontSizes.md } }}
                    />
                )}
                {field.kind === 'enum' && (
                    <Select
                        size="small"
                        value={value}
                        displayEmpty
                        onChange={(e) => setValue(e.target.value)}
                        sx={{ minWidth: 160, fontSize: fontSizes.md }}
                    >
                        <MenuItem value={UNSET}>
                            <Typography variant="body2" sx={{
                                color: "text.disabled"
                            }}>{unsetLabel}</Typography>
                        </MenuItem>
                        {field.enumValues!.map((v) => (
                            <MenuItem key={v} value={v} sx={{ fontFamily: fontMono, fontSize: fontSizes.md }}>{v}</MenuItem>
                        ))}
                    </Select>
                )}
            </Box>
        </Box>
        {warning && (
            <Alert severity={warning.severity} variant="outlined" sx={{ mb: 1, py: 0, fontSize: fontSizes.sm }}>
                {warning.message}
            </Alert>
        )}
        </Box>
    );
};

// ── Catalog section text ───────────────────────────────────────────────

const CATALOG_TEXT: Record<AppLanguage, { sectionTitle: string; label: string; purpose: string; tooltip: string }> = {
    zh: {
        sectionTitle: '文件',
        label: '写入模型目录',
        purpose: '让 Codex 的 /model 选择器列出 tingly 托管的模型',
        tooltip: '写入 ~/.codex/tingly-model-catalog.json。Codex 启动时读取该文件，将 tingly 服务的模型加入 /model 选择器。关闭后 config.toml 中不写入 model_catalog_json，Codex 使用内置模型列表。',
    },
    en: {
        sectionTitle: 'Files',
        label: 'Write model catalog',
        purpose: 'Lets Codex\'s /model picker list tingly-served models',
        tooltip: 'Writes ~/.codex/tingly-model-catalog.json. Codex reads this on startup to populate the /model picker with tingly-served models. When off, model_catalog_json is omitted from config.toml and Codex uses its built-in model list.',
    },
    ru: {
        sectionTitle: 'Файлы',
        label: 'Каталог моделей',
        purpose: 'Чтобы в выборе /model в Codex появились модели, обслуживаемые tingly',
        tooltip: 'Записывает ~/.codex/tingly-model-catalog.json. Codex читает этот файл при запуске и добавляет модели tingly в выбор /model. Если выключено, model_catalog_json не пишется в config.toml, и Codex использует встроенный список моделей.',
    },
};

// ── Panel ──────────────────────────────────────────────────────────────

interface CodexQuickConfigProps {
    prefs: CodexPrefs;
    setPrefs: (p: CodexPrefs) => void;
    writeCatalog: boolean;
    setWriteCatalog: (v: boolean) => void;
}

const CodexQuickConfig: React.FC<CodexQuickConfigProps> = ({ prefs, setPrefs, writeCatalog, setWriteCatalog }) => {
    const lang = useLang();
    const uiText = UI_TEXT[lang];
    const fieldsText = FIELDS_TEXT[lang];
    const catalogText = CATALOG_TEXT[lang];

    const catalogTooltip = (
        <Box sx={{ maxWidth: 300 }}>
            <Typography variant="caption" sx={{ display: 'block', mb: 0.5 }}>{catalogText.purpose}</Typography>
            <Typography variant="caption" sx={{ display: 'block', opacity: 0.85 }}>{catalogText.tooltip}</Typography>
        </Box>
    );

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2.5 }}>
            <Typography variant="body2" sx={{
                color: "text.secondary"
            }}>{uiText.panelHeader}</Typography>
            {/* Catalog first — it's the more consequential toggle (it's what makes
                Codex's /model picker list tingly-served models). */}
            <Box>
                <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5, mb: 0.5 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>{catalogText.sectionTitle}</Typography>
                </Box>
                <Divider />
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, py: 1, minHeight: 44 }}>
                    <Box sx={{ flex: '0 0 180px', display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                        <Typography variant="body2" noWrap sx={{
                            fontWeight: 500
                        }}>{catalogText.label}</Typography>
                        <Tooltip placement="top" arrow title={catalogTooltip}>
                            <InfoOutlinedIcon sx={{ fontSize: 14, color: 'text.disabled', cursor: 'help' }} />
                        </Tooltip>
                    </Box>
                    <Box sx={{ flex: '0 0 320px', minWidth: 0 }}>
                        <Box
                            component="span"
                            sx={{
                                px: 0.75, py: 0.25, borderRadius: radius.chip,
                                bgcolor: 'action.hover', fontFamily: fontMono,
                                fontSize: fontSizes.micro, color: 'text.secondary', whiteSpace: 'nowrap',
                            }}
                        >
                            ~/.codex/tingly-model-catalog.json
                        </Box>
                    </Box>
                    <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'flex-end' }}>
                        <Switch
                            size="small"
                            checked={writeCatalog}
                            onChange={(_, c) => setWriteCatalog(c)}
                        />
                    </Box>
                </Box>
            </Box>
            <Box>
                <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5, mb: 0.5 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>{uiText.sectionTitle}</Typography>
                    <Typography variant="caption" sx={{
                        color: "text.secondary"
                    }}>{uiText.sectionHint}</Typography>
                </Box>
                <Divider />
                <Stack divider={<Divider flexItem />}>
                    {FIELD_STRUCT.map((f) => (
                        <FieldRow
                            key={f.key}
                            field={f}
                            text={fieldsText[f.key]}
                            unsetLabel={uiText.unsetLabel}
                            prefs={prefs}
                            setPrefs={setPrefs}
                        />
                    ))}
                </Stack>
            </Box>
        </Box>
    );
};

export default CodexQuickConfig;
