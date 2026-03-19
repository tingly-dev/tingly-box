import { Box, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, Link, Tab, Tabs, Typography } from '@mui/material';
import React, { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import CodeBlock from './CodeBlock';
import { isFullEdition } from '@/utils/edition';

type ConfigMode = 'unified' | 'separate' | 'smart';

interface ClaudeCodeConfigModalProps {
    open: boolean;
    onClose: () => void;
    configMode: ConfigMode;
    baseUrl: string;
    token: string;
    rules: any[];
    copyToClipboard: (text: string, label: string) => Promise<void>;
    // Apply handlers
    onApply?: () => Promise<void>;
    onApplyWithStatusLine?: () => Promise<void>;
    isApplyLoading?: boolean;
}

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

const ClaudeCodeConfigModal: React.FC<ClaudeCodeConfigModalProps> = ({
    open,
    onClose,
    configMode,
    baseUrl,
    token,
    rules,
    copyToClipboard,
    onApply,
    onApplyWithStatusLine,
    isApplyLoading = false,
}) => {
    const { t } = useTranslation();
    const [settingsTab, setSettingsTab] = React.useState<ScriptTab>('json');
    const [claudeJsonTab, setClaudeJsonTab] = React.useState<ScriptTab>('json');
    const [statusLineTab, setStatusLineTab] = React.useState<ScriptTab>('json');

    // Memoized configuration generators
    const {
        claudeCodeBaseUrl,
        modelForVariant,
        subagentModel,
        settingsEnvConfig,
        claudeJsonConfig,
    } = React.useMemo(() => {
        const claudeCodeBaseUrl = `${baseUrl}/tingly/claude_code`;

        const getModelForVariant = (variant: string): string => {
            if (configMode === 'unified') {
                return rules[0]?.request_model || '';
            }
            const rule = rules.find((r: any) => r?.uuid === `built-in-cc-${variant}`);
            return rule?.request_model || '';
        };

        const subagentModel = configMode === 'unified'
            ? (rules[0]?.request_model || '')
            : (getModelForVariant('subagent') || 'tingly/cc-subagent');

        // Generate env config for settings.json
        const settingsEnvConfig = configMode === 'unified'
            ? {
                ANTHROPIC_MODEL: rules[0]?.request_model,
                ANTHROPIC_DEFAULT_HAIKU_MODEL: rules[0]?.request_model,
                ANTHROPIC_DEFAULT_OPUS_MODEL: rules[0]?.request_model,
                ANTHROPIC_DEFAULT_SONNET_MODEL: rules[0]?.request_model,
                CLAUDE_CODE_SUBAGENT_MODEL: subagentModel,
                DISABLE_TELEMETRY: "1",
                DISABLE_ERROR_REPORTING: "1",
                CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
                API_TIMEOUT_MS: "3000000",
                ANTHROPIC_AUTH_TOKEN: token,
                ANTHROPIC_BASE_URL: claudeCodeBaseUrl,
            }
            : {
                ANTHROPIC_MODEL: getModelForVariant('default'),
                ANTHROPIC_DEFAULT_HAIKU_MODEL: getModelForVariant('haiku'),
                ANTHROPIC_DEFAULT_OPUS_MODEL: getModelForVariant('opus'),
                ANTHROPIC_DEFAULT_SONNET_MODEL: getModelForVariant('sonnet'),
                CLAUDE_CODE_SUBAGENT_MODEL: subagentModel,
                DISABLE_TELEMETRY: "1",
                DISABLE_ERROR_REPORTING: "1",
                CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
                API_TIMEOUT_MS: "3000000",
                ANTHROPIC_AUTH_TOKEN: token,
                ANTHROPIC_BASE_URL: claudeCodeBaseUrl,
            };

        const claudeJsonConfig = { hasCompletedOnboarding: true };

        return { claudeCodeBaseUrl, modelForVariant: getModelForVariant, subagentModel, settingsEnvConfig, claudeJsonConfig };
    }, [configMode, baseUrl, token, rules]);

    // Generate settings.json content
    const generateSettingsConfig = React.useCallback(() => {
        return JSON.stringify({ env: settingsEnvConfig }, null, 2);
    }, [settingsEnvConfig]);

    // Generate settings.json scripts
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

    // Generate .claude.json content
    const generateClaudeJsonConfig = React.useCallback(() => {
        return JSON.stringify(claudeJsonConfig, null, 2);
    }, [claudeJsonConfig]);

    // Generate .claude.json scripts
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

    // Status line config (TODO: implement when script is ready)
    const generateStatusLineConfig = React.useCallback(() => {
        const scriptPath = '~/.claude/tingly-statusline.sh';
        return JSON.stringify({
            statusLine: {
                type: 'command',
                command: scriptPath
            }
        }, null, 2);
    }, []);

    // Status line scripts (WIP - placeholder download URLs)
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

    const handleApplyClick = () => {
        if (onApply) {
            onApply();
        }
    };

    const handleApplyWithStatusLineClick = () => {
        if (onApplyWithStatusLine) {
            onApplyWithStatusLine();
        }
    };

    return (
        <Dialog
            open={open}
            onClose={(event, reason) => {
                // Only allow closing via the confirm button, not backdrop click or ESC
                if (reason === 'backdropClick' || reason === 'escapeKeyDown') {
                    return;
                }
                onClose();
            }}
            maxWidth="lg"
            fullWidth
            disableEscapeKeyDown
            PaperProps={{
                sx: {
                    borderRadius: 3,
                    maxHeight: '90vh',
                }
            }}
        >
            <DialogTitle sx={{
                pb: 1,
                borderBottom: 1,
                borderColor: 'divider',
            }}>
                <Typography variant="h6" fontWeight={600}>
                    {t('claudeCode.modal.title')}
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                    {t('claudeCode.modal.subtitle')}
                </Typography>
            </DialogTitle>

            <DialogContent sx={{ p: 3 }}>
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
                    {/* Settings.json section */}
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
                                    // bash, but use js for highlight
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
                                    // bash, but use js for highlight
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
                                    // bash, but use js for highlight
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
                                    // bash, but use js for highlight
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
                    {(generateStatusLineConfig || generateStatusLineScriptWindows || generateStatusLineScriptUnix) && (
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
                                    {generateStatusLineConfig && (
                                        <Tab label="JSON" value="json" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    )}
                                    {generateStatusLineScriptWindows && (
                                        <Tab label="Windows" value="windows" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    )}
                                    {generateStatusLineScriptUnix && (
                                        <Tab label="Linux/macOS" value="unix" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    )}
                                </Tabs>
                            </Box>
                            <Box>
                                {statusLineTab === 'json' && generateStatusLineConfig && (
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
                                {statusLineTab === 'windows' && generateStatusLineScriptWindows && (
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
                                {statusLineTab === 'unix' && generateStatusLineScriptUnix && (
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
                    )}
                </Box>
            </DialogContent>

            <DialogActions sx={{ px: 3, pb: 2, pt: 1, gap: 1, justifyContent: 'flex-end', flexWrap: 'wrap' }}>
                <Button onClick={onClose} color="inherit">
                    {t('common.cancel')}
                </Button>
                {/* Hide Apply buttons in lite edition */}
                {isFullEdition && onApply && (
                    <Button
                        onClick={handleApplyClick}
                        variant="contained"
                        disabled={isApplyLoading}
                        startIcon={isApplyLoading ? <CircularProgress size={16} color="inherit" /> : null}
                    >
                        {t('claudeCode.quickApply')}
                    </Button>
                )}
                {isFullEdition && onApplyWithStatusLine && (
                    <Button
                        onClick={handleApplyWithStatusLineClick}
                        variant="contained"
                        disabled={isApplyLoading}
                        startIcon={isApplyLoading ? <CircularProgress size={16} color="inherit" /> : null}
                    >
                        {t('claudeCode.quickApplyWithStatusLine')}
                    </Button>
                )}
            </DialogActions>
        </Dialog>
    );
};

export default ClaudeCodeConfigModal;
