import { Box, Button, CircularProgress, Collapse, Dialog, DialogActions, DialogContent, DialogTitle, Link, Tab, Tabs, Typography } from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import React from 'react';
import { useTranslation } from 'react-i18next';
import CodeBlock from './CodeBlock';
import { isFullEdition } from '@/utils/edition';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';
import ClaudeCodeQuickConfig, { derivePrefsFromRules } from './ClaudeCodeQuickConfig';
import type { ClaudeCodePrefs } from './ClaudeCodeQuickConfig';

type ConfigMode = 'unified' | 'separate' | 'smart';

interface ClaudeCodeConfigModalProps {
    open: boolean;
    onClose: () => void;
    configMode: ConfigMode;
    baseUrl: string;
    rules: any[];
    copyToClipboard: (text: string, label: string) => Promise<void>;
    // Legacy auto-apply (uses configMode to derive defaults server-side)
    onApply?: () => Promise<void>;
    onApplyWithStatusLine?: () => Promise<void>;
    // Quick-config apply (sends user-edited prefs)
    onApplyWithPrefs?: (prefs: ClaudeCodePrefs, installStatusLine: boolean) => Promise<void>;
    isApplyLoading?: boolean;
}

type MainTab = 'quick' | 'manual' | 'auto';
type ScriptTab = 'json' | 'windows' | 'unix';

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

// Strip the keys that are server-injected (base URL, auth token) — they
// shouldn't be exposed in the quick-config form but ARE part of the env
// map shown in the manual tab.
const stripServerKeys = (env: Record<string, string | undefined>): Record<string, string> => {
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(env)) {
        if (v === undefined || v === '') continue;
        out[k] = v;
    }
    return out;
};

