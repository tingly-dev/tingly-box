import { useEffect, useMemo, useState } from 'react';
import {
    Alert, Autocomplete, Box, Button, Chip, CircularProgress, IconButton, InputAdornment, MenuItem, Paper, Select,
    Stack, TextField, ToggleButton, ToggleButtonGroup, Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type ConnectionDraft, type Model, type TestResponse } from '../client';
import { Cable, Check, Refresh, Visibility, VisibilityOff, Warning } from '../components/icons';
import { useNotify } from '../hooks/useNotify';
import { invalidateModels } from '../hooks/useModels';
import { useAppData } from '../contexts/AppDataContext';
import { useThemeMode, type ThemeMode } from '../contexts/ThemeModeContext';
import { THEME_MODE_VALUES, ThemeModeIcon } from '../theme/modes';
import { LANGUAGES } from '../i18n';
import { PageHeader } from '@tingly/ui';

type Style = 'anthropic' | 'openai';

const PRESETS: { key: string; style: Style; url: string }[] = [
    { key: 'tbAnthropic', style: 'anthropic', url: 'http://localhost:12580/tingly/anthropic' },
    { key: 'tbOpenAI', style: 'openai', url: 'http://localhost:12580/tingly/openai/v1' },
    { key: 'anthropic', style: 'anthropic', url: 'https://api.anthropic.com' },
    { key: 'openai', style: 'openai', url: 'https://api.openai.com/v1' },
];

/** Mirrors llm.Endpoint on the server so the field shows the real URL. */
function endpointFor(style: Style, base: string) {
    const b = base.trim().replace(/\/+$/, '');
    if (!b) return '';
    return style === 'openai' ? `${b}/chat/completions` : `${b}/v1/messages`;
}

