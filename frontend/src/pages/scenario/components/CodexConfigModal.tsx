import { Alert, Box, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, FormControl, FormControlLabel, IconButton, MenuItem, Radio, RadioGroup, Select, Tab, Tabs, Tooltip, Typography } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { InfoOutlined, RestartAlt } from '@/components/icons';
import CodeBlock from '@/components/CodeBlock';
import CodexQuickConfig, { type CodexPrefs, defaultCodexPrefs, mergeSavedCodexPrefs } from './CodexQuickConfig';
import Context1MChangeBanner from './Context1MChangeBanner';
import { shouldIgnoreDialogClose } from '@/components/dialogClose';
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
type ScriptTab = 'json' | 'windows' | 'unix';
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
    const [configTab, setConfigTab] = React.useState<ScriptTab>('json');
    const [authTab, setAuthTab] = React.useState<ScriptTab>('json');
    const [catalogTab, setCatalogTab] = React.useState<ScriptTab>('json');
    const [configToml, setConfigToml] = React.useState<string>('# Loading...');
    const [authJson, setAuthJson] = React.useState<string>(`{\n  "OPENAI_API_KEY": "${token}"\n}`);
    const [catalogJson, setCatalogJson] = React.useState<string>('');
    const [previewModels, setPreviewModels] = React.useState<string[]>([]);

    // True while the applied-config readback is in flight, so the Quick tab
    // can show a spinner instead of flashing the defaults before the saved
    // values land. Mirrors ClaudeCodeConfigModal's isConfigLoading.
    const [isConfigLoading, setIsConfigLoading] = React.useState(false);

    // Apply configuration state
    const [isApplying, setIsApplying] = React.useState(false);

    // On open, restore the prefs/writeCatalog previously applied to
    // ~/.codex/config.toml; first-time users (nothing applied yet) fall back to
    // defaults. Mirrors ClaudeCodeConfigModal's open-effect hydration so the
    // form no longer resets the user's last applied values on every open.
    //
    // pendingContext1MChange is in the deps so reopening after a 1M toggle
    // re-hydrates fresh state, but it does NOT branch the body: Codex's 1M
    // setting only affects the catalog's context window (generated at preview
    // time), never the reasoning prefs edited here — so a 1M toggle must not
    // skip the applied-config readback the way it used to.
    React.useEffect(() => {
        if (!open) {
            setPrefs(defaultCodexPrefs());
            setWriteCatalog(true);
            setAuthMode('apikey');
            setSelectedOAuthProvider('');
            setCodexOAuthProviders([]);
            setIsConfigLoading(false);
            return;
        }
        let active = true;
        setIsConfigLoading(true);
        void api.getAppliedCodexConfig().then(result => {
            if (!active) return;
            if (result?.success && result.exists) {
                setPrefs(mergeSavedCodexPrefs(result.preferences || {}));
                setWriteCatalog(result.writeCatalog !== false);
            } else {
                setPrefs(defaultCodexPrefs());
                setWriteCatalog(true);
            }
        }).finally(() => {
            if (active) setIsConfigLoading(false);
        });
        return () => {
            active = false;
        };
    }, [open, pendingContext1MChange]);

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

    // Re-render the server-authoritative TOML whenever prefs or writeCatalog change
    // while the modal is open. Debounced so dragging through Select options doesn't
    // spam the backend.
    React.useEffect(() => {
        if (!open) return;
        // Direct/ChatGPT-mode preview would render OAuth tokens — skip it
        // entirely; the user gets an info card on the modal instead. Hybrid
        // still previews config.toml (it carries the provider-scoped token).
        if (authMode === 'chatgpt') return;
        let cancelled = false;
        const handle = setTimeout(async () => {
            try {
                const resp = await api.getCodexConfigPreview(prefs as Record<string, string>, writeCatalog, authMode);
                if (cancelled) return;
                if (resp?.success) {
                    setConfigToml(resp.configToml || '');
                    // Hybrid leaves auth.json alone → backend returns no authJson.
                    setAuthJson(resp.authJson || (authMode === 'apikey' ? `{\n  "OPENAI_API_KEY": "${token}"\n}` : ''));
                    setCatalogJson(resp.catalogJson || '');
                    setPreviewModels(resp.models || []);
                }
            } catch {
                // Leave existing placeholders in place; the user can still copy the
                // base URL from the page itself.
            }
        }, 250);
        return () => { cancelled = true; clearTimeout(handle); };
    }, [open, prefs, writeCatalog, token, authMode]);

    const windowsCatalogScript = `$catalogDir = Join-Path $HOME ".codex"
$catalogPath = Join-Path $catalogDir "tingly-model-catalog.json"

New-Item -ItemType Directory -Force -Path $catalogDir | Out-Null

@'
${catalogJson}
'@ | Set-Content -Path $catalogPath`;

    const unixCatalogScript = `mkdir -p ~/.codex

cat > ~/.codex/tingly-model-catalog.json <<'EOF'
${catalogJson}
EOF`;

    const windowsConfigScript = `$configDir = Join-Path $HOME ".codex"
$configPath = Join-Path $configDir "config.toml"

New-Item -ItemType Directory -Force -Path $configDir | Out-Null

@'
${configToml}
'@ | Set-Content -Path $configPath`;

    const unixConfigScript = `mkdir -p ~/.codex

cat > ~/.codex/config.toml <<'EOF'
${configToml}
EOF`;

    const windowsAuthScript = `$configDir = Join-Path $HOME ".codex"
$authPath = Join-Path $configDir "auth.json"

New-Item -ItemType Directory -Force -Path $configDir | Out-Null

@'
${authJson}
'@ | Set-Content -Path $authPath`;

    const unixAuthScript = `mkdir -p ~/.codex

cat > ~/.codex/auth.json <<'EOF'
${authJson}
EOF`;

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
        <Dialog
            open={open}
            onClose={(event, reason) => {
                if (shouldIgnoreDialogClose(reason)) {
                    return;
                }
                onClose();
            }}
            maxWidth="lg"
            fullWidth
            slotProps={{
                paper: {
                    sx: {
                        borderRadius: 3,
                        maxHeight: '90vh',
                    },
                }
            }}
        >
            <DialogTitle sx={{ pb: 1, borderBottom: 1, borderColor: 'divider', position: 'relative' }}>
                <Typography variant="h6" sx={{
                    fontWeight: 600
                }}>
                    {t('codexConfig.title')}
                </Typography>
                <Typography
                    variant="body2"
                    sx={{
                        color: "text.secondary",
                        mt: 0.5
                    }}>
                    {t('codexConfig.subtitle')}
                </Typography>
                {/* Reset only touches the Quick Config prefs, so it's shown only
                    where those prefs are visible (Quick tab, non-direct). */}
                {mainTab === 'quick' && authMode !== 'chatgpt' && (
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
                <Tabs
                    value={mainTab}
                    onChange={(_, value) => setMainTab(value)}
                    sx={{ mt: 1, minHeight: 40, '& .MuiTabs-indicator': { height: 3 } }}
                >
                    <Tab label={t('codexConfig.tabQuick')} value="quick" sx={{ minHeight: 40, textTransform: 'none' }} />
                    <Tab label={t('codexConfig.tabManual')} value="manual" sx={{ minHeight: 40, textTransform: 'none' }} />
                </Tabs>
            </DialogTitle>
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
                        <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                            <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                                <Typography variant="subtitle2" sx={{
                                    color: "text.secondary"
                                }}>
                                    Step 1 · Create or update `~/.codex/config.toml`
                                </Typography>
                                <Tabs
                                    value={configTab}
                                    onChange={(_, value) => setConfigTab(value)}
                                    variant="standard"
                                    sx={{ minHeight: 32, '& .MuiTabs-indicator': { height: 3 } }}
                                >
                                    <Tab label="TOML" value="json" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    <Tab label="Windows" value="windows" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    <Tab label="Linux/macOS" value="unix" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                </Tabs>
                            </Box>
                            <Box>
                                {configTab === 'json' && (
                                    <CodeBlock
                                        code={configToml}
                                        language="toml"
                                        filename="Create or update ~/.codex/config.toml"
                                        wrap={true}
                                        onCopy={(code) => copyToClipboard(code, 'config.toml')}
                                        maxHeight={220}
                                        minHeight={180}
                                    />
                                )}
                                {configTab === 'windows' && (
                                    <CodeBlock
                                        code={windowsConfigScript}
                                        language="js"
                                        filename="PowerShell script to setup ~/.codex/config.toml"
                                        wrap={true}
                                        onCopy={(code) => copyToClipboard(code, 'Windows config script')}
                                        maxHeight={260}
                                        minHeight={220}
                                    />
                                )}
                                {configTab === 'unix' && (
                                    <CodeBlock
                                        code={unixConfigScript}
                                        language="js"
                                        filename="Bash script to setup ~/.codex/config.toml"
                                        wrap={true}
                                        onCopy={(code) => copyToClipboard(code, 'Unix config script')}
                                        maxHeight={260}
                                        minHeight={220}
                                    />
                                )}
                            </Box>
                        </Box>

                        {authMode === 'hybrid' ? (
                            <Alert severity="info" variant="outlined" sx={{ py: 0.5 }}>
                                <strong>No <code>auth.json</code> step in hybrid mode.</strong> The gateway token
                                lives in <code>config.toml</code> above (<code>experimental_bearer_token</code>), so
                                your existing <code>~/.codex/auth.json</code> ChatGPT login is left untouched.
                            </Alert>
                        ) : (
                        <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                            <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                                <Typography variant="subtitle2" sx={{
                                    color: "text.secondary"
                                }}>
                                    Step 2 · Create or update `~/.codex/auth.json`
                                </Typography>
                                <Tabs
                                    value={authTab}
                                    onChange={(_, value) => setAuthTab(value)}
                                    variant="standard"
                                    sx={{ minHeight: 32, '& .MuiTabs-indicator': { height: 3 } }}
                                >
                                    <Tab label="JSON" value="json" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    <Tab label="Windows" value="windows" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    <Tab label="Linux/macOS" value="unix" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                </Tabs>
                            </Box>
                            <Box sx={{ mb: 1.5 }}>
                                <Typography variant="body2" sx={{
                                    color: "text.secondary"
                                }}>
                                    Set `OPENAI_API_KEY` in `~/.codex/auth.json` to the API key generated by Tingly Box. If the file already exists, update the existing value.
                                </Typography>
                            </Box>
                            <Box>
                                {authTab === 'json' && (
                                    <CodeBlock
                                        code={authJson}
                                        language="json"
                                        filename="Create or update ~/.codex/auth.json"
                                        wrap={true}
                                        onCopy={(code) => copyToClipboard(code, 'auth.json')}
                                        maxHeight={140}
                                        minHeight={100}
                                    />
                                )}
                                {authTab === 'windows' && (
                                    <CodeBlock
                                        code={windowsAuthScript}
                                        language="js"
                                        filename="PowerShell script to setup ~/.codex/auth.json"
                                        wrap={true}
                                        onCopy={(code) => copyToClipboard(code, 'Windows auth script')}
                                        maxHeight={220}
                                        minHeight={180}
                                    />
                                )}
                                {authTab === 'unix' && (
                                    <CodeBlock
                                        code={unixAuthScript}
                                        language="js"
                                        filename="Bash script to setup ~/.codex/auth.json"
                                        wrap={true}
                                        onCopy={(code) => copyToClipboard(code, 'Unix auth script')}
                                        maxHeight={220}
                                        minHeight={180}
                                    />
                                )}
                            </Box>
                        </Box>
                        )}

                        {writeCatalog && previewModels.length > 0 && catalogJson && (
                            <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                                <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                                    <Typography variant="subtitle2" sx={{
                                        color: "text.secondary"
                                    }}>
                                        Step 3 · Create or update `~/.codex/tingly-model-catalog.json`
                                    </Typography>
                                    <Tabs
                                        value={catalogTab}
                                        onChange={(_, value) => setCatalogTab(value)}
                                        variant="standard"
                                        sx={{ minHeight: 32, '& .MuiTabs-indicator': { height: 3 } }}
                                    >
                                        <Tab label="JSON" value="json" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                        <Tab label="Windows" value="windows" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                        <Tab label="Linux/macOS" value="unix" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    </Tabs>
                                </Box>
                                <Box sx={{ mb: 1.5 }}>
                                    <Typography variant="body2" sx={{
                                        color: "text.secondary"
                                    }}>
                                        Lets Codex's <code>/model</code> picker list tingly-served models. Required when <code>model_catalog_json</code> is set in config.toml.
                                    </Typography>
                                </Box>
                                <Box>
                                    {catalogTab === 'json' && (
                                        <CodeBlock
                                            code={catalogJson}
                                            language="json"
                                            filename="Create or update ~/.codex/tingly-model-catalog.json"
                                            wrap={true}
                                            onCopy={(code) => copyToClipboard(code, 'tingly-model-catalog.json')}
                                            maxHeight={220}
                                            minHeight={140}
                                        />
                                    )}
                                    {catalogTab === 'windows' && (
                                        <CodeBlock
                                            code={windowsCatalogScript}
                                            language="js"
                                            filename="PowerShell script to setup ~/.codex/tingly-model-catalog.json"
                                            wrap={true}
                                            onCopy={(code) => copyToClipboard(code, 'Windows catalog script')}
                                            maxHeight={260}
                                            minHeight={220}
                                        />
                                    )}
                                    {catalogTab === 'unix' && (
                                        <CodeBlock
                                            code={unixCatalogScript}
                                            language="js"
                                            filename="Bash script to setup ~/.codex/tingly-model-catalog.json"
                                            wrap={true}
                                            onCopy={(code) => copyToClipboard(code, 'Unix catalog script')}
                                            maxHeight={260}
                                            minHeight={220}
                                        />
                                    )}
                                </Box>
                            </Box>
                        )}
                    </Box>
                )}
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Box sx={{ display: 'flex', justifyContent: 'flex-end', gap: 1, width: '100%' }}>
                    <Button onClick={onClose} variant="outlined">
                        {t('common.close')}
                    </Button>
                    <Button
                        onClick={handleApplyConfiguration}
                        variant="contained"
                        disabled={isApplying}
                        startIcon={isApplying ? <CircularProgress size={16} color="inherit" /> : null}
                    >
                        {isApplying ? t('common.applying') : t('scenarioPage.autoConfig')}
                    </Button>
                </Box>
            </DialogActions>
        </Dialog>
    );
};

export default CodexConfigModal;
