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
    Popover,
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
import { Add, ArrowForward, Close, RestartAlt, Search } from '@/components/icons';
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
import { fontMono } from '@/theme/fonts';
import { useNotify } from '@/hooks/useNotify';

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
        inheritedFrom: '继承值',
        notSet: '未设置',
        on: '开',
        off: '关',
        discard: '放弃',
        add: '添加',
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
        restoreAllShort: '恢复继承',
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
        inheritedFrom: 'Inherited',
        notSet: 'Not set',
        on: 'On',
        off: 'Off',
        discard: 'Discard',
        add: 'Add',
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
        restoreAllShort: 'Restore inheritance',
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
        inheritedFrom: 'Наследуется',
        notSet: 'Не задано',
        on: 'Вкл',
        off: 'Выкл',
        discard: 'Отменить',
        add: 'Добавить',
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
        restoreAllShort: 'Вернуть наследование',
        restoreTitle: 'Очистить все переопределения профиля?',
        restoreBody: 'Будут очищены только переопределения рабочих параметров этого профиля. Основная конфигурация, маршрутизация моделей и сам профиль не изменятся.',
        restored: 'Наследование основной конфигурации и маршрутизации моделей восстановлено',
        cancel: 'Отмена',
        confirmRestore: 'Вернуть наследование',
        loadFailed: 'Не удалось загрузить переопределения профиля',
        saveFailed: 'Не удалось сохранить переопределения профиля',
    },
} as const;

// Most used first: permission mode, then the context-size knobs.
const COMMON_KEYS: OverrideKey[] = ['defaultMode', 'CLAUDE_CODE_AUTO_COMPACT_WINDOW', 'CLAUDE_CODE_MAX_OUTPUT_TOKENS'];
const PROFILE_OVERRIDE_FIELDS = CLAUDE_CODE_FIELD_STRUCT.filter(field => field.kind !== 'model');
const RULE_OWNED_MODEL_KEYS = new Set(CLAUDE_CODE_FIELD_STRUCT.filter(field => field.kind === 'model').map(field => field.envName));

// Model slots can change independently while this editor is open. They are
// owned by routing rules and must not be submitted as stale profile overrides.
export const profileRuntimePreferences = (prefs: ClaudeCodePrefs): Record<string, string> =>
    Object.fromEntries(Object.entries(prefs).filter(([key]) => !RULE_OWNED_MODEL_KEYS.has(key as PrefsKey)));
const FIELD_ORDER: OverrideKey[] = [
    ...COMMON_KEYS,
    ...PROFILE_OVERRIDE_FIELDS
        .map(field => field.envName)
        .filter(key => !COMMON_KEYS.includes(key)),
];
const FIELD_BY_KEY = new Map<PrefsKey, FieldStruct>(
    CLAUDE_CODE_FIELD_STRUCT.map(field => [field.envName, field]),
);

