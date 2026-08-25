import {
    Alert,
    Box,
    Button,
    ButtonBase,
    CircularProgress,
    Collapse,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Divider,
    FormControl,
    IconButton,
    InputAdornment,
    MenuItem,
    Select,
    Stack,
    Switch,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { resolveLanguage } from '@/i18n';
import UnifiedCard from '@/components/UnifiedCard';
import { Add, Close, InfoOutlined, RestartAlt, Search } from '@/components/icons';
import { api } from '@/services/api';
import {
    CLAUDE_CODE_DEFAULT_MODE_OPTIONS,
    CLAUDE_CODE_DEFAULT_MODE_TEXT,
    CLAUDE_CODE_FIELDS_TEXT,
    CLAUDE_CODE_FIELD_STRUCT,
    CLAUDE_CONFIG_KEY_SX,
    type ClaudeCodeDefaultMode,
    type ClaudeCodePrefs,
    type FieldStruct,
    type PrefsKey,
} from './ClaudeCodeQuickConfig';

type OverrideKey = PrefsKey | 'defaultMode';

interface ClaudeCodeProfileOverridesProps {
    profileId: string;
    profileName?: string;
    onArtifactChange?: (artifact: ClaudeCodeProfileSettingsArtifact) => void;
}

export interface ClaudeCodeProfileSettingsArtifact {
    settingsPath: string;
    settingsExists: boolean;
}

const TEXT = {
    zh: {
        title: 'Profile 覆盖',
        inherited: '当前完全继承主配置和模型路由',
        pendingInheritance: '覆盖已移除；保存 Profile 后恢复继承',
        hint: '未列出的参数继续继承；启动时自动生成运行配置。',
        add: '添加',
        closeAdd: '关闭',
        search: '搜索可覆盖的运行参数',
        noSettings: '没有可添加的参数',
        common: '常用',
        runtime: '运行参数',
        limits: '限制与工具',
        behavior: '行为与隐私',
        network: '网络',
        permissionMode: '默认权限模式',
        permissionPurpose: '这个 Profile 启动 Claude Code 时使用的权限模式',
        remove: '移除覆盖并恢复继承',
        save: '保存',
        saved: 'Profile 覆盖已保存',
        restoreAll: '全部恢复继承',
        restoreTitle: '清除全部 Profile 覆盖？',
        restoreBody: '这只会清除当前 Profile 的运行参数覆盖；主配置、模型路由和 Profile 本身不会变化。',
        restored: '已恢复继承主配置和模型路由',
        cancel: '取消',
        confirmRestore: '恢复继承',
        loadFailed: '读取 Profile 覆盖失败',
        saveFailed: '保存 Profile 覆盖失败',
    },
    en: {
        title: 'Profile Overrides',
        inherited: 'Fully inherits the main configuration and model routing',
        pendingInheritance: 'Overrides removed; save the profile to restore inheritance',
        hint: 'Unlisted settings keep inheriting; runtime settings are generated automatically at launch.',
        add: 'Add',
        closeAdd: 'Close',
        search: 'Search runtime settings',
        noSettings: 'No settings available',
        common: 'Common',
        runtime: 'Runtime',
        limits: 'Limits & tools',
        behavior: 'Behavior & privacy',
        network: 'Network',
        permissionMode: 'Default permission mode',
        permissionPurpose: 'Permission mode used when this profile starts Claude Code',
        remove: 'Remove override and restore inheritance',
        save: 'Save',
        saved: 'Profile overrides saved',
        restoreAll: 'Restore all inheritance',
        restoreTitle: 'Clear all profile overrides?',
        restoreBody: 'This only clears runtime overrides for this profile. The main configuration, model routing, and profile remain unchanged.',
        restored: 'Main configuration and model routing inheritance restored',
        cancel: 'Cancel',
        confirmRestore: 'Restore inheritance',
        loadFailed: 'Failed to load profile overrides',
        saveFailed: 'Failed to save profile overrides',
    },
    ru: {
        title: 'Переопределения профиля',
        inherited: 'Полностью наследует основную конфигурацию и маршрутизацию моделей',
        pendingInheritance: 'Переопределения удалены; сохраните профиль, чтобы вернуть наследование',
        hint: 'Неуказанные параметры продолжают наследоваться; рабочие настройки создаются автоматически при запуске.',
        add: 'Добавить',
        closeAdd: 'Закрыть',
        search: 'Поиск рабочих параметров',
        noSettings: 'Доступных параметров нет',
        common: 'Основные',
        runtime: 'Рабочие параметры',
        limits: 'Ограничения и инструменты',
        behavior: 'Поведение и приватность',
        network: 'Сеть',
        permissionMode: 'Режим прав по умолчанию',
        permissionPurpose: 'Режим прав, с которым этот профиль запускает Claude Code',
        remove: 'Убрать переопределение и вернуть наследование',
        save: 'Сохранить',
        saved: 'Переопределения профиля сохранены',
        restoreAll: 'Вернуть всё наследование',
        restoreTitle: 'Очистить все переопределения профиля?',
        restoreBody: 'Будут очищены только переопределения рабочих параметров этого профиля. Основная конфигурация, маршрутизация моделей и сам профиль не изменятся.',
        restored: 'Наследование основной конфигурации и маршрутизации моделей восстановлено',
        cancel: 'Отмена',
        confirmRestore: 'Вернуть наследование',
        loadFailed: 'Не удалось загрузить переопределения профиля',
        saveFailed: 'Не удалось сохранить переопределения профиля',
    },
    fa: {
        title: 'بازنویسی‌های پروفایل',
        inherited: 'به‌طور کامل از پیکربندی اصلی و مسیریابی مدل‌ها ارث می‌برد',
        pendingInheritance: 'بازنویسی‌ها برداشته شد؛ برای بازگشت ارث‌بری پروفایل را ذخیره کنید',
        hint: 'پارامترهای فهرست‌نشده همچنان ارث می‌برند؛ تنظیمات اجرایی هنگام اجرا خودکار ساخته می‌شوند.',
        add: 'افزودن',
        closeAdd: 'بستن',
        search: 'جست‌وجوی پارامترهای اجرایی',
        noSettings: 'پارامتری برای افزودن نیست',
        common: 'پرکاربرد',
        runtime: 'پارامترهای اجرایی',
        limits: 'محدودیت‌ها و ابزارها',
        behavior: 'رفتار و حریم خصوصی',
        network: 'شبکه',
        permissionMode: 'حالت دسترسی پیش‌فرض',
        permissionPurpose: 'حالت دسترسی‌ای که این پروفایل هنگام اجرای Claude Code به کار می‌برد',
        remove: 'برداشتن بازنویسی و بازگشت به ارث‌بری',
        save: 'ذخیره',
        saved: 'بازنویسی‌های پروفایل ذخیره شد',
        restoreAll: 'بازگرداندن همهٔ ارث‌بری‌ها',
        restoreTitle: 'همهٔ بازنویسی‌های پروفایل پاک شود؟',
        restoreBody: 'تنها بازنویسی پارامترهای اجرایی این پروفایل پاک می‌شود. پیکربندی اصلی، مسیریابی مدل‌ها و خودِ پروفایل تغییر نمی‌کنند.',
        restored: 'ارث‌بری از پیکربندی اصلی و مسیریابی مدل‌ها بازگردانده شد',
        cancel: 'انصراف',
        confirmRestore: 'بازگرداندن ارث‌بری',
        loadFailed: 'بارگذاری بازنویسی‌های پروفایل ناموفق بود',
        saveFailed: 'ذخیرهٔ بازنویسی‌های پروفایل ناموفق بود',
    },
    ar: {
        title: 'تجاوزات الملف',
        inherited: 'يرث بالكامل التهيئة الرئيسية وتوجيه النماذج',
        pendingInheritance: 'أُزيلت التجاوزات؛ احفظ الملف لاستعادة الوراثة',
        hint: 'تظل الإعدادات غير المدرَجة موروثة؛ وتُولَّد إعدادات التشغيل تلقائيًا عند الإقلاع.',
        add: 'إضافة',
        closeAdd: 'إغلاق',
        search: 'ابحث في إعدادات التشغيل',
        noSettings: 'لا توجد إعدادات متاحة',
        common: 'شائعة',
        runtime: 'إعدادات التشغيل',
        limits: 'الحدود والأدوات',
        behavior: 'السلوك والخصوصية',
        network: 'الشبكة',
        permissionMode: 'وضع الأذونات الافتراضي',
        permissionPurpose: 'وضع الأذونات الذي يستخدمه هذا الملف عند تشغيل Claude Code',
        remove: 'إزالة التجاوز واستعادة الوراثة',
        save: 'حفظ',
        saved: 'حُفظت تجاوزات الملف',
        restoreAll: 'استعادة كل الوراثة',
        restoreTitle: 'مسح كل تجاوزات الملف؟',
        restoreBody: 'يمسح هذا تجاوزات إعدادات التشغيل لهذا الملف فقط. أما التهيئة الرئيسية وتوجيه النماذج والملف نفسه فتبقى دون تغيير.',
        restored: 'استُعيدت وراثة التهيئة الرئيسية وتوجيه النماذج',
        cancel: 'إلغاء',
        confirmRestore: 'استعادة الوراثة',
        loadFailed: 'تعذَّر تحميل تجاوزات الملف',
        saveFailed: 'تعذَّر حفظ تجاوزات الملف',
    },
} as const;

const COMMON_KEYS: OverrideKey[] = ['CLAUDE_CODE_MAX_OUTPUT_TOKENS', 'defaultMode'];
const PROFILE_OVERRIDE_FIELDS = CLAUDE_CODE_FIELD_STRUCT.filter(field => field.kind !== 'model');
const FIELD_ORDER: OverrideKey[] = [
    ...COMMON_KEYS,
    ...PROFILE_OVERRIDE_FIELDS
        .map(field => field.envName)
        .filter(key => !COMMON_KEYS.includes(key)),
];
const FIELD_BY_KEY = new Map<PrefsKey, FieldStruct>(
    CLAUDE_CODE_FIELD_STRUCT.map(field => [field.envName, field]),
);

const prefsEqual = (left: ClaudeCodePrefs, right: ClaudeCodePrefs): boolean => {
    const keys = new Set([...Object.keys(left), ...Object.keys(right)]);
    return [...keys].every(key => left[key as PrefsKey] === right[key as PrefsKey]);
};

const deriveOverrideKeys = (
    basePrefs: ClaudeCodePrefs,
    effectivePrefs: ClaudeCodePrefs,
    inheritedMode: ClaudeCodeDefaultMode,
    effectiveMode: ClaudeCodeDefaultMode,
): Set<OverrideKey> => {
    const keys = new Set<OverrideKey>();
    for (const field of PROFILE_OVERRIDE_FIELDS) {
        if ((basePrefs[field.envName] ?? '') !== (effectivePrefs[field.envName] ?? '')) {
            keys.add(field.envName);
        }
    }
    if (inheritedMode !== effectiveMode) keys.add('defaultMode');
    return keys;
};

const ClaudeCodeProfileOverrides: React.FC<ClaudeCodeProfileOverridesProps> = ({
    profileId,
    profileName,
    onArtifactChange,
}) => {
    const { i18n } = useTranslation();
    const lang = resolveLanguage(i18n.language);
    const text = TEXT[lang];
    const fieldText = CLAUDE_CODE_FIELDS_TEXT[lang];
    const modeText = CLAUDE_CODE_DEFAULT_MODE_TEXT[lang];

    const [basePrefs, setBasePrefs] = React.useState<ClaudeCodePrefs>({});
    const [prefs, setPrefs] = React.useState<ClaudeCodePrefs>({});
    const [loadedPrefs, setLoadedPrefs] = React.useState<ClaudeCodePrefs>({});
    const [inheritedMode, setInheritedMode] = React.useState<ClaudeCodeDefaultMode>('acceptEdits');
    const [defaultMode, setDefaultMode] = React.useState<ClaudeCodeDefaultMode>('acceptEdits');
    const [loadedMode, setLoadedMode] = React.useState<ClaudeCodeDefaultMode>('acceptEdits');
    const [selectedKeys, setSelectedKeys] = React.useState<Set<OverrideKey>>(new Set());
    const [hasOverrides, setHasOverrides] = React.useState(false);
    const [loading, setLoading] = React.useState(true);
    const [savingAction, setSavingAction] = React.useState<'save' | 'restore' | null>(null);
    const [loadError, setLoadError] = React.useState(false);
    const [message, setMessage] = React.useState<{ severity: 'success' | 'error'; text: string } | null>(null);
    const [addOpen, setAddOpen] = React.useState(false);
    const [addQuery, setAddQuery] = React.useState('');
    const [restoreOpen, setRestoreOpen] = React.useState(false);

    const applyResponse = React.useCallback((result: any): boolean => {
        if (!result?.success || !result.data) return false;
        const nextBase = (result.data.basePreferences || {}) as ClaudeCodePrefs;
        const nextPrefs = (result.data.preferences || {}) as ClaudeCodePrefs;
        const nextInheritedMode = (result.data.inheritedDefaultMode || 'acceptEdits') as ClaudeCodeDefaultMode;
        const nextMode = (result.data.defaultMode || nextInheritedMode) as ClaudeCodeDefaultMode;
        setBasePrefs(nextBase);
        setPrefs(nextPrefs);
        setLoadedPrefs(nextPrefs);
        setInheritedMode(nextInheritedMode);
        setDefaultMode(nextMode);
        setLoadedMode(nextMode);
        setSelectedKeys(deriveOverrideKeys(nextBase, nextPrefs, nextInheritedMode, nextMode));
        setHasOverrides(!!result.data.hasOverrides);
        onArtifactChange?.({
            settingsPath: result.data.settingsPath || '',
            settingsExists: !!result.data.settingsExists,
        });
        setLoadError(false);
        return true;
    }, [onArtifactChange]);

    React.useEffect(() => {
        if (!profileId) return;
        let active = true;
        setLoading(true);
        setLoadError(false);
        setMessage(null);
        void api.getClaudeCodeProfileConfig('claude_code', profileId).then(result => {
            if (!active) return;
            if (!applyResponse(result)) {
                setLoadError(true);
                setMessage({ severity: 'error', text: result?.error || text.loadFailed });
            }
        }).finally(() => {
            if (active) setLoading(false);
        });
        return () => {
            active = false;
        };
    }, [applyResponse, profileId, profileName, text.loadFailed]);

    const orderedSelectedKeys = React.useMemo(
        () => FIELD_ORDER.filter(key => selectedKeys.has(key)),
        [selectedKeys],
    );
    const availableCommon = COMMON_KEYS.filter(key => !selectedKeys.has(key));
    const availableMore = FIELD_ORDER.filter(key => !COMMON_KEYS.includes(key) && !selectedKeys.has(key));
    const isDirty = !prefsEqual(prefs, loadedPrefs) || defaultMode !== loadedMode;
    const saving = savingAction !== null;

    const addOverride = (key: OverrideKey) => {
        setSelectedKeys(current => new Set(current).add(key));
        setAddOpen(false);
        setAddQuery('');
        setMessage(null);
    };

    const removeOverride = (key: OverrideKey) => {
        setSelectedKeys(current => {
            const next = new Set(current);
            next.delete(key);
            return next;
        });
        if (key === 'defaultMode') {
            setDefaultMode(inheritedMode);
        } else {
            setPrefs(current => {
                const next = { ...current };
                const inheritedValue = basePrefs[key];
                if (inheritedValue) next[key] = inheritedValue;
                else delete next[key];
                return next;
            });
        }
        setMessage(null);
    };

    const updatePreference = (key: PrefsKey, value: string) => {
        setPrefs(current => ({ ...current, [key]: value }));
        setMessage(null);
    };

    const runMutation = async (
        action: 'save' | 'restore',
        request: () => Promise<any>,
        successMessage: string,
    ) => {
        setSavingAction(action);
        setMessage(null);
        try {
            const result = await request();
            if (applyResponse(result)) {
                setMessage({ severity: 'success', text: successMessage });
            } else {
                setMessage({ severity: 'error', text: result?.error || text.saveFailed });
            }
        } finally {
            setSavingAction(null);
        }
    };

    const handleSave = () => runMutation(
        'save',
        () => api.updateClaudeCodeProfileConfig(
            'claude_code',
            profileId,
            prefs as Record<string, string>,
            defaultMode,
        ),
        text.saved,
    );

    const handleRestoreAll = () => {
        setRestoreOpen(false);
        return runMutation(
            'restore',
            () => api.resetClaudeCodeProfileConfig('claude_code', profileId),
            text.restored,
        );
    };

    const renderControl = (key: OverrideKey) => {
        if (key === 'defaultMode') {
            return (
                <FormControl size="small" sx={{ width: 360, maxWidth: '100%' }}>
                    <Select
                        value={defaultMode}
                        onChange={event => {
                            setDefaultMode(event.target.value as ClaudeCodeDefaultMode);
                            setMessage(null);
                        }}
                    >
                        {CLAUDE_CODE_DEFAULT_MODE_OPTIONS.map(mode => (
                            <MenuItem key={mode} value={mode}>
                                <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2, width: '100%' }}>
                                    <Typography variant="body2">{modeText[mode].label}</Typography>
                                    <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>{mode}</Typography>
                                </Box>
                            </MenuItem>
                        ))}
                    </Select>
                </FormControl>
            );
        }

        const field = FIELD_BY_KEY.get(key);
        if (!field) return null;
        const value = prefs[key] ?? '';
        if (field.kind === 'bool') {
            return (
                <Switch
                    size="small"
                    checked={value === '1'}
                    onChange={(_, checked) => updatePreference(key, checked ? '1' : '')}
                />
            );
        }
        return (
            <TextField
                size="small"
                value={value}
                onChange={event => updatePreference(key, event.target.value)}
                placeholder={fieldText[key].placeholder}
                sx={{ width: field.kind === 'model' ? 280 : field.kind === 'text' ? 320 : 180, maxWidth: '100%' }}
                slotProps={{
                    input: {
                        inputProps: field.kind === 'int' ? { inputMode: 'numeric' } : undefined,
                        endAdornment: field.unit
                            ? <InputAdornment position="end"><Typography variant="caption" color="text.disabled">{field.unit}</Typography></InputAdornment>
                            : undefined,
                        sx: { fontFamily: field.kind === 'model' ? 'monospace' : undefined, fontSize: '0.85rem' },
                    },
                }}
            />
        );
    };

    const fieldLabel = (key: OverrideKey) => key === 'defaultMode' ? text.permissionMode : fieldText[key].label;
    const fieldPurpose = (key: OverrideKey) => key === 'defaultMode' ? text.permissionPurpose : fieldText[key].purpose;
    const fieldGroup = (key: OverrideKey) => {
        if (COMMON_KEYS.includes(key)) return text.common;
        const group = key === 'defaultMode' ? 'behavior' : FIELD_BY_KEY.get(key)?.group;
        if (group === 'limits') return text.limits;
        if (group === 'switches') return text.behavior;
        if (group === 'network') return text.network;
        return text.runtime;
    };
    const availableKeys = [...availableCommon, ...availableMore];
    const normalizedAddQuery = addQuery.trim().toLocaleLowerCase();
    const filteredAvailableKeys = normalizedAddQuery
        ? availableKeys.filter(key => `${fieldLabel(key)} ${fieldPurpose(key)} ${key}`.toLocaleLowerCase().includes(normalizedAddQuery))
        : availableKeys;
    const availableGroups = [...new Set(filteredAvailableKeys.map(fieldGroup))]
        .map(group => ({ group, keys: filteredAvailableKeys.filter(key => fieldGroup(key) === group) }));

    return (
        <UnifiedCard
            size="full"
            title={text.title}
            subtitle={text.hint}
            titleMarginBottom={loading || orderedSelectedKeys.length > 0 ? 2 : 0.5}
            sx={{
                '& > .MuiCardContent-root': {
                    p: 2.5,
                    '&:last-child': { pb: 2.5 },
                },
            }}
            rightAction={(
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                    {!loading && !loadError && (hasOverrides || isDirty) && (
                        <Tooltip title={text.restoreAll} arrow>
                            <span>
                                <IconButton
                                    size="small"
                                    aria-label={text.restoreAll}
                                    onClick={() => setRestoreOpen(true)}
                                    disabled={saving}
                                >
                                    {savingAction === 'restore'
                                        ? <CircularProgress size={16} color="inherit" />
                                        : <RestartAlt fontSize="small" />}
                                </IconButton>
                            </span>
                        </Tooltip>
                    )}
                    <Button
                        size="small"
                        variant="outlined"
                        startIcon={addOpen ? <Close fontSize="small" /> : <Add fontSize="small" />}
                        onClick={() => {
                            setAddOpen(current => !current);
                            setAddQuery('');
                        }}
                        disabled={loading || saving || loadError || availableKeys.length === 0}
                    >
                        {addOpen ? text.closeAdd : text.add}
                    </Button>
                    {isDirty && (
                        <Button size="small" variant="contained" onClick={handleSave} disabled={saving}>
                            {savingAction === 'save' ? <CircularProgress size={15} color="inherit" /> : text.save}
                        </Button>
                    )}
                </Box>
            )}
        >
            {loading ? (
                <Box sx={{ display: 'flex', justifyContent: 'center', py: 1 }}><CircularProgress size={20} /></Box>
            ) : orderedSelectedKeys.length === 0 ? (
                <Box>
                    <Typography variant="body2" color="text.secondary">{isDirty ? text.pendingInheritance : text.inherited}</Typography>
                </Box>
            ) : null}

            <Collapse in={!loading && orderedSelectedKeys.length > 0} unmountOnExit>
                <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1.5, overflow: 'hidden' }}>
                    <Stack divider={<Divider flexItem sx={{ mx: 1.5 }} />}>
                        {orderedSelectedKeys.map(key => (
                            <Box
                                key={key}
                                sx={{
                                    display: 'grid',
                                    alignItems: 'center',
                                    gridTemplateColumns: {
                                        xs: 'minmax(0, 1fr) auto',
                                        sm: 'minmax(220px, 1fr) minmax(180px, 320px) auto',
                                    },
                                    columnGap: 1.5,
                                    rowGap: 1,
                                    minHeight: 64,
                                    px: 1.5,
                                    py: 1.25,
                                }}
                            >
                                <Box sx={{ minWidth: 0 }}>
                                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                                        <Typography variant="body2" noWrap sx={{ fontWeight: 600, color: 'text.primary' }}>{fieldLabel(key)}</Typography>
                                        <Tooltip title={fieldPurpose(key)} arrow>
                                            <InfoOutlined sx={{ fontSize: 14, color: 'text.disabled', cursor: 'help' }} />
                                        </Tooltip>
                                    </Box>
                                    <Box component="span" sx={{ ...CLAUDE_CONFIG_KEY_SX, display: 'inline-block', mt: 0.5 }}>{key}</Box>
                                </Box>
                                <Box
                                    sx={{
                                        minWidth: 0,
                                        display: 'flex',
                                        alignItems: 'center',
                                        justifyContent: { xs: 'flex-start', sm: 'flex-end' },
                                        gridColumn: { xs: '1 / -1', sm: '2' },
                                        gridRow: { xs: '2', sm: '1' },
                                    }}
                                >
                                    {renderControl(key)}
                                </Box>
                                <Tooltip title={text.remove} arrow>
                                    <IconButton
                                        size="small"
                                        aria-label={text.remove}
                                        onClick={() => removeOverride(key)}
                                        disabled={saving}
                                        sx={{
                                            gridColumn: { xs: '2', sm: '3' },
                                            gridRow: '1',
                                        }}
                                    >
                                        <Close fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            </Box>
                        ))}
                    </Stack>
                </Box>
            </Collapse>

            <Collapse in={addOpen && availableKeys.length > 0} unmountOnExit>
                <Box
                    sx={{
                        mt: 1.25,
                        maxWidth: 620,
                        border: '1px solid',
                        borderColor: 'divider',
                        borderRadius: 1.5,
                        overflow: 'hidden',
                    }}
                >
                    <TextField
                        autoFocus
                        fullWidth
                        size="small"
                        value={addQuery}
                        onChange={event => setAddQuery(event.target.value)}
                        placeholder={text.search}
                        slotProps={{
                            input: {
                                startAdornment: <InputAdornment position="start"><Search sx={{ fontSize: 17, color: 'text.secondary' }} /></InputAdornment>,
                            },
                        }}
                        sx={{
                            '& .MuiOutlinedInput-notchedOutline': { border: 0 },
                            '& .MuiOutlinedInput-root': { borderRadius: 0 },
                        }}
                    />
                    <Divider />
                    <Box sx={{ maxHeight: 260, overflowY: 'auto', py: 0.5 }}>
                        {availableGroups.map(({ group, keys }) => (
                            <Box key={group}>
                                <Typography variant="overline" color="text.secondary" sx={{ display: 'block', px: 1.5, pt: 0.5, lineHeight: 1.8 }}>
                                    {group}
                                </Typography>
                                {keys.map(key => (
                                    <ButtonBase
                                        key={key}
                                        onClick={() => addOverride(key)}
                                        sx={{
                                            width: '100%',
                                            minHeight: 36,
                                            px: 1.5,
                                            display: 'flex',
                                            alignItems: 'center',
                                            gap: 1.5,
                                            textAlign: 'left',
                                            '&:hover': { bgcolor: 'action.hover' },
                                        }}
                                    >
                                        <Typography variant="body2" sx={{ flex: 1, minWidth: 0, fontWeight: 500 }}>{fieldLabel(key)}</Typography>
                                        <Box component="span" sx={{ ...CLAUDE_CONFIG_KEY_SX, flexShrink: 1 }}>{key}</Box>
                                    </ButtonBase>
                                ))}
                            </Box>
                        ))}
                        {availableGroups.length === 0 && (
                            <Typography variant="body2" color="text.secondary" sx={{ px: 1.5, py: 1 }}>
                                {text.noSettings}
                            </Typography>
                        )}
                    </Box>
                </Box>
            </Collapse>

            {message && (
                <Alert severity={message.severity} sx={{ mt: 1 }}>{message.text}</Alert>
            )}

            <Dialog open={restoreOpen} onClose={() => setRestoreOpen(false)} maxWidth="xs" fullWidth>
                <DialogTitle>{text.restoreTitle}</DialogTitle>
                <DialogContent><Typography variant="body2">{text.restoreBody}</Typography></DialogContent>
                <DialogActions>
                    <Button color="inherit" onClick={() => setRestoreOpen(false)}>{text.cancel}</Button>
                    <Button variant="contained" onClick={handleRestoreAll}>{text.confirmRestore}</Button>
                </DialogActions>
            </Dialog>
        </UnifiedCard>
    );
};

export default ClaudeCodeProfileOverrides;
