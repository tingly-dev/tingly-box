import { Alert, AlertTitle, Box, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, IconButton, Link, Tab, Tabs, Typography } from '@mui/material';
import { Close as CloseIcon } from '@/components/icons';
import { VisibilityOutlined as VisibilityOutlinedIcon } from '@/components/icons';
import React from 'react';
import { useTranslation } from 'react-i18next';
import CodeBlock from './CodeBlock';
import { isFullEdition } from '@/utils/edition';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';
import ClaudeCodeQuickConfig, { derivePrefsFromRules, prefsToEnvPreview } from './ClaudeCodeQuickConfig';
import type { ClaudeCodePrefs } from './ClaudeCodeQuickConfig';
import type { AgentApplyResult } from './AgentSetupCard';

type ConfigMode = 'unified' | 'separate' | 'smart';

interface ClaudeCodeConfigModalProps {
    open: boolean;
    onClose: () => void;
    configMode: ConfigMode;
    baseUrl: string;
    rules: any[];
    copyToClipboard: (text: string, label: string) => Promise<void>;
    // Apply the current quick-config prefs. The modal owns prefs state;
    // this callback is what writes them to ~/.claude/settings.json. The
    // returned AgentApplyResult is rendered in-modal so the user sees
    // which files were touched and where the backup landed.
    onApplyWithPrefs?: (prefs: ClaudeCodePrefs, installStatusLine: boolean) => Promise<AgentApplyResult>;
    isApplyLoading?: boolean;
}

type MainTab = 'quick' | 'manual';
type ScriptTab = 'json' | 'windows' | 'unix';

// Modal-local copy that doesn't fit either `claudeCode.*` (English-only
// today) or QuickConfig's bundled text. Two flat maps picked at render
// time keeps this file self-contained and easy to tune.
const MODAL_TEXT = {
    zh: {
        tabQuick: '自动配置',
        tabManual: '手动',
        previewButton: '预览生成的 env',
        previewTitle: '预览 — 将写入 ~/.claude/settings.json 的 env 段',
        applySuccess: '配置已写入',
        applyFailure: '应用失败',
        createdLabel: '创建',
        updatedLabel: '更新',
        backupLabel: '已备份至',
    },
    en: {
        tabQuick: 'Auto Config',
        tabManual: 'Manual',
        previewButton: 'Preview generated env',
        previewTitle: 'Preview — env block written to ~/.claude/settings.json',
        applySuccess: 'Configuration applied',
        applyFailure: 'Apply failed',
        createdLabel: 'Created',
        updatedLabel: 'Updated',
        backupLabel: 'Backup saved to',
    },
} as const;

// Helper to generate common Node.js script for writing config files
const generateNodeScript = (settingsPath: string, envConfig: Record<string, any>) => {
    return `const fs = require("fs");
const path = require("path");
const os = require("os");

const homeDir = os.homedir();
const targetPath = path.join(homeDir, "${settingsPath}");

// Create directory if needed
const targetDir = path.dirname(targetPath);
if (!fs.existsSync(targetDir)) {
    fs.mkdirSync(targetDir, { recursive: true });
}

const config = ${JSON.stringify(envConfig, null, 4)};

let existing = {};
if (fs.existsSync(targetPath)) {
    const content = fs.readFileSync(targetPath, "utf-8");
    try { existing = JSON.parse(content); } catch (e) {}
}

const merged = settingsPath.includes("settings.json")
    ? { ...existing, env: config }
    : { ...existing, ...config };

fs.writeFileSync(targetPath, JSON.stringify(merged, null, 2));
console.log("Config written to", targetPath);`;
};