// Rows are single-line, so controls drop to a 32px height to match.
const COMPACT_INPUT_SX = {
    '& .MuiInputBase-input, & .MuiSelect-select': { py: 0.625 },
} as const;

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
    const notify = useNotify();

    const [basePrefs, setBasePrefs] = React.useState<ClaudeCodePrefs>({});
    const [prefs, setPrefs] = React.useState<ClaudeCodePrefs>({});
    const [loadedPrefs, setLoadedPrefs] = React.useState<ClaudeCodePrefs>({});
    const [inheritedMode, setInheritedMode] = React.useState<ClaudeCodeDefaultMode>('auto');
    const [defaultMode, setDefaultMode] = React.useState<ClaudeCodeDefaultMode>('auto');
    const [loadedMode, setLoadedMode] = React.useState<ClaudeCodeDefaultMode>('auto');
    const [selectedKeys, setSelectedKeys] = React.useState<Set<OverrideKey>>(new Set());
    const [loadedKeys, setLoadedKeys] = React.useState<Set<OverrideKey>>(new Set());
    const [hasOverrides, setHasOverrides] = React.useState(false);
    const [loading, setLoading] = React.useState(true);
    const [savingAction, setSavingAction] = React.useState<'save' | 'restore' | null>(null);
    const [loadError, setLoadError] = React.useState(false);
    const [message, setMessage] = React.useState<{ severity: 'success' | 'error'; text: string } | null>(null);
    const [addAnchor, setAddAnchor] = React.useState<HTMLElement | null>(null);
    const addOpen = Boolean(addAnchor);
    const [addQuery, setAddQuery] = React.useState('');
    const [restoreOpen, setRestoreOpen] = React.useState(false);

    const applyResponse = React.useCallback((result: any): boolean => {
        if (!result?.success || !result.data) return false;
        const nextBase = (result.data.basePreferences || {}) as ClaudeCodePrefs;
        const nextPrefs = (result.data.preferences || {}) as ClaudeCodePrefs;
        const nextInheritedMode = (result.data.inheritedDefaultMode || 'auto') as ClaudeCodeDefaultMode;
        const nextMode = (result.data.defaultMode || nextInheritedMode) as ClaudeCodeDefaultMode;
        setBasePrefs(nextBase);
        setPrefs(nextPrefs);
        setLoadedPrefs(nextPrefs);
        setInheritedMode(nextInheritedMode);
        setDefaultMode(nextMode);
        setLoadedMode(nextMode);
        const nextKeys = deriveOverrideKeys(nextBase, nextPrefs, nextInheritedMode, nextMode);
        setSelectedKeys(nextKeys);
        setLoadedKeys(nextKeys);
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
    // Marks the rows the pending Save will write. Value-based (like isDirty), so
    // a freshly added row still holding its inherited value stays unmarked.
    const isRowChanged = (key: OverrideKey) => key === 'defaultMode'
        ? defaultMode !== loadedMode
        : (prefs[key] ?? '') !== (loadedPrefs[key] ?? '');
    const saving = savingAction !== null;

    const addOverride = (key: OverrideKey) => {
        setSelectedKeys(current => new Set(current).add(key));
        setAddAnchor(null);
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

    const discardChanges = () => {
        setPrefs(loadedPrefs);
        setDefaultMode(loadedMode);
        setSelectedKeys(loadedKeys);
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
                notify.success(successMessage);
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
            profileRuntimePreferences(prefs),
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
                <FormControl size="small" sx={{ width: 260, maxWidth: '100%', ...COMPACT_INPUT_SX }}>
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
                                    <Typography variant="caption" color="text.secondary" sx={{ fontFamily: fontMono }}>{mode}</Typography>
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
                sx={{ width: field.kind === 'text' ? 260 : 160, maxWidth: '100%', ...COMPACT_INPUT_SX }}
                slotProps={{
                    input: {
                        inputProps: field.kind === 'int' ? { inputMode: 'numeric' } : undefined,
                        endAdornment: field.unit
                            ? <InputAdornment position="end"><Typography variant="caption" color="text.disabled">{field.unit}</Typography></InputAdornment>
                            : undefined,
                    },
                }}
            />
        );
    };

    // 'defaultMode' is a display-only key for this override row; the actual
    // settings.json location is nested under "permissions" (with a legacy
    // top-level mirror for older Claude Code installs) — see prefs.go.
    const fieldBadge = (key: OverrideKey) => key === 'defaultMode' ? 'permissions.defaultMode' : key;
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
    // The concrete value this profile would run with if the override were
    // removed, so each row reads as "inherited X → override Y".
    const inheritedDisplay = (key: OverrideKey): string => {
        if (key === 'defaultMode') return modeText[inheritedMode].label;
        const field = FIELD_BY_KEY.get(key);
        const value = basePrefs[key] ?? '';
        if (field?.kind === 'bool') return value === '1' ? text.on : text.off;
        if (!value) return text.notSet;
        return field?.unit ? `${value} ${field.unit}` : value;
    };

    const hasRows = orderedSelectedKeys.length > 0;
    // With rows on screen, each "inherited → override" row explains itself, so
    // the subtitle only speaks when the card is empty.
    const subtitle = loading || hasRows
        ? undefined
        : isDirty ? text.pendingInheritance : text.inherited;

    return (
        <UnifiedCard
            size="full"
            title={text.title}
            subtitle={subtitle}
            titleMarginBottom={hasRows ? 1.25 : 0}
            sx={{
                '& > .MuiCardContent-root': {
                    p: 2.5,
                    '&:last-child': { pb: 2.5 },
                },
            }}
            rightAction={(
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                    {loading && <CircularProgress size={16} />}
                    {isDirty ? (
                        <>
                            <Button size="small" color="inherit" onClick={discardChanges} disabled={saving} sx={{ color: 'text.secondary' }}>
                                {text.discard}
                            </Button>
                            <Button size="small" variant="contained" onClick={handleSave} disabled={saving}>
                                {savingAction === 'save' ? <CircularProgress size={15} color="inherit" /> : text.save}
                            </Button>
                        </>
                    ) : !loading && !loadError && hasOverrides && (
                        <Tooltip title={text.restoreAll} arrow>
                            <span>
                                <Button
                                    size="small"
                                    color="inherit"
                                    startIcon={savingAction === 'restore'
                                        ? <CircularProgress size={14} color="inherit" />
                                        : <RestartAlt fontSize="small" />}
                                    onClick={() => setRestoreOpen(true)}
                                    disabled={saving}
                                    sx={{ color: 'text.secondary' }}
                                >
                                    {text.restoreAllShort}
                                </Button>
                            </span>
                        </Tooltip>
                    )}
                    <Button
                        size="small"
                        variant="outlined"
                        startIcon={<Add fontSize="small" />}
                        onClick={event => {
                            setAddAnchor(event.currentTarget);
                            setAddQuery('');
                        }}
                        disabled={loading || saving || loadError || availableKeys.length === 0}
                    >
                        {text.add}
                    </Button>
                </Box>
            )}
        >
            <Collapse in={!loading && hasRows} unmountOnExit>
                <Stack divider={<Divider flexItem />} sx={{ borderTop: '1px solid', borderBottom: '1px solid', borderColor: 'divider' }}>
                    {orderedSelectedKeys.map(key => {
                        const changed = isRowChanged(key);
                        return (
                            <Box
                                key={key}
                                sx={{
                                    display: 'grid',
                                    alignItems: 'center',
                                    gridTemplateColumns: {
                                        xs: 'minmax(0, 1fr) auto',
                                        md: 'minmax(0, 1fr) auto auto',
                                    },
                                    columnGap: 1.5,
                                    rowGap: 0.75,
                                    px: 1.25,
                                    py: 0.75,
                                    // Unsaved rows get a left accent so the pending Save has a visible scope.
                                    boxShadow: theme => changed ? `inset 3px 0 0 ${theme.palette.primary.main}` : 'none',
                                }}
                            >
                                <Box sx={{ minWidth: 0, display: 'flex', alignItems: 'center', gap: 1 }}>
                                    <Tooltip title={fieldPurpose(key)} arrow placement="top-start">
                                        <Typography variant="body2" noWrap sx={{ fontWeight: 600, color: 'text.primary', flexShrink: 0, cursor: 'help' }}>
                                            {fieldLabel(key)}
                                        </Typography>
                                    </Tooltip>
                                    <Box component="span" sx={{ ...CLAUDE_CONFIG_KEY_SX, flexShrink: 1, minWidth: 0 }}>{fieldBadge(key)}</Box>
                                </Box>
                                <Box
                                    sx={{
                                        minWidth: 0,
                                        display: 'flex',
                                        alignItems: 'center',
                                        gap: 1,
                                        justifyContent: { xs: 'flex-start', md: 'flex-end' },
                                        gridColumn: { xs: '1 / -1', md: '2' },
                                        gridRow: { xs: '2', md: '1' },
                                    }}
                                >
                                    <Tooltip title={text.inheritedFrom} arrow>
                                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, whiteSpace: 'nowrap' }}>
                                            <Typography variant="caption" sx={{ fontFamily: key === 'defaultMode' ? undefined : fontMono, color: 'text.secondary' }}>{inheritedDisplay(key)}</Typography>
                                            <ArrowForward sx={{ fontSize: 14, color: 'text.disabled' }} />
                                        </Box>
                                    </Tooltip>
                                    {renderControl(key)}
                                </Box>
                                <Tooltip title={text.remove} arrow>
                                    <IconButton
                                        size="small"
                                        aria-label={text.remove}
                                        onClick={() => removeOverride(key)}
                                        disabled={saving}
                                        sx={{
                                            gridColumn: { xs: '2', md: '3' },
                                            gridRow: '1',
                                        }}
                                    >
                                        <Close fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            </Box>
                        );
                    })}
                </Stack>
            </Collapse>

            <Popover
                open={addOpen && availableKeys.length > 0}
                anchorEl={addAnchor}
                onClose={() => setAddAnchor(null)}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                transformOrigin={{ vertical: 'top', horizontal: 'right' }}
                slotProps={{ paper: { sx: { mt: 0.5, width: 'min(620px, calc(100vw - 32px))', borderRadius: 1.5 } } }}
            >
                <Box
                    sx={{
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
                                        <Typography variant="caption" color="text.secondary" noWrap sx={{ fontFamily: key === 'defaultMode' ? undefined : fontMono, flexShrink: 0 }}>{inheritedDisplay(key)}</Typography>
                                        <Box component="span" sx={{ ...CLAUDE_CONFIG_KEY_SX, flexShrink: 1 }}>{fieldBadge(key)}</Box>
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
            </Popover>

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
