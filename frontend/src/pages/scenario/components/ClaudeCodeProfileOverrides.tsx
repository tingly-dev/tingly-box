import {
    Alert,
    Box,
    Button,
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
    Menu,
    MenuItem,
    Select,
    Stack,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import UnifiedCard from '@/components/UnifiedCard';
import { Add, Close, InfoOutlined, RestartAlt } from '@/components/icons';
import { api } from '@/services/api';
import {
    CLAUDE_CODE_DEFAULT_MODE_OPTIONS,
    CLAUDE_CODE_DEFAULT_MODE_TEXT,
    CLAUDE_CODE_FIELDS_TEXT,
    CLAUDE_CODE_FIELD_STRUCT,
    type ClaudeCodeDefaultMode,
    type ClaudeCodePrefs,
    type FieldStruct,
    type Lang,
    type PrefsKey,
} from './ClaudeCodeQuickConfig';

type OverrideKey = PrefsKey | 'defaultMode';

interface ClaudeCodeProfileOverridesProps {
    profileId: string;
}

const TEXT = {
    zh: {
        title: 'Profile 覆盖',
        inherited: '当前完全继承主配置和模型路由',
        pendingInheritance: '覆盖已移除；保存 Profile 后恢复继承',
        summary: (count: number) => `${count} 项运行参数由此 Profile 覆盖`,
        hint: '未列出的参数继续继承；启动时自动生成运行配置。',
        add: '添加覆盖',
        allAdded: '所有支持的参数都已添加',
        common: '常用',
        more: '更多参数',
        permissionMode: '默认权限模式',
        permissionPurpose: '这个 Profile 启动 Claude Code 时使用的权限模式',
        inheritedValue: '继承值',
        notSet: '未设置',
        enabled: '开启',
        disabled: '关闭',
        remove: '移除覆盖并恢复继承',
        save: '保存 Profile',
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
        summary: (count: number) => `${count} runtime setting${count === 1 ? '' : 's'} overridden by this profile`,
        hint: 'Unlisted settings keep inheriting; runtime settings are generated automatically at launch.',
        add: 'Add override',
        allAdded: 'All supported settings are already added',
        common: 'Common',
        more: 'More settings',
        permissionMode: 'Default permission mode',
        permissionPurpose: 'Permission mode used when this profile starts Claude Code',
        inheritedValue: 'Inherited',
        notSet: 'Not set',
        enabled: 'On',
        disabled: 'Off',
        remove: 'Remove override and restore inheritance',
        save: 'Save Profile',
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
} as const;

const COMMON_KEYS: OverrideKey[] = ['CLAUDE_CODE_MAX_OUTPUT_TOKENS', 'defaultMode'];
const FIELD_ORDER: OverrideKey[] = [
    ...COMMON_KEYS,
    ...CLAUDE_CODE_FIELD_STRUCT
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
    for (const field of CLAUDE_CODE_FIELD_STRUCT) {
        if ((basePrefs[field.envName] ?? '') !== (effectivePrefs[field.envName] ?? '')) {
            keys.add(field.envName);
        }
    }
    if (inheritedMode !== effectiveMode) keys.add('defaultMode');
    return keys;
};

const ClaudeCodeProfileOverrides: React.FC<ClaudeCodeProfileOverridesProps> = ({ profileId }) => {
    const { i18n } = useTranslation();
    const lang: Lang = i18n.language === 'zh' ? 'zh' : 'en';
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
    const [saving, setSaving] = React.useState(false);
    const [loadError, setLoadError] = React.useState(false);
    const [message, setMessage] = React.useState<{ severity: 'success' | 'error'; text: string } | null>(null);
    const [addAnchor, setAddAnchor] = React.useState<HTMLElement | null>(null);
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
        setLoadError(false);
        return true;
    }, []);

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
    }, [applyResponse, profileId, text.loadFailed]);

    const orderedSelectedKeys = React.useMemo(
        () => FIELD_ORDER.filter(key => selectedKeys.has(key)),
        [selectedKeys],
    );
    const availableCommon = COMMON_KEYS.filter(key => !selectedKeys.has(key));
    const availableMore = FIELD_ORDER.filter(key => !COMMON_KEYS.includes(key) && !selectedKeys.has(key));
    const isDirty = !prefsEqual(prefs, loadedPrefs) || defaultMode !== loadedMode;

    const addOverride = (key: OverrideKey) => {
        setSelectedKeys(current => new Set(current).add(key));
        setAddAnchor(null);
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

    const handleSave = async () => {
        setSaving(true);
        setMessage(null);
        try {
            const result = await api.updateClaudeCodeProfileConfig(
                'claude_code',
                profileId,
                prefs as Record<string, string>,
                defaultMode,
            );
            if (applyResponse(result)) {
                setMessage({ severity: 'success', text: text.saved });
            } else {
                setMessage({ severity: 'error', text: result?.error || text.saveFailed });
            }
        } finally {
            setSaving(false);
        }
    };

    const handleRestoreAll = async () => {
        setRestoreOpen(false);
        setSaving(true);
        setMessage(null);
        try {
            const result = await api.resetClaudeCodeProfileConfig('claude_code', profileId);
            if (applyResponse(result)) {
                setMessage({ severity: 'success', text: text.restored });
            } else {
                setMessage({ severity: 'error', text: result?.error || text.saveFailed });
            }
        } finally {
            setSaving(false);
        }
    };

    const renderInheritedValue = (key: OverrideKey): string => {
        if (key === 'defaultMode') return modeText[inheritedMode].label;
        const value = basePrefs[key];
        if (!value) return text.notSet;
        const field = FIELD_BY_KEY.get(key);
        if (field?.kind === 'bool') return value === '1' ? text.enabled : text.disabled;
        return value;
    };

    const renderControl = (key: OverrideKey) => {
        if (key === 'defaultMode') {
            return (
                <FormControl size="small" fullWidth>
                    <Select
                        value={defaultMode}
                        onChange={event => {
                            setDefaultMode(event.target.value as ClaudeCodeDefaultMode);
                            setMessage(null);
                        }}
                    >
                        {CLAUDE_CODE_DEFAULT_MODE_OPTIONS.map(mode => (
                            <MenuItem key={mode} value={mode}>
                                <Stack direction="row" justifyContent="space-between" spacing={2} sx={{ width: '100%' }}>
                                    <Typography variant="body2">{modeText[mode].label}</Typography>
                                    <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>{mode}</Typography>
                                </Stack>
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
                <FormControl size="small" fullWidth>
                    <Select value={value === '1' ? '1' : '0'} onChange={event => updatePreference(key, event.target.value === '1' ? '1' : '')}>
                        <MenuItem value="1">{text.enabled}</MenuItem>
                        <MenuItem value="0">{text.disabled}</MenuItem>
                    </Select>
                </FormControl>
            );
        }
        return (
            <TextField
                size="small"
                fullWidth
                value={value}
                onChange={event => updatePreference(key, event.target.value)}
                placeholder={fieldText[key].placeholder}
                inputProps={field.kind === 'int' ? { inputMode: 'numeric' } : undefined}
                InputProps={{
                    endAdornment: field.unit
                        ? <InputAdornment position="end"><Typography variant="caption" color="text.disabled">{field.unit}</Typography></InputAdornment>
                        : undefined,
                    sx: { fontFamily: field.kind === 'model' ? 'monospace' : undefined, fontSize: '0.85rem' },
                }}
            />
        );
    };

    const fieldLabel = (key: OverrideKey) => key === 'defaultMode' ? text.permissionMode : fieldText[key].label;
    const fieldPurpose = (key: OverrideKey) => key === 'defaultMode' ? text.permissionPurpose : fieldText[key].purpose;

    return (
        <UnifiedCard
            size="full"
            title={text.title}
            subtitle={text.hint}
            rightAction={(
                <Button
                    size="small"
                    variant="outlined"
                    startIcon={<Add fontSize="small" />}
                    onClick={event => setAddAnchor(event.currentTarget)}
                    disabled={loading || saving || loadError || (availableCommon.length === 0 && availableMore.length === 0)}
                >
                    {text.add}
                </Button>
            )}
        >

            <Menu
                anchorEl={addAnchor}
                open={Boolean(addAnchor)}
                onClose={() => setAddAnchor(null)}
                slotProps={{ paper: { sx: { width: 390, maxHeight: 440 } } }}
            >
                {availableCommon.length > 0 && (
                    <Typography variant="overline" color="text.secondary" sx={{ display: 'block', px: 2, pt: 0.5 }}>{text.common}</Typography>
                )}
                {availableCommon.map(key => (
                    <MenuItem key={key} onClick={() => addOverride(key)}>
                        <Box sx={{ minWidth: 0 }}>
                            <Typography variant="body2" fontWeight={500}>{fieldLabel(key)}</Typography>
                            <Typography variant="caption" color="text.secondary" noWrap>{fieldPurpose(key)}</Typography>
                        </Box>
                    </MenuItem>
                ))}
                {availableCommon.length > 0 && availableMore.length > 0 && <Divider />}
                {availableMore.length > 0 && (
                    <Typography variant="overline" color="text.secondary" sx={{ display: 'block', px: 2, pt: 0.5 }}>{text.more}</Typography>
                )}
                {availableMore.map(key => (
                    <MenuItem key={key} onClick={() => addOverride(key)}>
                        <Box sx={{ minWidth: 0 }}>
                            <Typography variant="body2" fontWeight={500}>{fieldLabel(key)}</Typography>
                            <Typography variant="caption" color="text.secondary" noWrap>{fieldPurpose(key)}</Typography>
                        </Box>
                    </MenuItem>
                ))}
                {availableCommon.length === 0 && availableMore.length === 0 && <MenuItem disabled>{text.allAdded}</MenuItem>}
            </Menu>

            {loading ? (
                <Box sx={{ display: 'flex', justifyContent: 'center', py: 2 }}><CircularProgress size={20} /></Box>
            ) : orderedSelectedKeys.length === 0 ? (
                <Box sx={{ py: 1.5 }}>
                    <Typography variant="body2" color="text.secondary">{isDirty ? text.pendingInheritance : text.inherited}</Typography>
                </Box>
            ) : (
                <Typography variant="body2" color="text.secondary" sx={{ mb: 0.75 }}>
                    {text.summary(orderedSelectedKeys.length)}
                </Typography>
            )}

            <Collapse in={!loading && orderedSelectedKeys.length > 0} unmountOnExit>
                <Box>
                    <Stack divider={<Divider flexItem />}>
                        {orderedSelectedKeys.map(key => (
                            <Box
                                key={key}
                                sx={{
                                    display: 'grid',
                                    alignItems: 'center',
                                    gap: { xs: 1, md: 2 },
                                    gridTemplateColumns: { xs: 'minmax(0, 1fr) auto', md: 'minmax(180px, 0.8fr) minmax(180px, 0.8fr) minmax(220px, 1fr) auto' },
                                    py: 1.25,
                                }}
                            >
                                <Box sx={{ minWidth: 0 }}>
                                    <Stack direction="row" spacing={0.5} alignItems="center">
                                        <Typography variant="body2" fontWeight={600} noWrap>{fieldLabel(key)}</Typography>
                                        <Tooltip title={fieldPurpose(key)} arrow>
                                            <InfoOutlined sx={{ fontSize: 14, color: 'text.disabled' }} />
                                        </Tooltip>
                                    </Stack>
                                    <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }} noWrap>{key}</Typography>
                                </Box>
                                <Box sx={{ minWidth: 0, gridColumn: { xs: '1', md: '2' } }}>
                                    <Typography variant="caption" color="text.secondary">{text.inheritedValue}</Typography>
                                    <Typography variant="body2" color="text.secondary" noWrap title={renderInheritedValue(key)}>{renderInheritedValue(key)}</Typography>
                                </Box>
                                <Box sx={{ minWidth: 0, gridColumn: { xs: '1', md: '3' } }}>{renderControl(key)}</Box>
                                <Tooltip title={text.remove} arrow>
                                    <IconButton
                                        size="small"
                                        aria-label={text.remove}
                                        onClick={() => removeOverride(key)}
                                        disabled={saving}
                                        sx={{ gridColumn: { xs: '2', md: '4' }, gridRow: { xs: '1 / 4', md: 'auto' } }}
                                    >
                                        <Close fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            </Box>
                        ))}
                    </Stack>
                </Box>
            </Collapse>

            {!loading && !loadError && (orderedSelectedKeys.length > 0 || hasOverrides || isDirty) && (
                <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ pt: 1.25 }}>
                    <Button
                        size="small"
                        color="inherit"
                        startIcon={<RestartAlt fontSize="small" />}
                        onClick={() => setRestoreOpen(true)}
                        disabled={saving || (!hasOverrides && !isDirty)}
                    >
                        {text.restoreAll}
                    </Button>
                    <Button size="small" variant="contained" onClick={handleSave} disabled={saving || !isDirty}>
                        {saving ? <CircularProgress size={15} color="inherit" /> : text.save}
                    </Button>
                </Stack>
            )}

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