const mono = { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', fontSize: 13 } as const;

function Card({ title, subtitle, children }: { title: string; subtitle?: string; children: React.ReactNode }) {
    return (
        <Paper variant="outlined" sx={{ borderRadius: 3, p: { xs: 2, md: 3 }, mb: 3 }}>
            <Typography variant="h4">{title}</Typography>
            {subtitle && <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>{subtitle}</Typography>}
            <Box sx={{ mt: 2.5 }}>{children}</Box>
        </Paper>
    );
}

export default function SettingsPage() {
    const { t, i18n } = useTranslation('projects');
    const notify = useNotify();
    const { settings, refreshSettings } = useAppData();
    const { mode, setMode } = useThemeMode();

    const [style, setStyle] = useState<Style>('anthropic');
    const [baseURL, setBaseURL] = useState('');
    const [apiKey, setApiKey] = useState<string | undefined>(undefined); // undefined = keep saved
    const [showKey, setShowKey] = useState(false);
    const [defaultModel, setDefaultModel] = useState('');
    const [models, setModels] = useState<Model[]>([]);
    const [modelsError, setModelsError] = useState<string | null>(null);
    const [loadingModels, setLoadingModels] = useState(false);
    const [test, setTest] = useState<TestResponse | null>(null);
    const [testing, setTesting] = useState(false);
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        if (!settings) return;
        setStyle(settings.api_style as Style);
        setBaseURL(settings.base_url);
        setDefaultModel(settings.default_model);
        setApiKey(undefined);
    }, [settings]);

    const draft: ConnectionDraft = useMemo(
        () => ({ api_style: style, base_url: baseURL, api_key: apiKey, default_model: defaultModel }),
        [style, baseURL, apiKey, defaultModel],
    );
    const dirty = !!settings && (style !== settings.api_style || baseURL !== settings.base_url || apiKey !== undefined || defaultModel !== settings.default_model);

    const loadModels = async () => {
        if (!baseURL.trim()) return;
        setLoadingModels(true);
        try {
            const r = await unwrap(api.POST('/api/v1/settings/models', { body: draft }));
            setModels(r.models);
            setModelsError(r.error ?? null);
        } catch (e) {
            setModelsError((e as Error).message);
        } finally {
            setLoadingModels(false);
        }
    };

    // Refresh the model list when the endpoint changes (debounced).
    useEffect(() => {
        if (!settings) return;
        const timer = window.setTimeout(() => void loadModels(), 500);
        return () => window.clearTimeout(timer);
    }, [style, baseURL, apiKey, !!settings]);

    const runTest = async () => {
        setTesting(true);
        setTest(null);
        try {
            setTest(await unwrap(api.POST('/api/v1/settings/test', { body: { ...draft, model: defaultModel || undefined } })));
        } catch (e) {
            setTest({ ok: false, endpoint: endpointFor(style, baseURL), model: defaultModel, latency_ms: 0, error: (e as Error).message });
        } finally {
            setTesting(false);
        }
    };

    const save = async () => {
        setSaving(true);
        try {
            await unwrap(api.PUT('/api/v1/settings', { body: draft }));
            invalidateModels();
            refreshSettings();
            notify(t('settings.saved'), 'success');
        } catch (e) {
            notify((e as Error).message, 'error');
        } finally {
            setSaving(false);
        }
    };

    const endpoint = endpointFor(style, baseURL);

    return (
        <Box sx={{ flex: 1, overflowY: 'auto' }}>
            <Box sx={{ maxWidth: 760, mx: 'auto', px: { xs: 2, md: 4 }, pt: { xs: 7, md: 5 }, pb: 6 }}>
                <PageHeader title={t('settings.title')} sx={{ mb: 3 }} />

                <Card title={t('settings.connectionTitle')} subtitle={t('settings.connectionIntro')}>
                    {!settings ? (
                        <CircularProgress size={24} />
                    ) : (
                        <Stack spacing={2.5}>
                            <Box>
                                <Typography variant="caption" color="text.secondary">{t('settings.presets')}</Typography>
                                <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, mt: 0.75 }}>
                                    {PRESETS.map((p) => {
                                        const active = p.style === style && p.url === baseURL;
                                        return (
                                            <Chip
                                                key={p.key}
                                                label={t(`settings.preset.${p.key}`)}
                                                variant={active ? 'filled' : 'outlined'}
                                                color={active ? 'primary' : 'default'}
                                                icon={active ? <Check sx={{ fontSize: 16 }} /> : undefined}
                                                onClick={() => {
                                                    setStyle(p.style);
                                                    setBaseURL(p.url);
                                                    setTest(null);
                                                }}
                                            />
                                        );
                                    })}
                                </Box>
                            </Box>

                            <Box>
                                <Typography variant="caption" color="text.secondary">{t('settings.apiStyle')}</Typography>
                                <ToggleButtonGroup exclusive size="small" value={style} onChange={(_, v) => v && setStyle(v)} sx={{ display: 'flex', mt: 0.75 }}>
                                    <ToggleButton value="anthropic" sx={{ flex: 1 }}>{t('settings.styleAnthropic')}</ToggleButton>
                                    <ToggleButton value="openai" sx={{ flex: 1 }}>{t('settings.styleOpenAI')}</ToggleButton>
                                </ToggleButtonGroup>
                            </Box>

                            <TextField
                                label={t('settings.baseURL')}
                                value={baseURL}
                                onChange={(e) => setBaseURL(e.target.value)}
                                fullWidth
                                slotProps={{ htmlInput: { style: mono, spellCheck: false } }}
                                helperText={
                                    endpoint ? (
                                        <span>
                                            {t('settings.requestsGoTo')}{' '}
                                            <Box component="span" sx={{ ...mono, fontSize: 12, color: 'text.primary' }}>{endpoint}</Box>
                                        </span>
                                    ) : undefined
                                }
                            />

                            <TextField
                                label={t('settings.apiKey')}
                                type={showKey ? 'text' : 'password'}
                                value={apiKey ?? ''}
                                onChange={(e) => setApiKey(e.target.value)}
                                placeholder={settings.api_key_set ? t('settings.apiKeyKeep', { hint: settings.api_key_hint }) : t('settings.apiKeyNone')}
                                fullWidth
                                autoComplete="off"
                                helperText={t('settings.apiKeyHelp')}
                                slotProps={{
                                    inputLabel: { shrink: true },
                                    htmlInput: { style: mono, spellCheck: false },
                                    input: {
                                        endAdornment: (
                                            <InputAdornment position="end">
                                                {settings.api_key_set && apiKey === undefined && (
                                                    <Button size="small" color="inherit" onClick={() => setApiKey('')}>{t('settings.clearKey')}</Button>
                                                )}
                                                <IconButton size="small" onClick={() => setShowKey((v) => !v)} aria-label={t('settings.toggleKey')}>
                                                    {showKey ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}
                                                </IconButton>
                                            </InputAdornment>
                                        ),
                                    },
                                }}
                            />

                            <Box>
                                <Autocomplete
                                    freeSolo
                                    options={models.map((m) => m.id)}
                                    value={defaultModel}
                                    onInputChange={(_, v) => setDefaultModel(v)}
                                    renderInput={(params) => (
                                        <TextField
                                            {...params}
                                            label={t('settings.defaultModel')}
                                            placeholder={t('settings.defaultModelAuto')}
                                            helperText={
                                                modelsError
                                                    ? t('settings.modelsFailed', { error: modelsError })
                                                    : !defaultModel && models[0]
                                                        ? t('settings.defaultModelFallback', { model: models[0].id, count: models.length })
                                                        : t('settings.modelsFound', { count: models.length })
                                            }
                                            slotProps={{
                                                ...params.slotProps,
                                                inputLabel: { ...params.slotProps?.inputLabel, shrink: true },
                                                htmlInput: { ...params.slotProps?.htmlInput, style: mono, spellCheck: false },
                                                input: {
                                                    ...params.slotProps?.input,
                                                    endAdornment: (
                                                        <>
                                                            {loadingModels ? <CircularProgress size={16} /> : (
                                                                <IconButton size="small" onClick={() => void loadModels()} aria-label={t('settings.reloadModels')}><Refresh fontSize="small" /></IconButton>
                                                            )}
                                                            {(params.slotProps?.input as { endAdornment?: React.ReactNode } | undefined)?.endAdornment}
                                                        </>
                                                    ),
                                                },
                                            }}
                                        />
                                    )}
                                />
                            </Box>

                            {test && (
                                <Alert severity={test.ok ? 'success' : 'error'} icon={test.ok ? <Check /> : <Warning />} sx={{ '& .MuiAlert-message': { minWidth: 0, width: '100%' } }}>
                                    <Typography variant="body2" sx={{ fontWeight: 600, color: 'inherit' }}>
                                        {test.ok ? t('settings.testOk', { ms: test.latency_ms }) : t('settings.testFailed')}
                                    </Typography>
                                    <Box component="dl" sx={{ m: 0, mt: 0.75, display: 'grid', gridTemplateColumns: 'auto 1fr', columnGap: 1.5, rowGap: 0.25, fontSize: 12.5, '& dt': { color: 'text.secondary' }, '& dd': { m: 0, ...mono, fontSize: 12.5, wordBreak: 'break-all' } }}>
                                        <dt>{t('settings.testEndpoint')}</dt><dd>{test.endpoint}</dd>
                                        {test.model && (<><dt>{t('settings.testModel')}</dt><dd>{test.model}</dd></>)}
                                        {test.reply && (<><dt>{t('settings.testReply')}</dt><dd>{test.reply}</dd></>)}
                                        {test.error && (<><dt>{t('settings.testError')}</dt><dd>{test.error}</dd></>)}
                                    </Box>
                                </Alert>
                            )}

                            <Box sx={{ display: 'flex', gap: 1.5, justifyContent: 'flex-end' }}>
                                <Button variant="outlined" startIcon={testing ? <CircularProgress size={14} /> : <Cable />} onClick={() => void runTest()} disabled={testing || !baseURL.trim()}>
                                    {t('settings.test')}
                                </Button>
                                <Button variant="contained" onClick={() => void save()} disabled={!dirty || saving || !baseURL.trim()}>
                                    {t('common.save')}
                                </Button>
                            </Box>
                        </Stack>
                    )}
                </Card>

                <Card title={t('settings.appearanceTitle')}>
                    <Stack spacing={2}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                            <Typography sx={{ flex: 1, color: 'text.primary' }}>{t('settings.theme')}</Typography>
                            <Select size="small" value={mode} onChange={(e) => setMode(e.target.value as ThemeMode)}
                                renderValue={(m) => <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}><ThemeModeIcon mode={m} size={16} />{t(`theme.${m}`)}</Box>}>
                                {THEME_MODE_VALUES.map((m) => (
                                    <MenuItem key={m} value={m} sx={{ gap: 1 }}><ThemeModeIcon mode={m} size={16} />{t(`theme.${m}`)}</MenuItem>
                                ))}
                            </Select>
                        </Box>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                            <Typography sx={{ flex: 1, color: 'text.primary' }}>{t('settings.language')}</Typography>
                            <Select size="small" value={i18n.resolvedLanguage ?? 'en'} onChange={(e) => void i18n.changeLanguage(e.target.value)}>
                                {LANGUAGES.map((l) => <MenuItem key={l.code} value={l.code}>{l.label}</MenuItem>)}
                            </Select>
                        </Box>
                    </Stack>
                </Card>
            </Box>
        </Box>
    );
}
