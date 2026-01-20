import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import {
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    ToggleButton,
    ToggleButtonGroup,
    Typography
} from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import ClaudeCodeConfigModal from '../components/ClaudeCodeConfigModal';
import EmptyStateGuide from '../components/EmptyStateGuide';
import PageLayout from '../components/PageLayout';
import TemplatePage from '../components/TemplatePage.tsx';
import { FEATURE_FLAGS, isFeatureEnabled } from '../constants/featureFlags';
import { useFunctionPanelData } from '../hooks/useFunctionPanelData';
import { api, getBaseUrl } from '../services/api';

type ConfigMode = 'unified' | 'separate' | 'smart';

const MODEL_VARIANTS = ['default', 'haiku', 'sonnet', 'opus'] as const;

// Configuration mode options
const CONFIG_MODES: { value: ConfigMode; label: string; description: string; enabled: boolean }[] = [
    { value: 'unified', label: 'Unified', description: 'Single model for all requests', enabled: true },
    { value: 'separate', label: 'Separate', description: 'Distinct models for each variant', enabled: true },
    { value: 'smart', label: 'Smart', description: '(WIP) Smart routing according to request field / content / model feature / user intent / ...', enabled: false },
];

const UseClaudeCodePage: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const {
        showTokenModal,
        setShowTokenModal,
        token,
        showNotification,
        providers,
    } = useFunctionPanelData();
    const [baseUrl, setBaseUrl] = React.useState<string>('');
    const [rules, setRules] = React.useState<any[]>([]);
    const [loadingRule, setLoadingRule] = React.useState(true);
    const [isDockerMode, setIsDockerMode] = React.useState(false);
    const [configMode, setConfigMode] = React.useState<ConfigMode>('unified');
    const [pendingMode, setPendingMode] = React.useState<ConfigMode | null>(null);
    const [confirmDialogOpen, setConfirmDialogOpen] = React.useState(false);

    // Claude Code config modal state
    const [configModalOpen, setConfigModalOpen] = React.useState(false);
    const [dontRemindAgain, setDontRemindAgain] = React.useState(false);

    // localStorage key for "do not remind again"
    const CLAUDE_CODE_DONT_REMIND_KEY = 'claudeCode_dontRemindAgain';

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

                // Reset "do not remind again" and show modal with new configuration
                localStorage.setItem(CLAUDE_CODE_DONT_REMIND_KEY, 'false');
                setDontRemindAgain(false);
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

    // Handle "do not remind again" checkbox change
    const handleDontRemindChange = (checked: boolean) => {
        setDontRemindAgain(checked);
        localStorage.setItem(CLAUDE_CODE_DONT_REMIND_KEY, String(checked));
    };

    // Show config guide modal (manual trigger) - user wants to be reminded again
    const handleShowConfigGuide = () => {
        // Reset "do not remind again" since user is manually requesting the guide
        setDontRemindAgain(false);
        localStorage.setItem(CLAUDE_CODE_DONT_REMIND_KEY, 'false');
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

    const loadData = async () => {
        const url = await getBaseUrl();
        setBaseUrl(url);
        setLoadingRule(true);

        if (configMode === 'unified') {
            const result = await api.getRule("built-in-cc");
            setRules(result.success ? [result.data] : []);
        } else {
            // Load separate rules for each model variant
            const loadedRules = await Promise.all(
                MODEL_VARIANTS.map(async (variant) => {
                    const result = await api.getRule(`built-in-cc-${variant}`);
                    return result.success ? result.data : null;
                })
            );
            setRules(loadedRules.filter((r): r is any => r !== null));
        }

        setLoadingRule(false);
    };

    React.useEffect(() => {
        loadScenarioConfig();
    }, []);

    React.useEffect(() => {
        loadData();
    }, [configMode]);

    const toDockerUrl = (url: string): string => {
        return url.replace(/\/\/([^/:]+)(?::(\d+))?/, '//host.docker.internal:$2');
    };

    const getClaudeCodeBaseUrl = () => {
        const url = `${baseUrl}/tingly/claude_code`;
        return isDockerMode ? toDockerUrl(url) : url;
    };

    // Get model name for each variant
    const getModelForVariant = (variant: string): string => {
        if (configMode === 'unified') {
            return rules[0]?.request_model;
        }
        const rule = rules.find(r => r?.uuid === `built-in-cc-${variant}`);
        return rule?.request_model || '';
    };

    const generateSettingsConfig = () => {
        const claudeCodeBaseUrl = getClaudeCodeBaseUrl();

        if (configMode === 'unified') {
            const model = rules[0]?.request_model;
            return JSON.stringify({
                env: {
                    ANTHROPIC_MODEL: model,
                    ANTHROPIC_DEFAULT_HAIKU_MODEL: model,
                    ANTHROPIC_DEFAULT_OPUS_MODEL: model,
                    ANTHROPIC_DEFAULT_SONNET_MODEL: model,
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
            }
            : {
                ANTHROPIC_MODEL: getModelForVariant('default'),
                ANTHROPIC_DEFAULT_HAIKU_MODEL: getModelForVariant('haiku'),
                ANTHROPIC_DEFAULT_OPUS_MODEL: getModelForVariant('opus'),
                ANTHROPIC_DEFAULT_SONNET_MODEL: getModelForVariant('sonnet'),
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
    DISABLE_TELEMETRY: "1",
    DISABLE_ERROR_REPORTING: "1",
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
    API_TIMEOUT_MS: "3000000",
    ANTHROPIC_AUTH_TOKEN: "${token}",
};

let existingSettings = {};
if (fs.existsSync(settingsPath)) {
    const content = fs.readFileSync(settingsPath, "utf-8");
    existingSettings = JSON.parse(content);
}

const newSettings = { ...existingSettings, env: envConfig };
fs.writeFileSync(settingsPath, JSON.stringify(newSettings, null, 2));
console.log("Settings written to", settingsPath);`;

        return `# PowerShell - Run in PowerShell
node -e @"
${nodeCode}
"@`;
    };

    const generateSettingsScriptUnix = () => {
        const claudeCodeBaseUrl = getClaudeCodeBaseUrl();

        const commonEnv = configMode === 'unified'
            ? {
                ANTHROPIC_MODEL: rules[0]?.request_model,
                ANTHROPIC_DEFAULT_HAIKU_MODEL: rules[0]?.request_model,
                ANTHROPIC_DEFAULT_OPUS_MODEL: rules[0]?.request_model,
                ANTHROPIC_DEFAULT_SONNET_MODEL: rules[0]?.request_model,
            }
            : {
                ANTHROPIC_MODEL: getModelForVariant('default'),
                ANTHROPIC_DEFAULT_HAIKU_MODEL: getModelForVariant('haiku'),
                ANTHROPIC_DEFAULT_OPUS_MODEL: getModelForVariant('opus'),
                ANTHROPIC_DEFAULT_SONNET_MODEL: getModelForVariant('sonnet'),
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
    DISABLE_TELEMETRY: "1",
    DISABLE_ERROR_REPORTING: "1",
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
    API_TIMEOUT_MS: "3000000",
    ANTHROPIC_AUTH_TOKEN: "${token}",
};

let existingSettings = {};
if (fs.existsSync(settingsPath)) {
    const content = fs.readFileSync(settingsPath, "utf-8");
    existingSettings = JSON.parse(content);
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
    existingConfig = JSON.parse(content);
}

const newConfig = { ...existingConfig, ...onboardingConfig };
fs.writeFileSync(claudeJsonPath, JSON.stringify(newConfig, null, 2));
console.log("Onboarding config written to", claudeJsonPath);`;

        return `# PowerShell - Run in PowerShell
node -e @"
${nodeCode}
"@`;
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
    existingConfig = JSON.parse(content);
}

const newConfig = { ...existingConfig, ...onboardingConfig };
fs.writeFileSync(claudeJsonPath, JSON.stringify(newConfig, null, 2));
console.log("Onboarding config written to", claudeJsonPath);`;

        return `# Bash - Run in terminal
node -e '${nodeCode.replace(/'/g, "'\\''")}'`;
    };


    return (
        <PageLayout loading={loadingRule}>
            {
                !providers.length
                    ?
                    // Show empty state if no providers
                    <PageLayout loading={loadingRule}>
                        <CardGrid>
                            <UnifiedCard title="Use Claude Code" size="full">
                                <EmptyStateGuide
                                    title="No Providers Configured"
                                    description="Add an API key or OAuth provider to get started"
                                    onAddApiKeyClick={handleAddApiKeyClick}
                                    onAddOAuthClick={handleAddOAuthClick}
                                />
                            </UnifiedCard>
                        </CardGrid>
                    </PageLayout>
                    :
                    <CardGrid>
                        <UnifiedCard
                            title="Use Claude Code"
                            size="full"
                            rightAction={
                                <Button
                                    onClick={handleShowConfigGuide}
                                    variant="contained"
                                    color="primary"
                                    size="small"
                                    sx={{ fontSize: '0.875rem' }}
                                >
                                    {t('claudeCode.modal.showGuide')}
                                </Button>
                            }
                        >
                            <Box sx={{ mb: 2 }}>
                                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                                    Configure Claude Code to use Tingly Box as your AI model proxy
                                </Typography>
                                {CONFIG_MODES.filter(m => m.enabled).map((mode) => (
                                    <Box key={mode.value} sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
                                        <Typography variant="body2" color="text.secondary">
                                            <Box component="span" sx={{ fontWeight: 600, color: 'primary.main' }}>
                                                {mode.label}:
                                            </Box> {mode.description}
                                        </Typography>
                                    </Box>
                                ))}
                            </Box>

                            {/* Mode switch - controlled by feature flag */}
                            {isFeatureEnabled(FEATURE_FLAGS.CLAUDE_CODE_MODE_SWITCH) && (
                                <>
                                    <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', py: 2 }}>
                                        {/*<Typography variant="h4" color="text.secondary">*/}
                                        {/*    Configuration Mode*/}
                                        {/*</Typography>*/}
                                        <ToggleButtonGroup
                                            value={configMode}
                                            exclusive
                                            size="small"
                                            onChange={(_, value) => value && handleConfigModeChange(value)}
                                            sx={{
                                                bgcolor: 'action.hover',
                                                '& .MuiToggleButton-root': {
                                                    color: 'text.primary',
                                                    padding: '4px 12px',
                                                    fontSize: '0.875rem',
                                                    '&:hover': {
                                                        bgcolor: 'action.selected',
                                                    },
                                                },
                                            }}
                                        >
                                            {CONFIG_MODES.filter(m => m.enabled).map((mode) => (
                                                <ToggleButton
                                                    key={mode.value}
                                                    value={mode.value}
                                                    sx={{
                                                        '&.Mui-selected': {
                                                            bgcolor: 'primary.main',
                                                            color: 'white',
                                                            '&:hover': {
                                                                bgcolor: 'primary.dark',
                                                            },
                                                        },
                                                    }}
                                                >
                                                    {mode.label}
                                                </ToggleButton>
                                            ))}
                                        </ToggleButtonGroup>
                                    </Box>
                                </>
                            )}
                        </UnifiedCard>



                        <TemplatePage
                            rules={rules}
                            showTokenModal={showTokenModal}
                            setShowTokenModal={setShowTokenModal}
                            token={token}
                            showNotification={showNotification}
                            providers={providers}
                            onRulesChange={setRules}
                            allowToggleRule={false}
                            collapsible={false}
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
                            <DialogActions sx={{ px: 3, pb: 2 }}>
                                <Button onClick={cancelModeChange} color="inherit">
                                    Cancel
                                </Button>
                                <Button onClick={confirmModeChange} variant="contained" color="primary">
                                    Confirm
                                </Button>
                            </DialogActions>
                        </Dialog>

                        {/* Claude Code Config Modal */}
                        <ClaudeCodeConfigModal
                            open={configModalOpen}
                            onClose={() => setConfigModalOpen(false)}
                            dontRemindAgain={dontRemindAgain}
                            onDontRemindChange={handleDontRemindChange}
                            configMode={configMode}
                            generateSettingsConfig={generateSettingsConfig}
                            generateSettingsScriptWindows={generateSettingsScriptWindows}
                            generateSettingsScriptUnix={generateSettingsScriptUnix}
                            generateClaudeJsonConfig={generateClaudeJsonConfig}
                            generateScriptWindows={generateScriptWindows}
                            generateScriptUnix={generateScriptUnix}
                            copyToClipboard={copyToClipboard}
                        />
                    </CardGrid>
            }
        </PageLayout>
    );
};

export default UseClaudeCodePage;