const ClaudeCodeConfigModal: React.FC<ClaudeCodeConfigModalProps> = ({
    open,
    onClose,
    configMode,
    baseUrl,
    rules,
    copyToClipboard,
    onApplyWithPrefs,
    isApplyLoading = false,
}) => {
    const { token } = useScenarioPageModal();
    const { t, i18n } = useTranslation();
    const modalText = MODAL_TEXT[i18n.language === 'zh' ? 'zh' : 'en'];
    const [mainTab, setMainTab] = React.useState<MainTab>('quick');
    const [settingsTab, setSettingsTab] = React.useState<ScriptTab>('json');
    const [claudeJsonTab, setClaudeJsonTab] = React.useState<ScriptTab>('json');
    const [statusLineTab, setStatusLineTab] = React.useState<ScriptTab>('json');
    const [previewOpen, setPreviewOpen] = React.useState(false);
    const [applyResult, setApplyResult] = React.useState<AgentApplyResult | null>(null);

    // Prefs is the single source of truth for both tabs. Re-seed when the
    // modal isn't open so we never clobber the user's unsaved edits.
    const [prefs, setPrefs] = React.useState<ClaudeCodePrefs>(() =>
        derivePrefsFromRules({ rules, mode: configMode })
    );
    React.useEffect(() => {
        if (!open) {
            setPrefs(derivePrefsFromRules({ rules, mode: configMode }));
            setApplyResult(null);
        }
    }, [open, configMode, rules]);

    // Editing prefs after a previous Apply invalidates the success state —
    // hide the old alert so the user can tell their next Apply hasn't run yet.
    const setPrefsAndClearResult = React.useCallback((next: ClaudeCodePrefs) => {
        setPrefs(next);
        setApplyResult(null);
    }, []);

    const claudeJsonConfig = { hasCompletedOnboarding: true };

    // Env map for both the manual tab (display/copy) and the preview dialog.
    // Derived from prefs so what the user sees matches what Apply will write.
    const envConfig = React.useMemo(
        () => prefsToEnvPreview(prefs, baseUrl, token),
        [prefs, baseUrl, token],
    );

    const generateSettingsConfig = React.useCallback(() => {
        return JSON.stringify({ env: envConfig }, null, 2);
    }, [envConfig]);

    const generateSettingsScriptWindows = React.useCallback(() => {
        const nodeCode = generateNodeScript('.claude/settings.json', envConfig);
        return `# PowerShell - Run in PowerShell
@"
${nodeCode}
"@ | node`;
    }, [envConfig]);

    const generateSettingsScriptUnix = React.useCallback(() => {
        const nodeCode = generateNodeScript('.claude/settings.json', envConfig);
        return `# Bash - Run in terminal
node -e '${nodeCode.replace(/'/g, "'\\''")}'`;
    }, [envConfig]);

    const generateClaudeJsonConfig = React.useCallback(() => {
        return JSON.stringify(claudeJsonConfig, null, 2);
    }, [claudeJsonConfig]);

    const generateScriptWindows = React.useCallback(() => {
        const nodeCode = generateNodeScript('.claude.json', claudeJsonConfig);
        return `# PowerShell - Run in PowerShell
@"
${nodeCode}
"@ | node`;
    }, [claudeJsonConfig]);

    const generateScriptUnix = React.useCallback(() => {
        const nodeCode = generateNodeScript('.claude.json', claudeJsonConfig);
        return `# Bash - Run in terminal
node -e '${nodeCode.replace(/'/g, "'\\''")}'`;
    }, [claudeJsonConfig]);

    const generateStatusLineConfig = React.useCallback(() => {
        return JSON.stringify({
            statusLine: { type: 'command', command: '~/.claude/tingly-statusline.sh' },
        }, null, 2);
    }, []);

    const generateStatusLineScriptWindows = React.useCallback(() => {
        const downloadUrl = "https://github.com/your-repo/tingly-statusline/raw/main/tingly-statusline.ps1";
        const nodeCode = `const fs = require("fs");
const path = require("path");
const os = require("os");
const https = require("https");

const homeDir = os.homedir();
const statusLineDir = path.join(homeDir, ".claude", "scripts");
const statusLinePath = path.join(statusLineDir, "tingly-statusline.ps1");

if (!fs.existsSync(statusLineDir)) {
    fs.mkdirSync(statusLineDir, { recursive: true });
}

const file = fs.createWriteStream(statusLinePath);
https.get("${downloadUrl}", (response) => {
    response.pipe(file);
    file.on('finish', () => {
        file.close();
        console.log("Status line script installed to:", statusLinePath);
        console.log("Add this to your PowerShell profile:\\n. ~/.claude/scripts/tingly-statusline.ps1");
    });
}).on('error', (err) => {
    fs.unlink(statusLinePath, () => {});
    console.error("Error downloading status line script:", err.message);
});`;
        return `# PowerShell - Run in PowerShell
@"
${nodeCode}
"@ | node`;
    }, []);

    const generateStatusLineScriptUnix = React.useCallback(() => {
        const downloadUrl = "https://github.com/your-repo/tingly-statusline/raw/main/tingly-statusline.sh";
        const nodeCode = `const fs = require("fs");
const path = require("path");
const os = require("os");
const https = require("https");

const homeDir = os.homedir();
const statusLineDir = path.join(homeDir, ".claude", "scripts");
const statusLinePath = path.join(statusLineDir, "tingly-statusline.sh");

if (!fs.existsSync(statusLineDir)) {
    fs.mkdirSync(statusLineDir, { recursive: true });
}

const file = fs.createWriteStream(statusLinePath);
https.get("${downloadUrl}", (response) => {
    response.pipe(file);
    file.on('finish', () => {
        file.close();
        fs.chmodSync(statusLinePath, '755');
        console.log("Status line script installed to:", statusLinePath);
        console.log("Add this to your shell profile (~/.bashrc, ~/.zshrc, etc.):\\nsource ~/.claude/scripts/tingly-statusline.sh");
    });
}).on('error', (err) => {
    fs.unlink(statusLinePath, () => {});
    console.error("Error downloading status line script:", err.message);
});`;
        return `# Bash - Run in terminal
node -e '${nodeCode.replace(/'/g, "'\\''")}'`;
    }, []);

    const handleApply = async (installStatusLine: boolean) => {
        if (!onApplyWithPrefs) return;
        const result = await onApplyWithPrefs(prefs, installStatusLine);
        setApplyResult(result);
    };

    const canApply = isFullEdition && !!onApplyWithPrefs;

    return (
        <>
            <Dialog
                open={open}
                onClose={(_event, reason) => {
                    if (reason === 'backdropClick' || reason === 'escapeKeyDown') return;
                    onClose();
                }}
                maxWidth="lg"
                fullWidth
                disableEscapeKeyDown
                PaperProps={{ sx: { borderRadius: 3, maxHeight: '90vh' } }}
            >
                <DialogTitle sx={{ pb: 1, borderBottom: 1, borderColor: 'divider' }}>
                    <Typography variant="h6" fontWeight={600}>
                        {t('claudeCode.modal.title')}
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                        {t('claudeCode.modal.subtitle')}
                    </Typography>
                    <Tabs
                        value={mainTab}
                        onChange={(_, v) => setMainTab(v)}
                        sx={{ mt: 1.5, minHeight: 36, '& .MuiTab-root': { minHeight: 36, py: 0.5, textTransform: 'none' } }}
                    >
                        <Tab label={modalText.tabQuick} value="quick" />
                        <Tab label={modalText.tabManual} value="manual" />
                    </Tabs>
                </DialogTitle>

                <DialogContent sx={{ p: 3 }}>
                    {applyResult && (
                        <Alert
                            severity={applyResult.success ? 'success' : 'error'}
                            sx={{ mb: 2 }}
                            action={
                                <IconButton size="small" onClick={() => setApplyResult(null)} aria-label="dismiss">
                                    <CloseIcon fontSize="small" />
                                </IconButton>
                            }
                        >
                            <AlertTitle sx={{ mb: applyResult.success ? 0.5 : 0 }}>
                                {applyResult.success ? modalText.applySuccess : modalText.applyFailure}
                            </AlertTitle>
                            {applyResult.success ? (
                                <Box sx={{ fontSize: '0.8rem' }}>
                                    {(applyResult.createdFiles?.length ?? 0) > 0 && (
                                        <Box sx={{ mt: 0.5 }}>
                                            <Typography variant="caption" sx={{ fontWeight: 600 }}>{modalText.createdLabel}:</Typography>
                                            {applyResult.createdFiles!.map(f => (
                                                <Typography key={f} variant="caption" sx={{ display: 'block', fontFamily: 'monospace', pl: 1 }}>{f}</Typography>
                                            ))}
                                        </Box>
                                    )}
                                    {(applyResult.updatedFiles?.length ?? 0) > 0 && (
                                        <Box sx={{ mt: 0.5 }}>
                                            <Typography variant="caption" sx={{ fontWeight: 600 }}>{modalText.updatedLabel}:</Typography>
                                            {applyResult.updatedFiles!.map(f => (
                                                <Typography key={f} variant="caption" sx={{ display: 'block', fontFamily: 'monospace', pl: 1 }}>{f}</Typography>
                                            ))}
                                        </Box>
                                    )}
                                    {(applyResult.backupPaths?.length ?? 0) > 0 && (
                                        <Box sx={{ mt: 0.5 }}>
                                            <Typography variant="caption" sx={{ fontWeight: 600 }}>{modalText.backupLabel}:</Typography>
                                            {applyResult.backupPaths!.map(f => (
                                                <Typography key={f} variant="caption" sx={{ display: 'block', fontFamily: 'monospace', pl: 1, color: 'text.secondary' }}>{f}</Typography>
                                            ))}
                                        </Box>
                                    )}
                                </Box>
                            ) : (
                                <Typography variant="body2">{applyResult.error}</Typography>
                            )}
                        </Alert>
                    )}

                    {mainTab === 'quick' && (
                        <ClaudeCodeQuickConfig
                            prefs={prefs}
                            setPrefs={setPrefsAndClearResult}
                            onResetDefaults={() => setPrefsAndClearResult(derivePrefsFromRules({ rules, mode: configMode }))}
                        />
                    )}

                    {mainTab === 'manual' && (
                        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
                            {/* settings.json section */}
                            <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                                <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                                    <Typography variant="subtitle2" color="text.secondary">
                                        {t('claudeCode.step1')}
                                    </Typography>
                                    <Tabs
                                        value={settingsTab}
                                        onChange={(_, value) => setSettingsTab(value)}
                                        variant="standard"
                                        sx={{ minHeight: 32, '& .MuiTabs-indicator': { height: 3 } }}
                                    >
                                        <Tab label="JSON" value="json" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                        <Tab label="Windows" value="windows" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                        <Tab label="Linux/macOS" value="unix" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    </Tabs>
                                </Box>
                                <Box>
                                    {settingsTab === 'json' && (
                                        <CodeBlock
                                            code={generateSettingsConfig()}
                                            language="json"
                                            filename="Add the env section into ~/.claude/settings.json"
                                            wrap={true}
                                            onCopy={(code) => copyToClipboard(code, 'settings.json')}
                                            maxHeight={280}
                                            minHeight={280}
                                        />
                                    )}
                                    {settingsTab === 'windows' && (
                                        <CodeBlock
                                            code={generateSettingsScriptWindows()}
                                            language="js"
                                            filename="PowerShell script to setup ~/.claude/settings.json"
                                            wrap={true}
                                            onCopy={(code) => copyToClipboard(code, 'Windows script')}
                                            maxHeight={280}
                                            minHeight={280}
                                        />
                                    )}
                                    {settingsTab === 'unix' && (
                                        <CodeBlock
                                            code={generateSettingsScriptUnix()}
                                            language="js"
                                            filename="Bash script to setup ~/.claude/settings.json"
                                            wrap={true}
                                            onCopy={(code) => copyToClipboard(code, 'Unix script')}
                                            maxHeight={280}
                                            minHeight={280}
                                        />
                                    )}
                                </Box>
                            </Box>

                            {/* .claude.json section */}
                            <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                                <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                                    <Typography variant="subtitle2" color="text.secondary">
                                        {t('claudeCode.step2')}
                                    </Typography>
                                    <Tabs
                                        value={claudeJsonTab}
                                        onChange={(_, value) => setClaudeJsonTab(value)}
                                        variant="standard"
                                        sx={{ minHeight: 32, '& .MuiTabs-indicator': { height: 3 } }}
                                    >
                                        <Tab label="JSON" value="json" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                        <Tab label="Windows" value="windows" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                        <Tab label="Linux/macOS" value="unix" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    </Tabs>
                                </Box>
                                <Box>
                                    {claudeJsonTab === 'json' && (
                                        <CodeBlock
                                            code={generateClaudeJsonConfig()}
                                            language="json"
                                            filename="Set hasCompletedOnboarding as true into ~/.claude.json"
                                            wrap={true}
                                            onCopy={(code) => copyToClipboard(code, '.claude.json')}
                                            maxHeight={120}
                                            minHeight={80}
                                        />
                                    )}
                                    {claudeJsonTab === 'windows' && (
                                        <CodeBlock
                                            code={generateScriptWindows()}
                                            language="js"
                                            filename="PowerShell script to setup ~/.claude.json"
                                            wrap={true}
                                            onCopy={(code) => copyToClipboard(code, 'Windows script')}
                                            maxHeight={120}
                                            minHeight={80}
                                        />
                                    )}
                                    {claudeJsonTab === 'unix' && (
                                        <CodeBlock
                                            code={generateScriptUnix()}
                                            language="js"
                                            filename="Bash script to setup ~/.claude.json"
                                            wrap={true}
                                            onCopy={(code) => copyToClipboard(code, 'Unix script')}
                                            maxHeight={120}
                                            minHeight={80}
                                        />
                                    )}
                                </Box>
                            </Box>

                            {/* Status Line section */}
                            <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                                <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                                    <Typography variant="subtitle2" color="text.secondary">
                                        {t('claudeCode.step3')}
                                    </Typography>
                                    <Tabs
                                        value={statusLineTab}
                                        onChange={(_, value) => setStatusLineTab(value)}
                                        variant="standard"
                                        sx={{ minHeight: 32, '& .MuiTabs-indicator': { height: 3 } }}
                                    >
                                        <Tab label="JSON" value="json" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                        <Tab label="Windows" value="windows" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                        <Tab label="Linux/macOS" value="unix" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    </Tabs>
                                </Box>
                                <Box>
                                    {statusLineTab === 'json' && (
                                        <>
                                            <Box sx={{ mb: 2 }}>
                                                <Typography variant="body2" sx={{ mb: 1 }}>
                                                    {t('claudeCode.statusLine.jsonDescription')}
                                                </Typography>
                                                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                                                    {t('claudeCode.statusLine.addToSettingsJson')}
                                                </Typography>
                                                <Typography variant="body2" color="text.secondary">
                                                    {t('claudeCode.statusLine.manualSetup')}{' '}
                                                    <Link
                                                        href="https://raw.githubusercontent.com/tingly-dev/tingly-box/refs/heads/main/internal/script/tingly-statusline.sh"
                                                        target="_blank"
                                                        rel="noopener noreferrer"
                                                    >
                                                        {t('claudeCode.statusLine.downloadLink')}
                                                    </Link>
                                                </Typography>
                                            </Box>
                                            <CodeBlock
                                                code={generateStatusLineConfig()}
                                                language="json"
                                                filename="Add statusLine config to ~/.claude/settings.json"
                                                wrap={true}
                                                onCopy={(code) => copyToClipboard(code, 'statusLine config')}
                                                maxHeight={200}
                                                minHeight={150}
                                            />
                                        </>
                                    )}
                                    {(statusLineTab === 'windows' || statusLineTab === 'unix') && (
                                        <Box sx={{ mb: 2 }}>
                                            <Typography variant="body2" sx={{ mb: 1 }}>
                                                {t('claudeCode.statusLine.description')}
                                            </Typography>
                                            <Typography variant="body2" color="text.secondary">
                                                {t('claudeCode.statusLine.manualSetup')}{' '}
                                                <Link
                                                    href="https://raw.githubusercontent.com/tingly-dev/tingly-box/refs/heads/main/internal/script/tingly-statusline.sh"
                                                    target="_blank"
                                                    rel="noopener noreferrer"
                                                >
                                                    {t('claudeCode.statusLine.downloadLink')}
                                                </Link>
                                            </Typography>
                                        </Box>
                                    )}
                                    {statusLineTab === 'windows' && (
                                        <CodeBlock
                                            code={generateStatusLineScriptWindows()}
                                            language="js"
                                            filename="PowerShell script to install status line"
                                            wrap={true}
                                            onCopy={(code) => copyToClipboard(code, 'Status line script')}
                                            maxHeight={280}
                                            minHeight={280}
                                        />
                                    )}
                                    {statusLineTab === 'unix' && (
                                        <CodeBlock
                                            code={generateStatusLineScriptUnix()}
                                            language="js"
                                            filename="Bash script to install status line"
                                            wrap={true}
                                            onCopy={(code) => copyToClipboard(code, 'Status line script')}
                                            maxHeight={280}
                                            minHeight={280}
                                        />
                                    )}
                                </Box>
                            </Box>
                        </Box>
                    )}
                </DialogContent>

                <DialogActions sx={{ px: 3, pb: 2, pt: 1, gap: 1, justifyContent: 'space-between' }}>
                    <Button
                        onClick={() => setPreviewOpen(true)}
                        size="small"
                        startIcon={<VisibilityOutlinedIcon />}
                        sx={{ textTransform: 'none', color: 'text.secondary' }}
                    >
                        {modalText.previewButton}
                    </Button>
                    <Box sx={{ display: 'flex', gap: 1 }}>
                        <Button onClick={onClose} color="inherit">
                            {t('common.cancel')}
                        </Button>
                        {canApply && (
                            <>
                                <Button
                                    onClick={() => handleApply(false)}
                                    variant="outlined"
                                    disabled={isApplyLoading}
                                    startIcon={isApplyLoading ? <CircularProgress size={14} /> : null}
                                >
                                    {t('claudeCode.quickApply')}
                                </Button>
                                <Button
                                    onClick={() => handleApply(true)}
                                    variant="contained"
                                    disabled={isApplyLoading}
                                    startIcon={isApplyLoading ? <CircularProgress size={14} color="inherit" /> : null}
                                >
                                    {t('claudeCode.quickApplyWithStatusLine')}
                                </Button>
                            </>
                        )}
                    </Box>
                </DialogActions>
            </Dialog>

            {/* Preview dialog: shows the exact env block the backend will write */}
            <Dialog
                open={previewOpen}
                onClose={() => setPreviewOpen(false)}
                maxWidth="md"
                fullWidth
                PaperProps={{ sx: { borderRadius: 3 } }}
            >
                <DialogTitle>
                    <Typography variant="subtitle1" fontWeight={600}>{modalText.previewTitle}</Typography>
                </DialogTitle>
                <DialogContent>
                    <CodeBlock
                        code={JSON.stringify({ env: envConfig }, null, 2)}
                        language="json"
                        filename="settings.json env preview"
                        wrap={true}
                        onCopy={(code) => copyToClipboard(code, 'env preview')}
                        maxHeight={480}
                        minHeight={200}
                    />
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setPreviewOpen(false)}>{t('common.cancel')}</Button>
                </DialogActions>
            </Dialog>
        </>
    );
};

export default ClaudeCodeConfigModal;
