import { Alert, Box, Button, Checkbox, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, FormControlLabel, Tab, Tabs, Typography } from '@mui/material';
import React from 'react';
import CodeBlock from './CodeBlock';
import { shouldIgnoreDialogClose } from './dialogClose';
import { api } from '@/services/api';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';

interface CodexConfigModalProps {
    open: boolean;
    onClose: () => void;
    copyToClipboard: (text: string, label: string) => Promise<void>;
}

type ScriptTab = 'json' | 'windows' | 'unix';
type SessionAction = 'import' | 'undo';

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
    models?: string[];
    message?: string;
}

const SHOW_CODEX_SESSION_IMPORT = false;

const CodexConfigModal: React.FC<CodexConfigModalProps> = ({
    open,
    onClose,
    copyToClipboard,
}) => {
    // Keep token in context as a fallback for the auth.json preview while
    // the preview API request is in flight.
    const { token } = useScenarioPageModal();
    const [configTab, setConfigTab] = React.useState<ScriptTab>('json');
    const [authTab, setAuthTab] = React.useState<ScriptTab>('json');
    const [sessionAction, setSessionAction] = React.useState<SessionAction | null>(null);
    const [isSubmitting, setIsSubmitting] = React.useState(false);
    const [result, setResult] = React.useState<any | null>(null);
    const [error, setError] = React.useState<string | null>(null);
    const [createBackup, setCreateBackup] = React.useState(false);
    const [autoUndoOnStop, setAutoUndoOnStop] = React.useState(false);
    const [configToml, setConfigToml] = React.useState<string>('# Loading...');
    const [authJson, setAuthJson] = React.useState<string>(`{\n  "OPENAI_API_KEY": "${token}"\n}`);

    // Apply configuration state
    const [isApplying, setIsApplying] = React.useState(false);
    const [applyResult, setApplyResult] = React.useState<ApplyCodexConfigResponse | null>(null);
    const [applyError, setApplyError] = React.useState<string | null>(null);

    React.useEffect(() => {
        if (!open) {
            resetApplyState();
            return;
        }
        let cancelled = false;
        (async () => {
            try {
                const resp = await api.getCodexConfigPreview();
                if (cancelled) return;
                if (resp?.success) {
                    setConfigToml(resp.configToml || '');
                    setAuthJson(resp.authJson || `{\n  "OPENAI_API_KEY": "${token}"\n}`);
                }
            } catch {
                // Leave existing placeholders in place; the user can still copy the
                // base URL from the page itself.
            }
        })();
        return () => { cancelled = true; };
    }, [open, token]);

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
        setIsApplying(true);
        setApplyError(null);
        setApplyResult(null);
        try {
            const response = await api.applyCodexConfig();
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
            </DialogTitle>

            <DialogContent sx={{ p: 3 }}>
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
                                    sx={{ color: 'common.white' }}
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
            </DialogContent>

            <DialogActions sx={{ px: 3, pb: 2, display: 'flex', flexDirection: 'column', gap: 1 }}>
                {applyResult?.success && (
                    <Alert severity="success" sx={{ width: '100%' }}>
                        <Typography variant="body2" fontWeight={600}>
                            Configuration applied successfully!
                        </Typography>
                        <Box sx={{ mt: 1, display: 'flex', flexDirection: 'column', gap: 0.5 }}>
                            {applyResult.configResult?.created && (
                                <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                                    ✓ Created ~/.codex/config.toml
                                </Typography>
                            )}
                            {applyResult.configResult?.updated && (
                                <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                                    ✓ Updated ~/.codex/config.toml
                                </Typography>
                            )}
                            {applyResult.authResult?.created && (
                                <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                                    ✓ Created ~/.codex/auth.json
                                </Typography>
                            )}
                            {applyResult.authResult?.updated && (
                                <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                                    ✓ Updated ~/.codex/auth.json
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
                        {isApplying ? 'Applying...' : 'Apply Configuration'}
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
                        Cancel
                    </Button>
                    <Button
                        onClick={handleSessionAction}
                        variant="contained"
                        disabled={isSubmitting || !sessionAction}
                        startIcon={isSubmitting ? <CircularProgress size={16} color="inherit" /> : null}
                        sx={{ color: 'common.white' }}
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
