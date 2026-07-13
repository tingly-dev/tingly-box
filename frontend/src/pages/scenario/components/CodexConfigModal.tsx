import { Alert, Box, Button, Checkbox, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, FormControl, FormControlLabel, MenuItem, Radio, RadioGroup, Select, Tab, Tabs, Typography } from '@mui/material';
import React from 'react';
import CodeBlock from '@/components/CodeBlock';
import CodexQuickConfig, { type CodexPrefs, defaultCodexPrefs } from './CodexQuickConfig';
import Context1MChangeBanner from './Context1MChangeBanner';
import { shouldIgnoreDialogClose } from '@/components/dialogClose';
import { api } from '@/services/api';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';

interface CodexConfigModalProps {
    open: boolean;
    onClose: () => void;
    copyToClipboard: (text: string, label: string) => Promise<void>;
    pendingContext1MChange?: boolean | null;
}

type MainTab = 'quick' | 'manual';
type ScriptTab = 'json' | 'windows' | 'unix';
type SessionAction = 'import' | 'undo';
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

interface ApplyCodexConfigResponse {
    success: boolean;
    configResult?: {
        success: boolean;
        backupPath?: string;
        message?: string;
        created?: boolean;
        updated?: boolean;
    };
    authResult?: {
        success: boolean;
        backupPath?: string;
        message?: string;
        created?: boolean;
        updated?: boolean;
    };
    catalogWritten?: boolean;
    models?: string[];
    message?: string;
}

const SHOW_CODEX_SESSION_IMPORT = false;

