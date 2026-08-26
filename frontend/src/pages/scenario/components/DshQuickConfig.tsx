import {
    Box,
    Divider,
    MenuItem,
    Select,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import { InfoOutlined as InfoOutlinedIcon } from '@/components/icons';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { type AppLanguage, resolveLanguage } from '@/i18n';

// DshPrefs mirrors the Go struct in internal/server/config (DshPrefs). Keys
// are the literal settings.yaml provider-stanza keys so the object
// round-trips through the backend without an intermediate mapping layer.
// All values are strings; "" means "omit this key, dsh treats the provider
// as text-only" — except `protocol`, which dsh always requires, so an empty
// value there just defers to the backend's default (openai-completions).
export interface DshPrefs {
    default_input?: string; // "text" | "text_image" | ""
    protocol?: string; // "openai-completions" | "openai-responses" | "anthropic-messages" | ""
}

// PROTOCOL_VALUES lists the wire protocols dsh's llm-pi-ai adapter supports
// for a custom provider (see dshProtocolValues in Go's apply_config_dsh.go).
export const PROTOCOL_VALUES = ['openai-completions', 'openai-responses', 'anthropic-messages'] as const;

export function defaultDshPrefs(): DshPrefs {
    // Conservative default for defaultInput: no key written, so dsh treats
    // the provider as text-only until the user opts a model into vision.
    // protocol defaults to openai-completions, a concrete pre-selected value
    // (there is no meaningful "unset" state for a required field) — keep
    // both in sync with Go's DefaultDshPrefs.
    return { protocol: 'openai-completions' };
}

// DSH_PREF_KEYS is the single source of truth for the durable dsh pref keys
// on the frontend (mirrors CODEX_PREF_KEYS in CodexQuickConfig.tsx and the
// backend DshPrefs struct).
const DSH_PREF_KEYS = ['default_input', 'protocol'] as const satisfies readonly (keyof DshPrefs)[];

// Merge a previously-applied prefs object over the current defaults so
// reopening the dsh config modal restores durable user choices rather than
// resetting to defaults every time.
export function mergeSavedDshPrefs(applied: DshPrefs = {}): DshPrefs {
    const merged: DshPrefs = { ...defaultDshPrefs() };
    for (const key of DSH_PREF_KEYS) {
        const v = applied[key];
        if (v !== undefined && v !== '') {
            merged[key] = v;
        }
    }
    return merged;
}


const UNSET = '';
const DEFAULT_INPUT_VALUES = ['text', 'text_image'];

interface FieldText {
    label: string;
    purpose: string;
    tooltip: string;
}

const FIELD_TEXT: Record<AppLanguage, FieldText> = {
    zh: {
        label: '支持的输入模态',
        purpose: '控制该 provider 下的模型默认能否接收图片',
        tooltip: 'text 仅文本；text_image 允许图片输入。留空表示不写入 defaultInput，dsh 按文本模型处理。',
    },
    en: {
        label: 'Supported input modality',
        purpose: 'Whether models under this provider accept image input by default',
        tooltip: 'text is text-only; text_image also accepts images. Empty = omit defaultInput, dsh treats models as text-only.',
    },
    ru: {
        label: 'Поддерживаемые модальности ввода',
        purpose: 'Принимают ли модели этого провайдера изображения по умолчанию',
        tooltip: 'text — только текст; text_image допускает и изображения. Пусто — defaultInput не записывается, и dsh считает модели текстовыми.',
    },
    fa: {
        label: 'حالت‌های ورودی پشتیبانی‌شده',
        purpose: 'آیا مدل‌های این ارائه‌دهنده به‌طور پیش‌فرض تصویر می‌پذیرند',
        tooltip: '‏text فقط متن است؛ text_image تصویر را هم می‌پذیرد. خالی یعنی defaultInput نوشته نمی‌شود و dsh مدل‌ها را متنی در نظر می‌گیرد.',
    },
    ar: {
        label: 'أنماط الإدخال المدعومة',
        purpose: 'هل تقبل نماذج هذا المزوِّد الصور افتراضيًا',
        tooltip: '‏text نص فقط؛ وtext_image يقبل الصور أيضًا. والفراغ يعني عدم كتابة defaultInput، فيعامل dsh النماذج كنصية.',
    },
};

const VALUE_LABEL: Record<AppLanguage, Record<string, string>> = {
    zh: { text: 'text（仅文本）', text_image: 'text_image（文本 + 图片）' },
    en: { text: 'text (text-only)', text_image: 'text_image (text + image)' },
    ru: { text: 'text (только текст)', text_image: 'text_image (текст + изображения)' },
    fa: { text: 'text (فقط متن)', text_image: 'text_image (متن + تصویر)' },
    ar: { text: 'text (نص فقط)', text_image: 'text_image (نص + صور)' },
};

const UI_TEXT: Record<AppLanguage, { panelHeader: string; sectionTitle: string; sectionHint: string; unsetLabel: string }> = {
    zh: {
        panelHeader: '这些项写入 $DSH_HOME/settings.yaml 的 tingly-box provider 条目',
        sectionTitle: '模型能力',
        sectionHint: '留空表示用 dsh 默认（仅文本）',
        unsetLabel: '（默认，仅文本）',
    },
    en: {
        panelHeader: 'Written into the tingly-box provider entry in $DSH_HOME/settings.yaml',
        sectionTitle: 'Model capabilities',
        sectionHint: 'Empty = dsh default (text-only)',
        unsetLabel: '(default, text-only)',
    },
    ru: {
        panelHeader: 'Эти значения записываются в запись провайдера tingly-box в $DSH_HOME/settings.yaml',
        sectionTitle: 'Возможности моделей',
        sectionHint: 'Пусто — значение dsh по умолчанию (только текст)',
        unsetLabel: '(по умолчанию, только текст)',
    },
    fa: {
        panelHeader: 'این مقادیر در ورودی ارائه‌دهندهٔ tingly-box در $DSH_HOME/settings.yaml نوشته می‌شوند',
        sectionTitle: 'قابلیت‌های مدل',
        sectionHint: 'خالی یعنی مقدار پیش‌فرض dsh (فقط متن)',
        unsetLabel: '(پیش‌فرض، فقط متن)',
    },
    ar: {
        panelHeader: 'تُكتب هذه القيم في مدخل مزوِّد tingly-box داخل $DSH_HOME/settings.yaml',
        sectionTitle: 'قدرات النماذج',
        sectionHint: 'الفراغ يعني القيمة الافتراضية لـ dsh (نص فقط)',
        unsetLabel: '(افتراضي، نص فقط)',
    },
};

// Protocol has no meaningful "unset" state — dsh always requires an `api`
// value, so unlike defaultInput's UNSET sentinel, every option here writes a
// concrete value and one is always pre-selected (see defaultDshPrefs).
const PROTOCOL_TEXT: Record<AppLanguage, { sectionTitle: string; label: string; purpose: string; tooltip: string }> = {
    zh: {
        sectionTitle: '连接',
        label: '主协议',
        purpose: '控制 tingly-box 转发给 dsh 时使用的接口格式',
        tooltip: 'OpenAI Chat 是最通用的兼容格式；OpenAI Responses 用于走 Responses API 的模型；Anthropic Messages 用于走 Claude 消息格式的模型。选错会导致该 provider 下的模型无法正常工作。',
    },
    en: {
        sectionTitle: 'Connection',
        label: 'Primary protocol',
        purpose: 'Which wire format tingly-box speaks to dsh with',
        tooltip: 'OpenAI Chat is the most widely compatible format; OpenAI Responses is for models that use the Responses API; Anthropic Messages is for models that speak the Claude message format. Picking the wrong one breaks models under this provider.',
    },
    ru: {
        sectionTitle: 'Подключение',
        label: 'Основной протокол',
        purpose: 'В каком формате tingly-box передаёт запросы в dsh',
        tooltip: 'OpenAI Chat — самый совместимый формат; OpenAI Responses — для моделей, работающих через Responses API; Anthropic Messages — для моделей, использующих формат сообщений Claude. Неверный выбор сломает модели этого провайдера.',
    },
    fa: {
        sectionTitle: 'اتصال',
        label: 'پروتکل اصلی',
        purpose: 'قالبی که tingly-box با آن درخواست‌ها را به dsh می‌فرستد',
        tooltip: '‏OpenAI Chat سازگارترین قالب است؛ OpenAI Responses برای مدل‌هایی است که از Responses API استفاده می‌کنند؛ Anthropic Messages برای مدل‌هایی است که قالب پیام Claude را می‌فهمند. انتخاب نادرست، مدل‌های این ارائه‌دهنده را از کار می‌اندازد.',
    },
    ar: {
        sectionTitle: 'الاتصال',
        label: 'البروتوكول الأساسي',
        purpose: 'الصيغة التي يمرِّر بها tingly-box الطلبات إلى dsh',
        tooltip: '‏OpenAI Chat هو الأوسع توافقًا؛ وOpenAI Responses للنماذج التي تستخدم Responses API؛ وAnthropic Messages للنماذج التي تتحدث صيغة رسائل Claude. والاختيار الخاطئ يُعطِّل نماذج هذا المزوِّد.',
    },
};

const PROTOCOL_VALUE_LABEL: Record<AppLanguage, Record<string, string>> = {
    zh: {
        'openai-completions': 'OpenAI Chat（Completions）',
        'openai-responses': 'OpenAI Responses',
        'anthropic-messages': 'Anthropic Messages',
    },
    en: {
        'openai-completions': 'OpenAI Chat (Completions)',
        'openai-responses': 'OpenAI Responses',
        'anthropic-messages': 'Anthropic Messages',
    },
    ru: {
        'openai-completions': 'OpenAI Chat (Completions)',
        'openai-responses': 'OpenAI Responses',
        'anthropic-messages': 'Anthropic Messages',
    },
    fa: {
        'openai-completions': 'OpenAI Chat (Completions)',
        'openai-responses': 'OpenAI Responses',
        'anthropic-messages': 'Anthropic Messages',
    },
    ar: {
        'openai-completions': 'OpenAI Chat (Completions)',
        'openai-responses': 'OpenAI Responses',
        'anthropic-messages': 'Anthropic Messages',
    },
};

function useLang(): AppLanguage {
    const { i18n } = useTranslation();
    return resolveLanguage(i18n.language);
}

interface DshQuickConfigProps {
    prefs: DshPrefs;
    setPrefs: (p: DshPrefs) => void;
}

const DshQuickConfig: React.FC<DshQuickConfigProps> = ({ prefs, setPrefs }) => {
    const lang = useLang();
    const uiText = UI_TEXT[lang];
    const text = FIELD_TEXT[lang];
    const valueLabel = VALUE_LABEL[lang];
    const protocolText = PROTOCOL_TEXT[lang];
    const protocolValueLabel = PROTOCOL_VALUE_LABEL[lang];

    const value = prefs.default_input ?? '';
    const setValue = (next: string) => setPrefs({ ...prefs, default_input: next });

    const protocolValue = prefs.protocol || PROTOCOL_VALUES[0];
    const setProtocolValue = (next: string) => setPrefs({ ...prefs, protocol: next });

    const richTooltip = (
        <Box sx={{ maxWidth: 280 }}>
            <Typography variant="caption" sx={{ display: 'block', mb: 0.5 }}>{text.purpose}</Typography>
            <Typography variant="caption" sx={{ display: 'block', opacity: 0.85 }}>{text.tooltip}</Typography>
        </Box>
    );

    const protocolTooltip = (
        <Box sx={{ maxWidth: 280 }}>
            <Typography variant="caption" sx={{ display: 'block', mb: 0.5 }}>{protocolText.purpose}</Typography>
            <Typography variant="caption" sx={{ display: 'block', opacity: 0.85 }}>{protocolText.tooltip}</Typography>
        </Box>
    );

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2.5 }}>
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>{uiText.panelHeader}</Typography>
            <Box>
                <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5, mb: 0.5 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>{protocolText.sectionTitle}</Typography>
                </Box>
                <Divider />
                <Stack>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, py: 1, minHeight: 44 }}>
                        <Box sx={{ flex: '0 0 180px', display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                            <Typography variant="body2" noWrap sx={{ fontWeight: 500 }}>{protocolText.label}</Typography>
                            <Tooltip placement="top" arrow title={protocolTooltip}>
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
                                api
                            </Box>
                        </Box>
                        <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'flex-end' }}>
                            <Select
                                size="small"
                                value={protocolValue}
                                onChange={(e) => setProtocolValue(e.target.value)}
                                sx={{ minWidth: 220, fontSize: '0.85rem' }}
                            >
                                {PROTOCOL_VALUES.map((v) => (
                                    <MenuItem key={v} value={v} sx={{ fontSize: '0.85rem' }}>{protocolValueLabel[v]}</MenuItem>
                                ))}
                            </Select>
                        </Box>
                    </Box>
                </Stack>
            </Box>
            <Box>
                <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5, mb: 0.5 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>{uiText.sectionTitle}</Typography>
                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>{uiText.sectionHint}</Typography>
                </Box>
                <Divider />
                <Stack>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, py: 1, minHeight: 44 }}>
                        <Box sx={{ flex: '0 0 180px', display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                            <Typography variant="body2" noWrap sx={{ fontWeight: 500 }}>{text.label}</Typography>
                            <Tooltip placement="top" arrow title={richTooltip}>
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
                                defaultInput
                            </Box>
                        </Box>
                        <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'flex-end' }}>
                            <Select
                                size="small"
                                value={value}
                                displayEmpty
                                onChange={(e) => setValue(e.target.value)}
                                sx={{ minWidth: 220, fontSize: '0.85rem' }}
                            >
                                <MenuItem value={UNSET}>
                                    <Typography variant="body2" sx={{ color: 'text.disabled' }}>{uiText.unsetLabel}</Typography>
                                </MenuItem>
                                {DEFAULT_INPUT_VALUES.map((v) => (
                                    <MenuItem key={v} value={v} sx={{ fontSize: '0.85rem' }}>{valueLabel[v]}</MenuItem>
                                ))}
                            </Select>
                        </Box>
                    </Box>
                </Stack>
            </Box>
        </Box>
    );
};

export default DshQuickConfig;
