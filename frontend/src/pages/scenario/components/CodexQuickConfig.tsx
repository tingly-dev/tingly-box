import {
    Box,
    Divider,
    MenuItem,
    Select,
    Stack,
    Switch,
    Tooltip,
    Typography,
} from '@mui/material';
import { InfoOutlined as InfoOutlinedIcon } from '@/components/icons';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { type AppLanguage, resolveLanguage } from '@/i18n';

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
    'model_supports_reasoning_summaries',
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
type Kind = 'enum' | 'bool';

// ── Field structure (language-agnostic) ────────────────────────────────
// Keep in sync with codexPrefSpec in internal/server/config/apply_config.go.
// Adding a key: append here AND add an entry in every FIELDS_TEXT_* bundle
// below (TS will flag the missing keys).

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
    model_supports_reasoning_summaries: {
        label: 'Сводки принудительно',
        purpose: 'Включить сводки рассуждений для моделей не от OpenAI',
        tooltip: 'Моделям, проксируемым через tingly-box, эта опция нужна, чтобы возвращать сводки рассуждений. tb включает её по умолчанию.',
    },
};

const FIELDS_TEXT_FA: FieldTextMap = {
    model_reasoning_effort: {
        label: 'شدت استدلال',
        purpose: 'مدل تا چه اندازه عمیق فکر کند',
        tooltip: '‏none/minimal سریع‌ترند؛ high/xhigh بیشتر استدلال می‌کنند اما کندتر و گران‌ترند. خالی یعنی پیش‌فرض Codex (medium).',
    },
    model_reasoning_summary: {
        label: 'خلاصهٔ استدلال',
        purpose: 'آیا و چگونه روند فکر کردن نمایش داده شود',
        tooltip: '‏auto تصمیم را به Codex می‌سپارد؛ concise/detailed میزان جزئیات را تعیین می‌کنند؛ none آن را پنهان می‌کند. پیش‌فرض tb روی auto است.',
    },
    model_verbosity: {
        label: 'میزان توضیح',
        purpose: 'پاسخ چقدر پرگو باشد',
        tooltip: '‏low برای دستیار برنامه‌نویسی کوتاه‌گو مناسب است؛ high توضیح بیشتری می‌دهد. خالی یعنی پیش‌فرض Codex (medium).',
    },
    model_supports_reasoning_summaries: {
        label: 'اجبار خلاصهٔ استدلال',
        purpose: 'فعال کردن اجباری خلاصهٔ استدلال روی مدل‌های غیر OpenAI',
        tooltip: 'مدل‌هایی که از راه tingly-box پراکسی می‌شوند برای برگرداندن خلاصهٔ استدلال به این گزینه نیاز دارند. tb آن را به‌طور پیش‌فرض روشن می‌کند.',
    },
};

const FIELDS_TEXT_AR: FieldTextMap = {
    model_reasoning_effort: {
        label: 'عمق الاستدلال',
        purpose: 'إلى أي مدى يفكِّر النموذج بعمق',
        tooltip: '‏none/minimal الأسرع؛ وhigh/xhigh يستدلان أعمق لكن أبطأ وأغلى. والفراغ يعني القيمة الافتراضية لـ Codex ‏(medium).',
    },
    model_reasoning_summary: {
        label: 'ملخص الاستدلال',
        purpose: 'هل يُعرَض سير التفكير وبأي قدر من التفصيل',
        tooltip: '‏auto يترك القرار لـ Codex؛ وconcise/detailed يحدِّدان التفصيل؛ وnone يُخفيه. والافتراضي في tb هو auto.',
    },
    model_verbosity: {
        label: 'إسهاب الرد',
        purpose: 'مدى إسهاب الرد',
        tooltip: '‏low يناسب مساعد برمجة موجزًا؛ وhigh يقدِّم شرحًا أوفى. والفراغ يعني القيمة الافتراضية لـ Codex ‏(medium).',
    },
    model_supports_reasoning_summaries: {
        label: 'إلزام ملخصات الاستدلال',
        purpose: 'إلزام ملخصات الاستدلال في النماذج غير التابعة لـ OpenAI',
        tooltip: 'تحتاج النماذج المُمرَّرة عبر tingly-box إلى تفعيل هذا الخيار لإعادة ملخصات الاستدلال. ويفعِّله tb افتراضيًا.',
    },
};

const FIELDS_TEXT: Record<AppLanguage, FieldTextMap> = { zh: FIELDS_TEXT_ZH, en: FIELDS_TEXT_EN, ru: FIELDS_TEXT_RU, fa: FIELDS_TEXT_FA, ar: FIELDS_TEXT_AR };

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
    fa: {
        panelHeader: 'این مقادیر در سطح بالای ~/.codex/config.toml و در هر پروفایل tingly نوشته می‌شوند',
        sectionTitle: 'مدل و استدلال',
        sectionHint: 'خالی یعنی مقدار پیش‌فرض درون‌ساخت Codex',
        unsetLabel: '(پیش‌فرض)',
    },
    ar: {
        panelHeader: 'تُكتب هذه القيم في المستوى الأعلى من ~/.codex/config.toml وفي كل ملف tingly',
        sectionTitle: 'النموذج والاستدلال',
        sectionHint: 'الفراغ يعني القيمة الافتراضية المدمجة في Codex',
        unsetLabel: '(افتراضي)',
    },
};

function useLang(): AppLanguage {
    const { i18n } = useTranslation();
    return resolveLanguage(i18n.language);
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
                            <Typography variant="body2" sx={{
                                color: "text.disabled"
                            }}>{unsetLabel}</Typography>
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
    fa: {
        sectionTitle: 'فایل‌ها',
        label: 'فهرست مدل‌ها',
        purpose: 'تا انتخابگر /model در Codex مدل‌های ارائه‌شده توسط tingly را نشان دهد',
        tooltip: 'فایل ~/.codex/tingly-model-catalog.json را می‌نویسد. ‏Codex این فایل را هنگام اجرا می‌خواند و مدل‌های tingly را به انتخابگر /model اضافه می‌کند. اگر خاموش باشد، model_catalog_json در config.toml نوشته نمی‌شود و Codex از فهرست مدل‌های درون‌ساخت خود استفاده می‌کند.',
    },
    ar: {
        sectionTitle: 'الملفات',
        label: 'فهرس النماذج',
        purpose: 'ليُظهِر مُحدِّد /model في Codex النماذج التي يقدِّمها tingly',
        tooltip: 'يكتب الملف ~/.codex/tingly-model-catalog.json. ويقرؤه Codex عند الإقلاع فيضيف نماذج tingly إلى مُحدِّد /model. وعند الإيقاف لا يُكتب model_catalog_json في config.toml ويستخدم Codex قائمة نماذجه المدمجة.',
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