const ClaudeCodeConfigModal: React.FC<ClaudeCodeConfigModalProps> = ({
    open,
    onClose,
    configMode,
    baseUrl,
    rules,
    copyToClipboard,
    onApply,
    onApplyWithStatusLine,
    onApplyWithPrefs,
    isApplyLoading = false,
}) => {
    const { token } = useScenarioPageModal();
    const { t } = useTranslation();
    const [mainTab, setMainTab] = React.useState<MainTab>('quick');
    const [settingsTab, setSettingsTab] = React.useState<ScriptTab>('json');
    const [claudeJsonTab, setClaudeJsonTab] = React.useState<ScriptTab>('json');
    const [statusLineTab, setStatusLineTab] = React.useState<ScriptTab>('json');
    const [showPreview, setShowPreview] = React.useState(false);

    // Quick-config form state. Re-seed when the underlying rules/mode change
    // (e.g. user picks a different mode in the parent) — but only if the
    // modal isn't currently open, so we don't clobber unsaved edits.
    const [prefs, setPrefs] = React.useState<ClaudeCodePrefs>(() =>
        derivePrefsFromRules({ rules, mode: configMode })
    );
    React.useEffect(() => {
        if (!open) {
            setPrefs(derivePrefsFromRules({ rules, mode: configMode }));
        }
    }, [open, configMode, rules]);

    const claudeCodeBaseUrl = `${baseUrl}/tingly/claude_code`;

    // Legacy mode-based env map — drives the manual tab so users see the
    // exact bytes the auto-apply would write. Manual tab content does NOT
    // reflect quick-config edits; that's a deliberate split so manual is
    // a clean, reproducible reference.
    const settingsEnvConfig = React.useMemo(() => {
        const getModelForVariant = (variant: string): string => {
            if (configMode === 'unified') return rules[0]?.request_model || '';
            const rule = rules.find((r: any) => r?.uuid === `built-in-cc-${variant}`);
            return rule?.request_model || '';
        };
        const subagentModel = configMode === 'unified'
            ? (rules[0]?.request_model || '')
            : (getModelForVariant('subagent') || 'tingly/cc-subagent');
        const base = {
            DISABLE_TELEMETRY: "1",
            DISABLE_ERROR_REPORTING: "1",
            CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
            API_TIMEOUT_MS: "3000000",
            ANTHROPIC_AUTH_TOKEN: token,
            ANTHROPIC_BASE_URL: claudeCodeBaseUrl,
            CLAUDE_CODE_SUBAGENT_MODEL: subagentModel,
        };
        if (configMode === 'unified') {
            const m = rules[0]?.request_model;
            return {
                ANTHROPIC_MODEL: m,
                ANTHROPIC_DEFAULT_HAIKU_MODEL: m,
                ANTHROPIC_DEFAULT_OPUS_MODEL: m,
                ANTHROPIC_DEFAULT_SONNET_MODEL: m,
                ...base,
            };
        }
        return {
            ANTHROPIC_MODEL: getModelForVariant('default'),
            ANTHROPIC_DEFAULT_HAIKU_MODEL: getModelForVariant('haiku'),
            ANTHROPIC_DEFAULT_OPUS_MODEL: getModelForVariant('opus'),
            ANTHROPIC_DEFAULT_SONNET_MODEL: getModelForVariant('sonnet'),
            ...base,
        };
    }, [configMode, claudeCodeBaseUrl, token, rules]);

    const claudeJsonConfig = { hasCompletedOnboarding: true };

    // ── Preview env map for quick-config tab ──────────────────────────
    // Mirrors what the backend will write: merge user prefs with the
    // server-injected base URL + auth token so the preview is honest.
    const quickPreviewEnv = React.useMemo(() => {
        return stripServerKeys({
            ...prefs,
            ANTHROPIC_BASE_URL: claudeCodeBaseUrl,
            ANTHROPIC_AUTH_TOKEN: token,
        });
    }, [prefs, claudeCodeBaseUrl, token]);

    const generateSettingsConfig = React.useCallback(() => {
        return JSON.stringify({ env: settingsEnvConfig }, null, 2);
    }, [settingsEnvConfig]);

    const generateSettingsScriptWindows = React.useCallback(() => {
        const nodeCode = generateNodeScript('.claude/settings.json', settingsEnvConfig);
        return `# PowerShell - Run in PowerShell
@"
${nodeCode}
"@ | node`;
    }, [settingsEnvConfig]);

    const generateSettingsScriptUnix = React.useCallback(() => {
        const nodeCode = generateNodeScript('.claude/settings.json', settingsEnvConfig);
        return `# Bash - Run in terminal
node -e '${nodeCode.replace(/'/g, "'\\''")}'`;
    }, [settingsEnvConfig]);

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
        const scriptPath = '~/.claude/tingly-statusline.sh';
        return JSON.stringify({
            statusLine: {
                type: 'command',
                command: scriptPath
            }
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

    const handleQuickApply = async (installStatusLine: boolean) => {
        if (onApplyWithPrefs) {
            await onApplyWithPrefs(prefs, installStatusLine);
        }
    };

    return (
        <Dialog
            open={open}
            onClose={(event, reason) => {
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
                    <Tab label="快速配置" value="quick" />
                    <Tab label="手动配置" value="manual" />
                    {isFullEdition && (onApply || onApplyWithStatusLine) && (
                        <Tab label="自动配置" value="auto" />
                    )}
                </Tabs>
            </DialogTitle>

            <DialogContent sx={{ p: 3 }}>
                {mainTab === 'quick' && (
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                        <ClaudeCodeQuickConfig
                            prefs={prefs}
                            setPrefs={setPrefs}
                            onApply={handleQuickApply}
                            onResetDefaults={() => setPrefs(derivePrefsFromRules({ rules, mode: configMode }))}
                            isApplyLoading={isApplyLoading}
                            showApply={isFullEdition && !!onApplyWithPrefs}
                        />

                        <Box>
                            <Button
                                size="small"
                                onClick={() => setShowPreview(s => !s)}
                                endIcon={showPreview ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                                sx={{ textTransform: 'none', color: 'text.secondary' }}
                            >
                                {showPreview ? '收起' : '查看'}最终写入 settings.json 的 env
                            </Button>
                            <Collapse in={showPreview}>
                                <Box sx={{ mt: 1 }}>
                                    <CodeBlock
                                        code={JSON.stringify({ env: quickPreviewEnv }, null, 2)}
                                        language="json"
                                        filename="预览 ~/.claude/settings.json 中的 env 段"
                                        wrap={true}
                                        onCopy={(code) => copyToClipboard(code, 'settings.json env preview')}
                                        maxHeight={320}
                                        minHeight={120}
                                    />
                                </Box>
                            </Collapse>
                        </Box>
                    </Box>
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

                {mainTab === 'auto' && isFullEdition && (
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, py: 2 }}>
                        <Typography variant="body2">
                            一键应用 tb 推荐的默认配置到 <code>~/.claude/settings.json</code> 和 <code>~/.claude.json</code>。
                            模型槽位根据当前 <strong>{configMode}</strong> 模式自动选择，其他 env 使用 tb 推荐默认值。
                            如需自定义，请切换到「快速配置」tab。
                        </Typography>
                        <Box sx={{ display: 'flex', gap: 1, mt: 1 }}>
                            {onApply && (
                                <Button
                                    onClick={onApply}
                                    variant="outlined"
                                    disabled={isApplyLoading}
                                    startIcon={isApplyLoading ? <CircularProgress size={14} /> : null}
                                >
                                    {t('claudeCode.quickApply')}
                                </Button>
                            )}
                            {onApplyWithStatusLine && (
                                <Button
                                    onClick={onApplyWithStatusLine}
                                    variant="contained"
                                    disabled={isApplyLoading}
                                    startIcon={isApplyLoading ? <CircularProgress size={14} color="inherit" /> : null}
                                >
                                    {t('claudeCode.quickApplyWithStatusLine')}
                                </Button>
                            )}
                        </Box>
                    </Box>
                )}
            </DialogContent>

            <DialogActions sx={{ px: 3, pb: 2, pt: 1, justifyContent: 'flex-end' }}>
                <Button onClick={onClose} color="inherit">
                    {t('common.cancel')}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default ClaudeCodeConfigModal;
