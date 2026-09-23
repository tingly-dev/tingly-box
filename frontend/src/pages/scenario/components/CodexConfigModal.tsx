import { Alert, Box, CircularProgress, DialogActions, DialogContent, FormControl, FormControlLabel, IconButton, MenuItem, Radio, RadioGroup, Select, Tooltip, Typography } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { InfoOutlined, RestartAlt } from '@/components/icons';
import CodexQuickConfig, { type CodexPrefs, defaultCodexPrefs, mergeSavedCodexPrefs } from './CodexQuickConfig';
import Context1MChangeBanner from './Context1MChangeBanner';
import { ConfigModalShell, QuickApplyActions } from './config/ConfigModalShell';
import { ManualFileSection } from './config/ManualFileSection';
import { writeFileScripts } from './config/writeFileScripts';
import { useAppliedPrefs } from './config/useAppliedPrefs';
import { useDebouncedPreview } from './config/useDebouncedPreview';
import { api } from '@/services/api';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';

interface CodexConfigModalProps {
    open: boolean;
    onClose: () => void;
    copyToClipboard: (text: string, label: string) => Promise<void>;
    pendingContext1MChange?: boolean | null;
    // Shared page-level toast, used for apply success/error so feedback is
    // consistent with the rest of the scenario pages.
    showNotification?: (message: string, severity: 'success' | 'error' | 'info' | 'warning') => void;
}

type MainTab = 'quick' | 'manual';
// The three mutually-exclusive ways to authenticate Codex. Modeled as one
// 3-way select rather than routing×keep-login axes: those axes aren't truly
// orthogonal (direct routing always keeps the official login), so a grid would
// have a dead cell. See .design/codex-auth.md.
//   - apikey:  route through Tingly Box, gateway key in auth.json.
//   - hybrid:  route through Tingly Box, gateway key in config.toml, official
//              ChatGPT login preserved in auth.json.
//   - chatgpt: codex talks to OpenAI directly using the official login.
type AuthMode = 'apikey' | 'chatgpt' | 'hybrid';

interface CodexOAuthProviderOption {
    uuid: string;
    name: string;
}