const CodexConfigModal: React.FC<CodexConfigModalProps> = ({
    open,
    onClose,
    copyToClipboard,
    pendingContext1MChange,
}) => {
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
    const [sessionAction, setSessionAction] = React.useState<SessionAction | null>(null);
    const [isSubmitting, setIsSubmitting] = React.useState(false);
    const [result, setResult] = React.useState<any | null>(null);
    const [error, setError] = React.useState<string | null>(null);
    const [createBackup, setCreateBackup] = React.useState(false);
    const [autoUndoOnStop, setAutoUndoOnStop] = React.useState(false);
    const [configToml, setConfigToml] = React.useState<string>('# Loading...');
    const [authJson, setAuthJson] = React.useState<string>(`{\n  "OPENAI_API_KEY": "${token}"\n}`);
    const [catalogJson, setCatalogJson] = React.useState<string>('');
    const [previewModels, setPreviewModels] = React.useState<string[]>([]);

    // Apply configuration state
    const [isApplying, setIsApplying] = React.useState(false);
    const [applyResult, setApplyResult] = React.useState<ApplyCodexConfigResponse | null>(null);
    const [applyError, setApplyError] = React.useState<string | null>(null);

    // Seed defaults on open; reset transient state on close.
    React.useEffect(() => {
        if (!open) {
            resetApplyState();
            return;
        }
        setPrefs(defaultCodexPrefs());
        setAuthMode('apikey');
        setSelectedOAuthProvider('');
        setCodexOAuthProviders([]);
    }, [open]);

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

    const handleSessionAction = async () => {
        if (!sessionAction) {
            return;
        }
        setIsSubmitting(true);
        setError(null);
        setResult(null);
        try {
            const payload = sessionAction === 'import'
                ? { createBackup, autoUndoOnStop }
                : { sourceProvider: 'tingly-box', targetProvider: 'openai', createBackup };
            const response = await api.importCodexOpenAISessions(payload);
            if (!response?.success) {
                setError(response?.error || response?.message || 'Failed to update Codex sessions');
                return;
            }
            setResult(response);
        } catch (err: any) {
            setError(err?.message || 'Failed to update Codex sessions');
        } finally {
            setIsSubmitting(false);
            setSessionAction(null);
        }
    };

    const handleApplyConfiguration = async () => {
        if (authMode === 'chatgpt' && !selectedOAuthProvider) {
            setApplyError('Select a Codex OAuth provider to export.');
            return;
        }
        setIsApplying(true);
        setApplyError(null);
        setApplyResult(null);
        try {
            const response = await api.applyCodexConfig(
                prefs as Record<string, string>,
                writeCatalog,
                authMode,
                showOAuthSelector ? (selectedOAuthProvider || undefined) : undefined,
            );
            if (response?.success) {
                setApplyResult(response);
            } else {
                setApplyError(response?.message || 'Failed to apply configuration');
            }
        } catch (err: any) {
            setApplyError(err?.message || 'Failed to apply configuration');
        } finally {
            setIsApplying(false);
        }
    };

    const resetApplyState = () => {
        setApplyResult(null);
        setApplyError(null);
    };

    return (
        <Dialog
            open={open}
            onClose={(event, reason) => {
                if (shouldIgnoreDialogClose(reason)) {
                    return;
                }
                resetApplyState();
                onClose();
            }}
            maxWidth="lg"
            fullWidth
            PaperProps={{
                sx: {
                    borderRadius: 3,
                    maxHeight: '90vh',
                },
            }}
        >
            <DialogTitle sx={{ pb: 1, borderBottom: 1, borderColor: 'divider' }}>
                <Typography variant="h6" fontWeight={600}>
                    Configure Codex
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                    Configure Codex to use Tingly Box through `~/.codex/config.toml` and `~/.codex/auth.json`
                </Typography>
                <Tabs
                    value={mainTab}
                    onChange={(_, value) => setMainTab(value)}
                    sx={{ mt: 1, minHeight: 40, '& .MuiTabs-indicator': { height: 3 } }}
                >
                    <Tab label="Quick Config" value="quick" sx={{ minHeight: 40, textTransform: 'none' }} />
                    <Tab label="Manual" value="manual" sx={{ minHeight: 40, textTransform: 'none' }} />
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
                        Authentication
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
                                    <Typography variant="body2">Tingly Box gateway</Typography>
                                    <Typography variant="caption" color="text.secondary">
                                        codex routes through Tingly Box. Gateway key written to <code>~/.codex/auth.json</code>.
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
                                        Tingly Box gateway + keep official ChatGPT login
                                    </Typography>
                                    <Typography variant="caption" color="text.secondary">
                                        Routes through Tingly Box, but the gateway key moves into <code>config.toml</code> so
                                        your ChatGPT login in <code>auth.json</code> stays intact — Codex App still recognizes
                                        your account (remote control, plugins, account display).
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
                                    <Typography variant="body2">Direct to OpenAI</Typography>
                                    <Typography variant="caption" color="text.secondary">
                                        codex talks to OpenAI directly using your official ChatGPT login. No gateway.
                                    </Typography>
                                </Box>
                            }
                        />
                    </RadioGroup>

                    {showOAuthSelector && (
                        <Box sx={{ mt: 1.5, display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                            <FormControl size="small" sx={{ maxWidth: 360 }}>
                                <Select
                                    displayEmpty
                                    value={selectedOAuthProvider}
                                    onChange={(e) => setSelectedOAuthProvider(e.target.value as string)}
                                >
                                    <MenuItem value="" disabled={authMode === 'chatgpt'}>
                                        {authMode === 'chatgpt'
                                            ? (codexOAuthProviders.length === 0
                                                ? 'No Codex OAuth provider — log in first'
                                                : 'Select a Codex OAuth provider')
                                            : 'Keep existing auth.json (don’t modify)'}
                                    </MenuItem>
                                    {codexOAuthProviders.map((p) => (
                                        <MenuItem key={p.uuid} value={p.uuid}>{p.name}</MenuItem>
                                    ))}
                                </Select>
                            </FormControl>
                            {authMode === 'hybrid' ? (
                                <Alert severity="info" variant="outlined" sx={{ py: 0.5 }}>
                                    The gateway token is written into <code>config.toml</code>'s provider block
                                    (<code>experimental_bearer_token</code>). Pick a stored Codex login above to
                                    (re)write it into <code>auth.json</code>, or leave it as <em>Keep existing</em> to
                                    not touch the file.
                                </Alert>
                            ) : (
                                <Alert severity="info" variant="outlined" sx={{ py: 0.5 }}>
                                    Exports the OAuth tokens to <code>~/.codex/auth.json</code> and removes the
                                    tingly gateway keys from <code>config.toml</code> so codex CLI talks directly to
                                    OpenAI. Tingly Box will <strong>not</strong> manage token refresh after this —
                                    codex CLI owns the token lifecycle from here on.{' '}
                                    If <code>id_token</code> is missing in the exported file, re-authenticate
                                    via the OAuth page and apply again.
                                </Alert>
                            )}
                        </Box>
                    )}
                </Box>
                {authMode !== 'chatgpt' && mainTab === 'quick' && (
                    <CodexQuickConfig
                        prefs={prefs}
                        setPrefs={setPrefs}
                        onResetDefaults={() => setPrefs(defaultCodexPrefs())}
                        writeCatalog={writeCatalog}
                        setWriteCatalog={setWriteCatalog}
                    />
                )}

                {authMode !== 'chatgpt' && mainTab === 'manual' && (
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
                        <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                            <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                                <Typography variant="subtitle2" color="text.secondary">
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
                                <Typography variant="subtitle2" color="text.secondary">
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
                                <Typography variant="body2" color="text.secondary">
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
                                    <Typography variant="subtitle2" color="text.secondary">
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
                                    <Typography variant="body2" color="text.secondary">
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

                        {SHOW_CODEX_SESSION_IMPORT && (
                            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                                <Typography variant="subtitle2" color="text.secondary">
                                    Step 3 · Optional: import previous OpenAI sessions
                                </Typography>
                                <Typography variant="body2" color="text.secondary">
                                    If you previously used Codex with the built-in OpenAI provider, import those local sessions so they remain visible after switching to `tingly-box`. If needed, you can undo the import later.
                                </Typography>
                                <Box sx={{ display: 'flex', gap: 1 }}>
                                    <Button
                                        variant="contained"
                                        onClick={() => setSessionAction('import')}
                                        disabled={isSubmitting}
                                    >
                                        Import Sessions
                                    </Button>
                                    <Button
                                        variant="contained"
                                        onClick={() => setSessionAction('undo')}
                                        disabled={isSubmitting}
                                    >
                                        Undo Import
                                    </Button>
                                </Box>
                                {error && <Alert severity="error">{error}</Alert>}
                                {result && (
                                    <Alert severity="success">
                                        Updated {result.updatedSessionFiles || 0} active sessions, {result.updatedArchivedFiles || 0} archived sessions, and {result.updatedThreadRows || 0} SQLite thread records.
                                        {Array.isArray(result.skippedLockedFiles) && result.skippedLockedFiles.length > 0
                                            ? ` Skipped ${result.skippedLockedFiles.length} locked files; close Codex and retry if needed.`
                                            : ''}
                                    </Alert>
                                )}
                            </Box>
                        )}
                    </Box>
                )}
            </DialogContent>

            <DialogActions sx={{ px: 3, pb: 2, display: 'flex', flexDirection: 'column', gap: 1 }}>
                {applyResult?.success && (
                    <Alert severity="success" sx={{ width: '100%' }}>
                        <Typography variant="body2" fontWeight={600}>
                            Configuration applied successfully!
                        </Typography>
                        <Box sx={{ mt: 1, display: 'flex', flexDirection: 'column', gap: 0.5 }}>
                            {applyResult.configResult?.message && (
                                <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                                    ✓ {applyResult.configResult.message}
                                </Typography>
                            )}
                            {applyResult.authResult?.message && (
                                <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                                    ✓ {applyResult.authResult.message}
                                </Typography>
                            )}
                            {applyResult.configResult?.backupPath && (
                                <Typography variant="caption" sx={{ fontFamily: 'monospace', color: 'text.secondary' }}>
                                    Backup: {applyResult.configResult.backupPath}
                                </Typography>
                            )}
                        </Box>
                    </Alert>
                )}
                {applyError && (
                    <Alert severity="error" sx={{ width: '100%' }}>
                        {applyError}
                    </Alert>
                )}
                <Box sx={{ display: 'flex', justifyContent: 'flex-end', gap: 1, width: '100%' }}>
                    <Button onClick={onClose} variant="outlined">
                        Close
                    </Button>
                    <Button
                        onClick={handleApplyConfiguration}
                        variant="contained"
                        disabled={isApplying}
                        startIcon={isApplying ? <CircularProgress size={16} color="inherit" /> : null}
                    >
                        {isApplying ? 'Applying...' : 'Auto Config'}
                    </Button>
                </Box>
            </DialogActions>

            <Dialog
                open={SHOW_CODEX_SESSION_IMPORT && sessionAction !== null}
                onClose={(event, reason) => {
                    if (isSubmitting || shouldIgnoreDialogClose(reason)) {
                        return;
                    }
                    setSessionAction(null);
                }}
                maxWidth="sm"
                fullWidth
            >
                <DialogTitle>
                    {sessionAction === 'import' ? 'Import Sessions' : 'Undo Import'}
                </DialogTitle>
                <DialogContent sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                    <Typography variant="body2" color="text.secondary">
                        {sessionAction === 'import'
                            ? 'This will rewrite local Codex session metadata from `openai` to `tingly-box`, and update the local SQLite thread index so those sessions are visible after switching providers.'
                            : 'This will rewrite local Codex session metadata from `tingly-box` back to `openai`, and update the local SQLite thread index so those sessions are visible again under the default OpenAI provider.'}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        {sessionAction === 'import'
                            ? 'Backups are optional. Enable them only if you need a rollback copy of local session files and the SQLite thread index.'
                            : 'Undo import rewrites local session metadata back to `openai` without creating backups.'}
                    </Typography>
                    {sessionAction === 'import' && (
                        <>
                            <FormControlLabel
                                control={
                                    <Checkbox
                                        checked={createBackup}
                                        onChange={(event) => setCreateBackup(event.target.checked)}
                                        disabled={isSubmitting}
                                    />
                                }
                                label="Create backup before modifying local Codex files"
                                sx={{ my: -0.5 }}
                            />
                            <FormControlLabel
                                control={
                                    <Checkbox
                                        checked={autoUndoOnStop}
                                        onChange={(event) => setAutoUndoOnStop(event.target.checked)}
                                        disabled={isSubmitting}
                                    />
                                }
                                label="Automatically undo import when Tingly Box exits"
                                sx={{ my: -0.5 }}
                            />
                        </>
                    )}
                    {error && <Alert severity="error">{error}</Alert>}
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={() => setSessionAction(null)} color="inherit" disabled={isSubmitting}>
                        Close
                    </Button>
                    <Button
                        onClick={handleSessionAction}
                        variant="contained"
                        disabled={isSubmitting || !sessionAction}
                        startIcon={isSubmitting ? <CircularProgress size={16} color="inherit" /> : null}
                    >
                        {isSubmitting
                            ? 'Processing...'
                            : sessionAction === 'import'
                                ? 'Confirm Import'
                                : 'Confirm Undo'}
                    </Button>
                </DialogActions>
            </Dialog>
        </Dialog>
    );
};

export default CodexConfigModal;
