import {
    Box,
    Divider,
    IconButton,
    MenuItem,
    Select,
    Stack,
    Switch,
    Tooltip,
    Typography,
} from '@mui/material';
import { InfoOutlined as InfoOutlinedIcon, RestartAlt as RestartAltIcon } from '@/components/icons';
import React from 'react';
import { useTranslation } from 'react-i18next';

// CodexPrefs mirrors the Go struct in internal/server/config (CodexPrefs).
// Keys are the literal Codex config.toml keys so the object round-trips
// through the backend without an intermediate mapping layer. All values are
// strings; "" means "omit this key, let Codex use its own default".
export interface CodexPrefs {
    model_reasoning_effort?: string;
    model_reasoning_summary?: string;
    model_verbosity?: string;
    model_supports_reasoning_summaries?: string; // "true" | ""
}

export function defaultCodexPrefs(): CodexPrefs {
    return {};
}

type PrefsKey = keyof CodexPrefs;
type Kind = 'enum' | 'bool';
type Lang = 'zh' | 'en';

// ── Field structure (language-agnostic) ────────────────────────────────
// Keep in sync with codexPrefSpec in internal/server/config/apply_config.go.
// Adding a key: append here AND add entries in FIELDS_TEXT_ZH / FIELDS_TEXT_EN.

interface FieldStruct {
    key: PrefsKey;
    kind: Kind;
    enumValues?: string[]; // first entry is the implicit "empty" sentinel below
}

// Sentinel rendered in the Select for "leave unset (use Codex default)".
const UNSET = '';

const FIELD_STRUCT: FieldStruct[] = [
    { key: 'model_reasoning_effort', kind: 'enum', enumValues: ['none', 'minimal', 'low', 'medium', 'high', 'xhigh'] },
    { key: 'model_reasoning_summary', kind: 'enum', enumValues: ['auto', 'concise', 'detailed', 'none'] },
    { key: 'model_verbosity', kind: 'enum', enumValues: ['low', 'medium', 'high'] },
    { key: 'model_supports_reasoning_summaries', kind: 'bool' },
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
    model_supports_reasoning_summaries: {
        label: '强制推理摘要',
        purpose: '在非 OpenAI 模型上强制开启推理摘要',
        tooltip: '经 tingly-box 转发的模型需要打开此项才能正常返回推理摘要。tb 默认开启。',
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
    model_supports_reasoning_summaries: {
        label: 'Force reasoning summaries',
        purpose: 'Force reasoning summaries on non-OpenAI models',
        tooltip: 'Models proxied through tingly-box need this on to return reasoning summaries. tb enables it by default.',
    },
};

const FIELDS_TEXT: Record<Lang, FieldTextMap> = { zh: FIELDS_TEXT_ZH, en: FIELDS_TEXT_EN };

const UI_TEXT: Record<Lang, { panelHeader: string; resetTooltip: string; sectionTitle: string; sectionHint: string; unsetLabel: string }> = {
    zh: {
        panelHeader: '这些项写入 ~/.codex/config.toml 的顶层与每个 tingly profile',
        resetTooltip: '恢复默认',
        sectionTitle: '模型与推理',
        sectionHint: '留空表示用 Codex 自带默认',
        unsetLabel: '（默认）',
    },
    en: {
        panelHeader: 'These are written to the top level of ~/.codex/config.toml and into each tingly profile',
        resetTooltip: 'Reset to defaults',
        sectionTitle: 'Model & reasoning',
        sectionHint: 'Empty = use Codex built-in default',
        unsetLabel: '(default)',
    },
};

function useLang(): Lang {
    const { i18n } = useTranslation();
    return i18n.language === 'zh' ? 'zh' : 'en';
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

    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, py: 1, minHeight: 44 }}>
            {/* Col 1 — Label + info icon */}
            <Box sx={{ flex: '0 0 180px', display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                <Typography variant="body2" fontWeight={500} noWrap>{text.label}</Typography>
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
                        borderRadius: 0.75,
                        bgcolor: 'action.hover',
                        fontFamily: 'monospace',
                        fontSize: '0.72rem',
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
                {field.kind === 'enum' && (
                    <Select
                        size="small"
                        value={value}
                        displayEmpty
                        onChange={(e) => setValue(e.target.value)}
                        sx={{ minWidth: 160, fontSize: '0.85rem' }}
                    >
                        <MenuItem value={UNSET}>
                            <Typography variant="body2" color="text.disabled">{unsetLabel}</Typography>
                        </MenuItem>
                        {field.enumValues!.map((v) => (
                            <MenuItem key={v} value={v} sx={{ fontFamily: 'monospace', fontSize: '0.85rem' }}>{v}</MenuItem>
                        ))}
                    </Select>
                )}
            </Box>
        </Box>
    );
};

// ── Catalog section text ───────────────────────────────────────────────

const CATALOG_TEXT: Record<Lang, { sectionTitle: string; label: string; purpose: string; tooltip: string }> = {
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
};

// ── Panel ──────────────────────────────────────────────────────────────

interface CodexQuickConfigProps {
    prefs: CodexPrefs;
    setPrefs: (p: CodexPrefs) => void;
    onResetDefaults: () => void;
    writeCatalog: boolean;
    setWriteCatalog: (v: boolean) => void;
}

const CodexQuickConfig: React.FC<CodexQuickConfigProps> = ({ prefs, setPrefs, onResetDefaults, writeCatalog, setWriteCatalog }) => {
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
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="body2" color="text.secondary">{uiText.panelHeader}</Typography>
                <Tooltip title={uiText.resetTooltip} arrow>
                    <IconButton size="small" onClick={onResetDefaults}>
                        <RestartAltIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
            </Box>

            <Box>
                <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5, mb: 0.5 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>{uiText.sectionTitle}</Typography>
                    <Typography variant="caption" color="text.secondary">{uiText.sectionHint}</Typography>
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

            <Box>
                <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5, mb: 0.5 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>{catalogText.sectionTitle}</Typography>
                </Box>
                <Divider />
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, py: 1, minHeight: 44 }}>
                    <Box sx={{ flex: '0 0 180px', display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                        <Typography variant="body2" fontWeight={500} noWrap>{catalogText.label}</Typography>
                        <Tooltip placement="top" arrow title={catalogTooltip}>
                            <InfoOutlinedIcon sx={{ fontSize: 14, color: 'text.disabled', cursor: 'help' }} />
                        </Tooltip>
                    </Box>
                    <Box sx={{ flex: '0 0 320px', minWidth: 0 }}>
                        <Box
                            component="span"
                            sx={{
                                px: 0.75, py: 0.25, borderRadius: 0.75,
                                bgcolor: 'action.hover', fontFamily: 'monospace',
                                fontSize: '0.72rem', color: 'text.secondary', whiteSpace: 'nowrap',
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
        </Box>
    );
};

export default CodexQuickConfig;