const CodexConfigModal: React.FC<CodexConfigModalProps> = ({
    open,
    onClose,
    copyToClipboard,
    pendingContext1MChange,
    showNotification,
}) => {
    const { t } = useTranslation();
    // Keep token in context as a fallback for the auth.json preview while
    // the preview API request is in flight.
    const { token } = useScenarioPageModal();
    const [mainTab, setMainTab] = React.useState<MainTab>('quick');
    const [prefs, setPrefs] = React.useState<CodexPrefs>(() => defaultCodexPrefs());
    const [writeCatalog, setWriteCatalog] = React.useState(true);
    // Three mutually-exclusive auth states, picked directly. They aren't two
    // orthogonal axes: "direct routing without keeping the official login" is
    // an invalid combination, so a routing×keep-login grid would leave a dead
    // cell (and force a disabled checkbox). A 3-way select models the real
    // state space honestly. See .design/codex-auth.md.
    const [authMode, setAuthMode] = React.useState<AuthMode>('apikey');
    // The OAuth provider picker is relevant whenever a ChatGPT login is in play.
    const showOAuthSelector = authMode === 'chatgpt' || authMode === 'hybrid';
    const [codexOAuthProviders, setCodexOAuthProviders] = React.useState<CodexOAuthProviderOption[]>([]);
    const [selectedOAuthProvider, setSelectedOAuthProvider] = React.useState<string>('');
    const [configToml, setConfigToml] = React.useState<string>('# Loading...');
    const [authJson, setAuthJson] = React.useState<string>(`{\n  "OPENAI_API_KEY": "${token}"\n}`);
    const [catalogJson, setCatalogJson] = React.useState<string>('');
    const [previewModels, setPreviewModels] = React.useState<string[]>([]);

    // Apply configuration state
    const [isApplying, setIsApplying] = React.useState(false);

    // On open, restore the prefs/writeCatalog previously applied to
    // ~/.codex/config.toml; first-time users (nothing applied yet) fall back to
    // defaults. Mirrors ClaudeCodeConfigModal's open-effect hydration so the
    // form no longer resets the user's last applied values on every open.
    //
    // pendingContext1MChange is a hydration dep so reopening after a 1M toggle
    // re-hydrates fresh state, but it does NOT branch the body: Codex's 1M
    // setting only affects the catalog's context window (generated at preview
    // time), never the reasoning prefs edited here — so a 1M toggle must not
    // skip the applied-config readback the way it used to.
    const isConfigLoading = useAppliedPrefs({
        open,
        deps: [pendingContext1MChange],
        fetch: () => api.getAppliedCodexConfig(),
        applySaved: (result) => {
            setPrefs(mergeSavedCodexPrefs(result.preferences || {}));
            setWriteCatalog(result.writeCatalog !== false);
        },
        applyDefaults: () => {
            setPrefs(defaultCodexPrefs());
            setWriteCatalog(true);
        },
        onClosed: () => {
            setAuthMode('apikey');
            setSelectedOAuthProvider('');
            setCodexOAuthProviders([]);
        },
    });

    // Fetch Codex OAuth providers only when the picker is actually shown (direct
    // or hybrid) — no network cost for the default gateway path. Direct mode
    // auto-selects the first provider (it's required); hybrid leaves it empty
    // (the smart default is "don't touch auth.json", per ux-principles #6).
    React.useEffect(() => {
        if (!open || !showOAuthSelector) return;
        let cancelled = false;
        (async () => {
            try {
                const resp = await api.getProviders();
                if (cancelled) return;
                const list: any[] = Array.isArray(resp?.data) ? resp.data : [];
                const codexOAuth = list
                    .filter((p) => p?.auth_type === 'oauth' && (p?.oauth_detail?.issuer === 'codex' || p?.oauth_detail?.provider_type === 'codex'))
                    .map((p) => ({ uuid: p.uuid, name: p.name }));
                setCodexOAuthProviders(codexOAuth);
                if (authMode === 'chatgpt') {
                    setSelectedOAuthProvider((prev) => prev || codexOAuth[0]?.uuid || '');
                }
            } catch {
                setCodexOAuthProviders([]);
            }
        })();
        return () => { cancelled = true; };
    }, [open, showOAuthSelector, authMode]);

    // Re-render the server-authoritative TOML whenever prefs or writeCatalog
    // change while the modal is open.
    // Direct/ChatGPT-mode preview would render OAuth tokens — skip it
    // entirely; the user gets an info card on the modal instead. Hybrid
    // still previews config.toml (it carries the provider-scoped token).
    useDebouncedPreview({
        open,
        enabled: authMode !== 'chatgpt',
        deps: [prefs, writeCatalog, token, authMode],
        fetch: async () => {
            const resp = await api.getCodexConfigPreview(prefs as Record<string, string>, writeCatalog, authMode);
            if (resp?.success) {
                setConfigToml(resp.configToml || '');
                // Hybrid leaves auth.json alone → backend returns no authJson.
                setAuthJson(resp.authJson || (authMode === 'apikey' ? `{\n  "OPENAI_API_KEY": "${token}"\n}` : ''));
                setCatalogJson(resp.catalogJson || '');
                setPreviewModels(resp.models || []);
            }
        },
    });

    // Here-doc write scripts for the ~/.codex files; only the file name
    // differs between the config.toml / auth.json / catalog steps.
    const codexWriteScripts = (fileVar: string, filename: string, content: string) => writeFileScripts({
        dirVar: '$configDir',
        fileVar,
        dirSetupWindows: `$configDir = Join-Path $HOME ".codex"`,
        fileSetupWindows: `${fileVar} = Join-Path $configDir "${filename}"`,
        dirSetupUnix: `mkdir -p ~/.codex`,
        fileUnix: `~/.codex/${filename}`,
        content,
    });

    const configScripts = codexWriteScripts('$configPath', 'config.toml', configToml);
    const authScripts = codexWriteScripts('$authPath', 'auth.json', authJson);
    const catalogScripts = codexWriteScripts('$catalogPath', 'tingly-model-catalog.json', catalogJson);

    const handleApplyConfiguration = async () => {
        if (authMode === 'chatgpt' && !selectedOAuthProvider) {
            showNotification?.(t('codexConfig.selectOAuthProvider'), 'error');
            return;
        }
        setIsApplying(true);
        try {
            const response = await api.applyCodexConfig(
                prefs as Record<string, string>,
                writeCatalog,
                authMode,
                showOAuthSelector ? (selectedOAuthProvider || undefined) : undefined,
            );
            if (response?.success) {
                showNotification?.(t('codexConfig.applySuccess'), 'success');
            } else {
                showNotification?.(response?.message || t('codexConfig.applyFailed'), 'error');
            }
        } catch (err: any) {
            showNotification?.(err?.message || t('codexConfig.applyFailed'), 'error');
        } finally {
            setIsApplying(false);
        }
    };

    return (
        <ConfigModalShell
            open={open}
            onClose={onClose}
            title={t('codexConfig.title')}
            subtitle={t('codexConfig.subtitle')}
            headerAction={mainTab === 'quick' && authMode !== 'chatgpt' && (
                // Reset only touches the Quick Config prefs, so it's shown only
                // where those prefs are visible (Quick tab, non-direct).
                <Tooltip title={t('codexConfig.resetTooltip')} arrow>
                    <IconButton
                        size="small"
                        onClick={() => setPrefs(defaultCodexPrefs())}
                        sx={{ position: 'absolute', top: 12, right: 12 }}
                    >
                        <RestartAlt fontSize="small" />
                    </IconButton>
                </Tooltip>
            )}
            tabs={{
                value: mainTab,
                onChange: (value) => setMainTab(value as MainTab),
                items: [
                    { value: 'quick', label: t('codexConfig.tabQuick') },
                    { value: 'manual', label: t('codexConfig.tabManual') },
                ],
            }}
        >
            <DialogContent sx={{ p: 3 }}>
                {pendingContext1MChange != null && (
                    <Context1MChangeBanner enabled={pendingContext1MChange} clientName="Codex" />
                )}

                <Box sx={{ mb: 2, p: 2, borderRadius: 2, bgcolor: 'action.hover' }}>
                    {/* One 3-way select for the three valid auth states. Each
                        option's caption carries the concrete consequence
                        (routing + what lands in auth.json) so the two
                        gateway-based options are easy to tell apart. */}
                    <Typography variant="subtitle2" sx={{ mb: 1 }}>
                        {t('codexConfig.authMode.title')}
                    </Typography>
                    <RadioGroup
                        value={authMode}
                        onChange={(e) => setAuthMode(e.target.value as AuthMode)}
                    >
                        <FormControlLabel
                            value="apikey"
                            sx={{ alignItems: 'flex-start', mb: 0.5 }}
                            control={<Radio size="small" sx={{ pt: 0 }} />}
                            label={
                                <Box>
                                    <Typography variant="body2">{t('codexConfig.authMode.apikey.label')}</Typography>
                                    <Typography variant="caption" sx={{
                                        color: "text.secondary"
                                    }}>
                                        {t('codexConfig.authMode.apikey.captionPrefix')} <code>~/.codex/auth.json</code>.
                                    </Typography>
                                </Box>
                            }
                        />
                        <FormControlLabel
                            value="hybrid"
                            sx={{ alignItems: 'flex-start', mb: 0.5 }}
                            control={<Radio size="small" sx={{ pt: 0 }} />}
                            label={
                                <Box>
                                    <Typography variant="body2">
                                        {t('codexConfig.authMode.hybrid.label')}
                                    </Typography>
                                    <Typography variant="caption" sx={{
                                        color: "text.secondary"
                                    }}>
                                        {t('codexConfig.authMode.hybrid.caption')}
                                    </Typography>
                                </Box>
                            }
                        />
                        <FormControlLabel
                            value="chatgpt"
                            sx={{ alignItems: 'flex-start' }}
                            control={<Radio size="small" sx={{ pt: 0 }} />}
                            label={
                                <Box>
                                    <Typography variant="body2">{t('codexConfig.authMode.chatgpt.label')}</Typography>
                                    <Typography variant="caption" sx={{
                                        color: "text.secondary"
                                    }}>
                                        {t('codexConfig.authMode.chatgpt.caption')}
                                    </Typography>
                                </Box>
                            }
                        />
                    </RadioGroup>
                </Box>

                {/* The account is a parameter of the two ChatGPT-login modes, not
                    part of picking the mode — so it lives in its own labeled block
                    rather than inside the Authentication selector. */}
                {showOAuthSelector && (
                    <Box sx={{ mb: 2, px: 0.5 }}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mb: 1 }}>
                            <Typography variant="subtitle2">{t('codexConfig.oauthAccount.title')}</Typography>
                            <Tooltip
                                arrow
                                placement="top"
                                title={
                                    <Box sx={{ maxWidth: 320 }}>
                                        <Typography variant="caption">
                                            {authMode === 'hybrid'
                                                ? t('codexConfig.oauthAccount.tooltipHybrid')
                                                : t('codexConfig.oauthAccount.tooltipChatgpt')}
                                        </Typography>
                                    </Box>
                                }
                            >
                                <InfoOutlined sx={{ fontSize: 16, color: 'text.secondary', cursor: 'help' }} />
                            </Tooltip>
                        </Box>
                        <FormControl size="small" fullWidth sx={{ maxWidth: 420 }}>
                            <Select
                                displayEmpty
                                value={selectedOAuthProvider}
                                onChange={(e) => setSelectedOAuthProvider(e.target.value as string)}
                            >
                                <MenuItem value="" disabled={authMode === 'chatgpt'}>
                                    {authMode === 'chatgpt'
                                        ? (codexOAuthProviders.length === 0
                                            ? t('codexConfig.oauthAccount.noProvider')
                                            : t('codexConfig.oauthAccount.selectProvider'))
                                        : t('codexConfig.oauthAccount.keepExisting')}
                                </MenuItem>
                                {codexOAuthProviders.map((p) => (
                                    <MenuItem key={p.uuid} value={p.uuid}>{p.name}</MenuItem>
                                ))}
                            </Select>
                        </FormControl>
                    </Box>
                )}
                {authMode !== 'chatgpt' && mainTab === 'quick' && isConfigLoading && (
                    <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
                        <CircularProgress size={28} />
                    </Box>
                )}

                {authMode !== 'chatgpt' && mainTab === 'quick' && !isConfigLoading && (
                    <CodexQuickConfig
                        prefs={prefs}
                        setPrefs={setPrefs}
                        writeCatalog={writeCatalog}
                        setWriteCatalog={setWriteCatalog}
                    />
                )}

                {authMode !== 'chatgpt' && mainTab === 'manual' && (
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
                        <ManualFileSection
                            heading="Step 1 · Create or update `~/.codex/config.toml`"
                            copyToClipboard={copyToClipboard}
                            tabs={[
                                {
                                    label: 'TOML',
                                    value: 'json',
                                    code: configToml,
                                    language: 'toml',
                                    filename: 'Create or update ~/.codex/config.toml',
                                    copyLabel: 'config.toml',
                                    maxHeight: 220,
                                    minHeight: 180,
                                },
                                {
                                    label: 'Windows',
                                    value: 'windows',
                                    code: configScripts.windows,
                                    language: 'js',
                                    filename: 'PowerShell script to setup ~/.codex/config.toml',
                                    copyLabel: 'Windows config script',
                                    maxHeight: 260,
                                    minHeight: 220,
                                },
                                {
                                    label: 'Linux/macOS',
                                    value: 'unix',
                                    code: configScripts.unix,
                                    language: 'js',
                                    filename: 'Bash script to setup ~/.codex/config.toml',
                                    copyLabel: 'Unix config script',
                                    maxHeight: 260,
                                    minHeight: 220,
                                },
                            ]}
                        />

                        {authMode === 'hybrid' ? (
                            <Alert severity="info" variant="outlined" sx={{ py: 0.5 }}>
                                <strong>No <code>auth.json</code> step in hybrid mode.</strong> The gateway token
                                lives in <code>config.toml</code> above (<code>experimental_bearer_token</code>), so
                                your existing <code>~/.codex/auth.json</code> ChatGPT login is left untouched.
                            </Alert>
                        ) : (
                            <ManualFileSection
                                heading="Step 2 · Create or update `~/.codex/auth.json`"
                                description="Set `OPENAI_API_KEY` in `~/.codex/auth.json` to the API key generated by Tingly Box. If the file already exists, update the existing value."
                                copyToClipboard={copyToClipboard}
                                tabs={[
                                    {
                                        label: 'JSON',
                                        value: 'json',
                                        code: authJson,
                                        language: 'json',
                                        filename: 'Create or update ~/.codex/auth.json',
                                        copyLabel: 'auth.json',
                                        maxHeight: 140,
                                        minHeight: 100,
                                    },
                                    {
                                        label: 'Windows',
                                        value: 'windows',
                                        code: authScripts.windows,
                                        language: 'js',
                                        filename: 'PowerShell script to setup ~/.codex/auth.json',
                                        copyLabel: 'Windows auth script',
                                        maxHeight: 220,
                                        minHeight: 180,
                                    },
                                    {
                                        label: 'Linux/macOS',
                                        value: 'unix',
                                        code: authScripts.unix,
                                        language: 'js',
                                        filename: 'Bash script to setup ~/.codex/auth.json',
                                        copyLabel: 'Unix auth script',
                                        maxHeight: 220,
                                        minHeight: 180,
                                    },
                                ]}
                            />
                        )}

                        {writeCatalog && previewModels.length > 0 && catalogJson && (
                            <ManualFileSection
                                heading="Step 3 · Create or update `~/.codex/tingly-model-catalog.json`"
                                description={
                                    <>
                                        Lets Codex's <code>/model</code> picker list tingly-served models. Required when <code>model_catalog_json</code> is set in config.toml.
                                    </>
                                }
                                copyToClipboard={copyToClipboard}
                                tabs={[
                                    {
                                        label: 'JSON',
                                        value: 'json',
                                        code: catalogJson,
                                        language: 'json',
                                        filename: 'Create or update ~/.codex/tingly-model-catalog.json',
                                        copyLabel: 'tingly-model-catalog.json',
                                        maxHeight: 220,
                                        minHeight: 140,
                                    },
                                    {
                                        label: 'Windows',
                                        value: 'windows',
                                        code: catalogScripts.windows,
                                        language: 'js',
                                        filename: 'PowerShell script to setup ~/.codex/tingly-model-catalog.json',
                                        copyLabel: 'Windows catalog script',
                                        maxHeight: 260,
                                        minHeight: 220,
                                    },
                                    {
                                        label: 'Linux/macOS',
                                        value: 'unix',
                                        code: catalogScripts.unix,
                                        language: 'js',
                                        filename: 'Bash script to setup ~/.codex/tingly-model-catalog.json',
                                        copyLabel: 'Unix catalog script',
                                        maxHeight: 260,
                                        minHeight: 220,
                                    },
                                ]}
                            />
                        )}
                    </Box>
                )}
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <QuickApplyActions
                    onClose={onClose}
                    onApply={handleApplyConfiguration}
                    applying={isApplying}
                />
            </DialogActions>
        </ConfigModalShell>
    );
};

export default CodexConfigModal;
