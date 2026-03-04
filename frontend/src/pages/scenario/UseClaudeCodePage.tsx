import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import {
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
    Typography,
    IconButton
} from '@mui/material';
import InfoOutlined from '@mui/icons-material/InfoOutlined';
import InfoIcon from '@mui/icons-material/Info';
import React, { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import ClaudeCodeConfigModal from '@/components/ClaudeCodeConfigModal';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import PageLayout from '@/components/PageLayout';
import TemplatePage from '@/components/TemplatePage.tsx';
import { useFunctionPanelData } from '@/hooks/useFunctionPanelData';
import { useHeaderHeight } from '@/hooks/useHeaderHeight';
import { api, getBaseUrl } from '@/services/api';
import { toggleButtonGroupStyle, toggleButtonStyle } from "@/styles/toggleStyles";

type ConfigMode = 'unified' | 'separate' | 'smart';

const MODEL_VARIANTS = ['default', 'haiku', 'sonnet', 'opus', 'subagent'] as const;

// Configuration mode options
const CONFIG_MODES: { value: ConfigMode; label: string; description: string; enabled: boolean }[] = [
    { value: 'unified', label: 'Unified', description: 'Single model for all requests', enabled: true },
    { value: 'separate', label: 'Separate', description: 'Distinct models for each variant', enabled: true },
    { value: 'smart', label: 'Smart', description: '(WIP) Smart routing according to request field / content / model feature / user intent / ...', enabled: false },
];

const UseClaudeCodePage: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const headerRef = useRef<HTMLDivElement>(null);
    const {
        showTokenModal,
        setShowTokenModal,
        token,
        showNotification,
        providers,
        loading: providersLoading,
        notification,
    } = useFunctionPanelData();
    const [baseUrl, setBaseUrl] = React.useState<string>('');
    const [rules, setRules] = React.useState<any[]>([]);
    const [loadingRule, setLoadingRule] = React.useState(true);
    const [configMode, setConfigMode] = React.useState<ConfigMode>('unified');
    const [pendingMode, setPendingMode] = React.useState<ConfigMode | null>(null);
    const [confirmDialogOpen, setConfirmDialogOpen] = React.useState(false);

    // Claude Code config modal state
    const [configModalOpen, setConfigModalOpen] = React.useState(false);
    const [isApplyLoading, setIsApplyLoading] = React.useState(false);

    // Use shared hook for header height measurement
    const headerHeight = useHeaderHeight(
        headerRef,
        providers.length > 0,
        [configMode]
    );

    const handleAddApiKeyClick = () => {
        navigate('/api-keys?dialog=add');
    };

    const handleAddOAuthClick = () => {
        navigate('/oauth?dialog=add');
    };

    // Load scenario config to get config mode
    const loadScenarioConfig = async () => {
        try {
            const result = await api.getScenarioConfig('claude_code');
            if (result.success && result.data && result.data.flags) {
                const { unified, separate, smart } = result.data.flags;
                if (separate) {
                    setConfigMode('separate');
                } else {
                    setConfigMode('unified');
                }
            }
        } catch (error) {
            console.error('Failed to load scenario config:', error);
        }
    };

    // Handle config mode change - show confirmation dialog first
    const handleConfigModeChange = (newMode: ConfigMode) => {
        if (newMode === configMode) return;
        setPendingMode(newMode);
        setConfirmDialogOpen(true);
    };

    // Confirm mode change
    const confirmModeChange = async () => {
        if (!pendingMode) return;

        setConfirmDialogOpen(false);
        try {
            const config = {
                scenario: 'claude_code',
                flags: {
                    unified: pendingMode === 'unified',
                    separate: pendingMode === 'separate',
                    smart: false,
                },
            };
            const result = await api.setScenarioConfig('claude_code', config);

            if (result.success) {
                setConfigMode(pendingMode);
                setConfigModalOpen(true);

                showNotification(
                    `Configuration mode changed to ${pendingMode}. Please reapply the configuration to Claude Code.`,
                    'success'
                );
            } else {
                showNotification('Failed to save configuration mode', 'error');
            }
        } catch (error) {
            console.error('Failed to save scenario config:', error);
            showNotification('Failed to save configuration mode', 'error');
        } finally {
            setPendingMode(null);
        }
    };

    // Cancel mode change
    const cancelModeChange = () => {
        setConfirmDialogOpen(false);
        setPendingMode(null);
    };

    // Show config guide modal
    const handleShowConfigGuide = () => {
        setConfigModalOpen(true);
    };

    const copyToClipboard = async (text: string, label: string) => {
        try {
            await navigator.clipboard.writeText(text);
            showNotification(`${label} copied to clipboard!`, 'success');
        } catch (err) {
            showNotification('Failed to copy to clipboard', 'error');
        }
    };

    useEffect(() => {
        let isMounted = true;

        const loadDataAsync = async () => {
            const url = await getBaseUrl();
            if (isMounted) setBaseUrl(url);

            setLoadingRule(true);
            if (configMode === 'unified') {
                const result = await api.getRule("built-in-cc");
                if (isMounted) {
                    setRules(result.success ? [result.data] : []);
                    setLoadingRule(false);
                }
            } else {
                // Load separate rules for each model variant
                const loadedRules = await Promise.all(
                    MODEL_VARIANTS.map(async (variant) => {
                        const result = await api.getRule(`built-in-cc-${variant}`);
                        return result.success ? result.data : null;
                    })
                );
                if (isMounted) {
                    setRules(loadedRules.filter((r): r is any => r !== null));
                    setLoadingRule(false);
                }
            }
        };

        loadDataAsync();

        return () => {
            isMounted = false;
        };
    }, [configMode]);

    useEffect(() => {
        loadScenarioConfig();
    }, []);

    const getClaudeCodeBaseUrl = () => {
        const url = `${baseUrl}/tingly/claude_code`;
        return url;
    };

    // Get model name for each variant
    const getModelForVariant = (variant: string): string => {
        if (configMode === 'unified') {
            return rules[0]?.request_model;
        }
        const rule = rules.find(r => r?.uuid === `built-in-cc-${variant}`);
        return rule?.request_model || '';
    };

    const getSubagentModel = (): string => {
        return configMode === 'unified'
            ? (rules[0]?.request_model || '')
            : (getModelForVariant('subagent') || 'tingly/cc-subagent');
    };

    // Generate settings.json JSON (from backend)
    const generateSettingsConfig = () => {
        const claudeCodeBaseUrl = getClaudeCodeBaseUrl();
        const subagentModel = getSubagentModel();

        if (configMode === 'unified') {
            const model = rules[0]?.request_model;
            return JSON.stringify({
                env: {
                    ANTHROPIC_MODEL: model,
                    ANTHROPIC_DEFAULT_HAIKU_MODEL: model,
                    ANTHROPIC_DEFAULT_OPUS_MODEL: model,
                    ANTHROPIC_DEFAULT_SONNET_MODEL: model,
                    CLAUDE_CODE_SUBAGENT_MODEL: subagentModel,
                    DISABLE_TELEMETRY: "1",
                    DISABLE_ERROR_REPORTING: "1",
                    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
                    API_TIMEOUT_MS: "3000000",
                    ANTHROPIC_AUTH_TOKEN: token,
                    ANTHROPIC_BASE_URL: claudeCodeBaseUrl,
                },
            }, null, 2);
        } else {
            return JSON.stringify({
                env: {
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
                },
            }, null, 2);
        }
    };

    const generateSettingsScriptWindows = () => {
        const claudeCodeBaseUrl = getClaudeCodeBaseUrl();

        const commonEnv = configMode === 'unified'
            ? {
                ANTHROPIC_MODEL: rules[0]?.request_model,
                ANTHROPIC_DEFAULT_HAIKU_MODEL: rules[0]?.request_model,
                ANTHROPIC_DEFAULT_OPUS_MODEL: rules[0]?.request_model,
                ANTHROPIC_DEFAULT_SONNET_MODEL: rules[0]?.request_model,
                CLAUDE_CODE_SUBAGENT_MODEL: rules[0]?.request_model,
            }
            : {
                ANTHROPIC_MODEL: getModelForVariant('default'),
                ANTHROPIC_DEFAULT_HAIKU_MODEL: getModelForVariant('haiku'),
                ANTHROPIC_DEFAULT_OPUS_MODEL: getModelForVariant('opus'),
                ANTHROPIC_DEFAULT_SONNET_MODEL: getModelForVariant('sonnet'),
                CLAUDE_CODE_SUBAGENT_MODEL: getSubagentModel(),
            };

        const nodeCode = `const fs = require("fs");
const path = require("path");
const os = require("os");

const homeDir = os.homedir();
const settingsPath = path.join(homeDir, ".claude", "settings.json");
const claudeDir = path.join(homeDir, ".claude");
if (!fs.existsSync(claudeDir)) {
    fs.mkdirSync(claudeDir, { recursive: true });
}

const envConfig = {
    ANTHROPIC_BASE_URL: "${claudeCodeBaseUrl}",
    ANTHROPIC_MODEL: "${commonEnv.ANTHROPIC_MODEL}",
    ANTHROPIC_DEFAULT_HAIKU_MODEL: "${commonEnv.ANTHROPIC_DEFAULT_HAIKU_MODEL}",
    ANTHROPIC_DEFAULT_OPUS_MODEL: "${commonEnv.ANTHROPIC_DEFAULT_OPUS_MODEL}",
    ANTHROPIC_DEFAULT_SONNET_MODEL: "${commonEnv.ANTHROPIC_DEFAULT_SONNET_MODEL}",
    CLAUDE_CODE_SUBAGENT_MODEL: "${commonEnv.CLAUDE_CODE_SUBAGENT_MODEL}",
    DISABLE_TELEMETRY: "1",
    DISABLE_ERROR_REPORTING: "1",
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
    API_TIMEOUT_MS: "3000000",
    ANTHROPIC_AUTH_TOKEN: "${token}",
};

let existingSettings = {};
if (fs.existsSync(settingsPath)) {
    const content = fs.readFileSync(settingsPath, "utf-8");
    try { existingSettings = JSON.parse(content); } catch (e) {}
}

const newSettings = { ...existingSettings, env: envConfig };
fs.writeFileSync(settingsPath, JSON.stringify(newSettings, null, 2));
console.log("Settings written to", settingsPath);`;

        return `# PowerShell - Run in PowerShell
@"
${nodeCode}
"@ | node`;
    };

    const generateSettingsScriptUnix = () => {
        const claudeCodeBaseUrl = getClaudeCodeBaseUrl();

        const commonEnv = configMode === 'unified'
            ? {
                ANTHROPIC_MODEL: rules[0]?.request_model,
                ANTHROPIC_DEFAULT_HAIKU_MODEL: rules[0]?.request_model,
                ANTHROPIC_DEFAULT_OPUS_MODEL: rules[0]?.request_model,
                ANTHROPIC_DEFAULT_SONNET_MODEL: rules[0]?.request_model,
                CLAUDE_CODE_SUBAGENT_MODEL: rules[0]?.request_model,
            }
            : {
                ANTHROPIC_MODEL: getModelForVariant('default'),
                ANTHROPIC_DEFAULT_HAIKU_MODEL: getModelForVariant('haiku'),
                ANTHROPIC_DEFAULT_OPUS_MODEL: getModelForVariant('opus'),
                ANTHROPIC_DEFAULT_SONNET_MODEL: getModelForVariant('sonnet'),
                CLAUDE_CODE_SUBAGENT_MODEL: getSubagentModel(),
            };

        const nodeCode = `const fs = require("fs");
const path = require("path");
const os = require("os");

const homeDir = os.homedir();
const settingsPath = path.join(homeDir, ".claude", "settings.json");
const claudeDir = path.join(homeDir, ".claude");
if (!fs.existsSync(claudeDir)) {
    fs.mkdirSync(claudeDir, { recursive: true });
}

const envConfig = {
    ANTHROPIC_BASE_URL: "${claudeCodeBaseUrl}",
    ANTHROPIC_MODEL: "${commonEnv.ANTHROPIC_MODEL}",
    ANTHROPIC_DEFAULT_HAIKU_MODEL: "${commonEnv.ANTHROPIC_DEFAULT_HAIKU_MODEL}",
    ANTHROPIC_DEFAULT_OPUS_MODEL: "${commonEnv.ANTHROPIC_DEFAULT_OPUS_MODEL}",
    ANTHROPIC_DEFAULT_SONNET_MODEL: "${commonEnv.ANTHROPIC_DEFAULT_SONNET_MODEL}",
    CLAUDE_CODE_SUBAGENT_MODEL: "${commonEnv.CLAUDE_CODE_SUBAGENT_MODEL}",
    DISABLE_TELEMETRY: "1",
    DISABLE_ERROR_REPORTING: "1",
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
    API_TIMEOUT_MS: "3000000",
    ANTHROPIC_AUTH_TOKEN: "${token}",
};

let existingSettings = {};
if (fs.existsSync(settingsPath)) {
    const content = fs.readFileSync(settingsPath, "utf-8");
    try { existingSettings = JSON.parse(content); } catch (e) {}
}

const newSettings = { ...existingSettings, env: envConfig };
fs.writeFileSync(settingsPath, JSON.stringify(newSettings, null, 2));
console.log("Settings written to", settingsPath);`;

        return `# Bash - Run in terminal
node -e '${nodeCode.replace(/'/g, "'\\''")}'`;
    };

    const generateClaudeJsonConfig = () => {
        return JSON.stringify({
            hasCompletedOnboarding: true
        }, null, 2);
    };

    const generateScriptWindows = () => {
        const nodeCode = `const fs = require("fs");
const path = require("path");
const os = require("os");

const homeDir = os.homedir();
const claudeJsonPath = path.join(homeDir, ".claude.json");

const onboardingConfig = {
    hasCompletedOnboarding: true
};

let existingConfig = {};
if (fs.existsSync(claudeJsonPath)) {
    const content = fs.readFileSync(claudeJsonPath, "utf-8");
    try { existingConfig = JSON.parse(content); } catch (e) {}
}

const newConfig = { ...existingConfig, ...onboardingConfig };
fs.writeFileSync(claudeJsonPath, JSON.stringify(newConfig, null, 2));
console.log("Onboarding config written to", claudeJsonPath);`;

        return `# PowerShell - Run in PowerShell
@"
${nodeCode}
"@ | node`;
    };

    const generateScriptUnix = () => {
        const nodeCode = `const fs = require("fs");
const path = require("path");
const os = require("os");

const homeDir = os.homedir();
const claudeJsonPath = path.join(homeDir, ".claude.json");

const onboardingConfig = {
    hasCompletedOnboarding: true
};

let existingConfig = {};
if (fs.existsSync(claudeJsonPath)) {
    const content = fs.readFileSync(claudeJsonPath, "utf-8");
    try { existingConfig = JSON.parse(content); } catch (e) {}
}

const newConfig = { ...existingConfig, ...onboardingConfig };
fs.writeFileSync(claudeJsonPath, JSON.stringify(newConfig, null, 2));
console.log("Onboarding config written to", claudeJsonPath);`;

        return `# Bash - Run in terminal
node -e '${nodeCode.replace(/'/g, "'\\''")}'`;
    };

    // Status line JSON config
    const generateStatusLineConfig = () => {
        // Default to Unix path for JSON config (user can adjust for Windows)
        const scriptPath = '~/.claude/tingly-statusline.sh';

        return JSON.stringify({
            statusLine: {
                type: 'command',
                command: scriptPath
            }
        }, null, 2);
    };

    // Status line scripts - downloads and installs the status line integration
    const generateStatusLineScriptWindows = () => {
        // TODO: Replace with actual download URL
        const downloadUrl = "https://github.com/your-repo/tingly-statusline/raw/main/tingly-statusline.ps1";

        const nodeCode = `const fs = require("fs");
const path = require("path");
const os = require("os");
const https = require("https");

const homeDir = os.homedir();
const statusLineDir = path.join(homeDir, ".claude", "scripts");
const statusLinePath = path.join(statusLineDir, "tingly-statusline.ps1");

// Create directory if it doesn't exist
if (!fs.existsSync(statusLineDir)) {
    fs.mkdirSync(statusLineDir, { recursive: true });
}

// Download the status line script
const file = fs.createWriteStream(statusLinePath);
https.get("${downloadUrl}", (response) => {
    response.pipe(file);
    file.on('finish', () => {
        file.close();
        console.log("Status line script installed to:", statusLinePath);
        console.log("Add this to your PowerShell profile:");
        console.log("\\n. ~/.claude/scripts/tingly-statusline.ps1");
    });
}).on('error', (err) => {
    fs.unlink(statusLinePath, () => {});
    console.error("Error downloading status line script:", err.message);
});`;

        return `# PowerShell - Run in PowerShell
# This will download and install the status line script
@"
${nodeCode}
"@ | node`;
    };

    const generateStatusLineScriptUnix = () => {
        // TODO: Replace with actual download URL
        const downloadUrl = "https://github.com/your-repo/tingly-statusline/raw/main/tingly-statusline.sh";

        const nodeCode = `const fs = require("fs");
const path = require("path");
const os = require("os");
const https = require("https");

const homeDir = os.homedir();
const statusLineDir = path.join(homeDir, ".claude", "scripts");
const statusLinePath = path.join(statusLineDir, "tingly-statusline.sh");

// Create directory if it doesn't exist
if (!fs.existsSync(statusLineDir)) {
    fs.mkdirSync(statusLineDir, { recursive: true });
}

// Download the status line script
const file = fs.createWriteStream(statusLinePath);
https.get("${downloadUrl}", (response) => {
    response.pipe(file);
    file.on('finish', () => {
        file.close();
        fs.chmodSync(statusLinePath, '755');
        console.log("Status line script installed to:", statusLinePath);
        console.log("Add this to your shell profile (~/.bashrc, ~/.zshrc, etc.):");
        console.log("\\nsource ~/.claude/scripts/tingly-statusline.sh");
    });
}).on('error', (err) => {
    fs.unlink(statusLinePath, () => {});
    console.error("Error downloading status line script:", err.message);
});`;

        return `# Bash - Run in terminal
# This will download and install the status line script
node -e '${nodeCode.replace(/'/g, "'\\''")}'`;
    };

    // Apply handler - calls backend to generate and write config
    const handleApply = async () => {
        try {
            setIsApplyLoading(true);
            const result = await api.applyClaudeConfig(configMode, false);

            if (result.success) {
                // Build success message from backend response
                const createdFiles = result.createdFiles || [];
                const updatedFiles = result.updatedFiles || [];
                const backupPaths = result.backupPaths || [];

                const allFiles = [...createdFiles, ...updatedFiles];
                let successMsg = `Configuration files written: ${allFiles.join(', ')}`;
                if (backupPaths.length > 0) {
                    successMsg += `\nBackups created: ${backupPaths.join(', ')}`;
                }
                showNotification(successMsg, 'success');
            } else {
                showNotification(`Failed to apply configurations: ${result.message || 'Unknown error'}`, 'error');
            }
        } catch (err) {
            showNotification('Failed to apply configurations', 'error');
        } finally {
            setIsApplyLoading(false);
        }
    };

    // Apply handler with status line
    const handleApplyWithStatusLine = async () => {
        try {
            setIsApplyLoading(true);
            const result = await api.applyClaudeConfig(configMode, true);

            if (result.success) {
                // Build success message from backend response
                const createdFiles = result.createdFiles || [];
                const updatedFiles = result.updatedFiles || [];
                const backupPaths = result.backupPaths || [];

                const allFiles = [...createdFiles, ...updatedFiles];
                let successMsg = `Configuration files written: ${allFiles.join(', ')}`;
                if (backupPaths.length > 0) {
                    successMsg += `\nBackups created: ${backupPaths.join(', ')}`;
                }
                showNotification(successMsg, 'success');
            } else {
                showNotification(`Failed to apply configurations: ${result.message || 'Unknown error'}`, 'error');
            }
        } catch (err) {
            showNotification('Failed to apply configurations', 'error');
        } finally {
            setIsApplyLoading(false);
        }
    };

    const isLoading = providersLoading || loadingRule;

    // Mode selection component
    const modeSelection = (
        <Box sx={{
            display: 'flex',
            alignItems: 'center',
            py: 2,
            gap: 3,
        }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 180 }}>
                <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                    Mode
                </Typography>
                <Tooltip
                    title={
                        <>
                            Unified: Single model for all requests
                            <br />
                            Separate: Distinct models for each variant
                        </>
                    }
                    arrow
                >
                    <InfoOutlined sx={{ fontSize: '1rem', color: 'text.secondary', cursor: 'help' }} />
                </Tooltip>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center', flex: 1 }}>
                <ToggleButtonGroup
                    value={configMode}
                    exclusive
                    size="small"
                    onChange={(_, value) => value && handleConfigModeChange(value)}
                    sx={toggleButtonGroupStyle}
                >
                    {CONFIG_MODES.filter(m => m.enabled).map((mode) => (
                        <Tooltip key={mode.value} title={mode.description} arrow>
                            <ToggleButton
                                value={mode.value}
                                sx={toggleButtonStyle}
                            >
                                {mode.label}
                            </ToggleButton>
                        </Tooltip>
                    ))}
                </ToggleButtonGroup>
            </Box>
        </Box>
    );

    return (
        <PageLayout loading={isLoading} notification={notification}>
            {!providers.length ? (
                <CardGrid>
                    <UnifiedCard
                        title={
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <span>Claude Code SDK Configuration</span>
                            </Box>
                        }
                        size="full"
                    >
                        <EmptyStateGuide
                            title="No Providers Configured"
                            description="Add an API key or OAuth provider to get started"
                            onAddApiKeyClick={handleAddApiKeyClick}
                            onAddOAuthClick={handleAddOAuthClick}
                        />
                    </UnifiedCard>
                </CardGrid>
            ) : (
                <CardGrid>
                    <UnifiedCard
                        title={
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <span>Claude Code SDK Configuration</span>
                                <Tooltip title={`Base URL: ${baseUrl}/tingly/claude_code`}>
                                    <IconButton size="small" sx={{ ml: 0.5 }}>
                                        <InfoIcon fontSize="small" sx={{ color: 'text.secondary' }} />
                                    </IconButton>
                                </Tooltip>
                            </Box>
                        }
                        size="full"
                        rightAction={
                            <Button
                                onClick={handleShowConfigGuide}
                                variant="contained"
                                color="primary"
                                size="small"
                            >
                                {t('claudeCode.configButton')}
                            </Button>
                        }
                    >
                        <ProviderConfigCard
                            headerRef={headerRef}
                            title="Claude Code SDK Configuration"
                            baseUrlPath="/tingly/claude_code"
                            baseUrl={baseUrl}
                            onCopy={copyToClipboard}
                            token={token}
                            onShowTokenModal={() => setShowTokenModal(true)}
                            scenario="claude_code"
                            modeSelection={modeSelection}
                            showApiKeyRow={true}
                            showBaseUrlRow={true}
                        />
                    </UnifiedCard>

                    <TemplatePage
                        title="Models and Forwarding Rules"
                        scenario="claude_code"
                        rules={rules}
                        showTokenModal={showTokenModal}
                        setShowTokenModal={setShowTokenModal}
                        token={token}
                        showNotification={showNotification}
                        providers={providers}
                        onRulesChange={setRules}
                        allowToggleRule={false}
                        collapsible={true}
                        showAddApiKeyButton={false}
                        showCreateRuleButton={false}
                        headerHeight={headerHeight}
                    />

                    {/* Confirmation dialog for mode change */}
                    <Dialog
                        open={confirmDialogOpen}
                        onClose={cancelModeChange}
                        maxWidth="sm"
                        fullWidth
                    >
                        <DialogTitle>Change Configuration Mode?</DialogTitle>
                        <DialogContent>
                            <Typography variant="body1" sx={{ mb: 1 }}>
                                You are about to switch from <strong>{configMode}</strong> to <strong>{pendingMode}</strong> mode.
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                                After changing the mode, you will need to reapply the configuration to Claude Code for the changes to take effect.
                            </Typography>
                        </DialogContent>
                        <DialogActions sx={{ px: 3, pb: 2, gap: 1, justifyContent: 'flex-end' }}>
                            <Button onClick={cancelModeChange} color="inherit">
                                Cancel
                            </Button>
                            <Button onClick={confirmModeChange} variant="contained">
                                Confirm
                            </Button>
                        </DialogActions>
                    </Dialog>

                    {/* Claude Code Config Modal */}
                    <ClaudeCodeConfigModal
                        open={configModalOpen}
                        onClose={() => {
                            setConfigModalOpen(false);
                        }}
                        configMode={configMode}
                        generateSettingsConfig={generateSettingsConfig}
                        generateSettingsScriptWindows={generateSettingsScriptWindows}
                        generateSettingsScriptUnix={generateSettingsScriptUnix}
                        generateClaudeJsonConfig={generateClaudeJsonConfig}
                        generateScriptWindows={generateScriptWindows}
                        generateScriptUnix={generateScriptUnix}
                        generateStatusLineConfig={generateStatusLineConfig}
                        generateStatusLineScriptWindows={generateStatusLineScriptWindows}
                        generateStatusLineScriptUnix={generateStatusLineScriptUnix}
                        copyToClipboard={copyToClipboard}
                        onApply={handleApply}
                        onApplyWithStatusLine={handleApplyWithStatusLine}
                        isApplyLoading={isApplyLoading}
                    />
                </CardGrid>
            )}
        </PageLayout>
    );
};

export default UseClaudeCodePage;
