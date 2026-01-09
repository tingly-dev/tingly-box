import { Box, Typography, ToggleButton, ToggleButtonGroup, Dialog, DialogTitle, DialogContent, DialogActions, Button } from '@mui/material';
import React from 'react';
import CodeBlock from '../components/CodeBlock';
import TemplatePage from '../components/TemplatePage.tsx';
import PageLayout from '../components/PageLayout';
import { api, getBaseUrl } from '../services/api';
import type { Provider } from '../types/provider';
import { useTranslation } from 'react-i18next';
import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import { useFunctionPanelData } from '../hooks/useFunctionPanelData';
import { useProviderDialog } from '../hooks/useProviderDialog';
import EmptyStateGuide from '../components/EmptyStateGuide';
import ProviderFormDialog from '../components/ProviderFormDialog';
import OAuthDialog from '../components/OAuthDialog';

type ClaudeJsonMode = 'json' | 'script';
type ConfigMode = 'unified' | 'separate';

const MODEL_VARIANTS = ['default', 'haiku', 'sonnet', 'opus'] as const;

// Configuration mode options
const CONFIG_MODES: { value: ConfigMode; label: string }[] = [
    { value: 'unified', label: 'Unified' },
    { value: 'separate', label: 'Separate' },
];

const UseClaudeCodePage: React.FC = () => {
    const { t } = useTranslation();
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
    const [claudeJsonMode, setClaudeJsonMode] = React.useState<ClaudeJsonMode>('script');
    const [configMode, setConfigMode] = React.useState<ConfigMode>('unified');
    const [pendingMode, setPendingMode] = React.useState<ConfigMode | null>(null);
    const [confirmDialogOpen, setConfirmDialogOpen] = React.useState(false);

    // Provider dialog hook
    const providerDialog = useProviderDialog(showNotification, {
        defaultApiStyle: 'anthropic',
        onProviderAdded: () => window.location.reload(),
    });

    // OAuth dialog state
    const [oauthDialogOpen, setOAuthDialogOpen] = React.useState(false);

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
        const rule = rules.find(r => r?.id === `built-in-cc-${variant}`);
        return rule?.request_model!;
    };

    const generateSettingsConfig = () => {
        const claudeCodeBaseUrl = getClaudeCodeBaseUrl();

        if (configMode === 'unified') {
            const model = getModelForVariant('unified');
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

    const generateSettingsScript = () => {
        const claudeCodeBaseUrl = getClaudeCodeBaseUrl();

        if (configMode === 'unified') {
            const model = getModelForVariant('unified');
            return `# Configure Claude Code settings
echo "Configuring Claude Code settings..."
mkdir -p ~/.claude
node --eval '
    const fs = require("fs");
    const path = require("path");
    const homeDir = os.homedir();
    const settingsPath = path.join(homeDir, ".claude", "settings.json");
    const env = {
        ANTHROPIC_BASE_URL: "${claudeCodeBaseUrl}",
        ANTHROPIC_MODEL: "${model}",
        ANTHROPIC_DEFAULT_HAIKU_MODEL: "${model}",
        ANTHROPIC_DEFAULT_OPUS_MODEL: "${model}",
        ANTHROPIC_DEFAULT_SONNET_MODEL: "${model}",
        DISABLE_TELEMETRY: "1",
        DISABLE_ERROR_REPORTING: "1",
        CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
        API_TIMEOUT_MS: "3000000",
        ANTHROPIC_AUTH_TOKEN: "${token}",
    };
    if (fs.existsSync(settingsPath)) {
        const content = JSON.parse(fs.readFileSync(settingsPath, "utf-8"));
        fs.writeFileSync(settingsPath, JSON.stringify({ ...content, env }, null, 2), "utf-8");
    } else {
        fs.writeFileSync(settingsPath, JSON.stringify({ env }, null, 2), "utf-8");
    }
'`;
        } else {
            // Get model values before building the template string
            const defaultModel = getModelForVariant('default');
            const haikuModel = getModelForVariant('haiku');
            const opusModel = getModelForVariant('opus');
            const sonnetModel = getModelForVariant('sonnet');

            return `# Configure Claude Code settings
echo "Configuring Claude Code settings..."
mkdir -p ~/.claude
node --eval '
    const fs = require("fs");
    const path = require("path");
    const homeDir = os.homedir();
    const settingsPath = path.join(homeDir, ".claude", "settings.json");
    const env = {
        ANTHROPIC_BASE_URL: "${claudeCodeBaseUrl}",
        ANTHROPIC_MODEL: "${defaultModel}",
        ANTHROPIC_DEFAULT_HAIKU_MODEL: "${haikuModel}",
        ANTHROPIC_DEFAULT_OPUS_MODEL: "${opusModel}",
        ANTHROPIC_DEFAULT_SONNET_MODEL: "${sonnetModel}",
        DISABLE_TELEMETRY: "1",
        DISABLE_ERROR_REPORTING: "1",
        CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
        API_TIMEOUT_MS: "3000000",
        ANTHROPIC_AUTH_TOKEN: "${token}",
    };
    if (fs.existsSync(settingsPath)) {
        const content = JSON.parse(fs.readFileSync(settingsPath, "utf-8"));
        fs.writeFileSync(settingsPath, JSON.stringify({ ...content, env }, null, 2), "utf-8");
    } else {
        fs.writeFileSync(settingsPath, JSON.stringify({ env }, null, 2), "utf-8");
    }
'`;
        }
    };

    const generateClaudeJsonConfig = () => {
        return JSON.stringify({
            hasCompletedOnboarding: true
        }, null, 2);
    };

    const generateScript = () => {
        return `# Configure Claude Code to skip onboarding
echo "Configuring Claude Code to skip onboarding..."
node --eval '
    const homeDir = os.homedir();
    const filePath = path.join(homeDir, ".claude.json");
    if (fs.existsSync(filePath)) {
        const content = JSON.parse(fs.readFileSync(filePath, "utf-8"));
        fs.writeFileSync(filePath, JSON.stringify({ ...content, hasCompletedOnboarding: true }, null, 2), "utf-8");
    } else {
        fs.writeFileSync(filePath, JSON.stringify({ hasCompletedOnboarding: true }, null, 2), "utf-8");
    }'`;
    };

    const header = (
        <Box sx={{ p: 2, display: 'flex', flexDirection: { xs: 'column', md: 'row' }, gap: 2, alignItems: 'stretch' }}>
            {/* Settings.json section */}
            <Box sx={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
                    <Box sx={{ mb: 1 }}>
                        <Typography variant="subtitle2" color="text.secondary">
                            {t('claudeCode.step1')}
                        </Typography>
                    </Box>
                    <Box sx={{ flex: 1, height: 400 }}>
                        <CodeBlock
                            code={claudeJsonMode === 'json' ? generateSettingsConfig() : generateSettingsScript()}
                            language={claudeJsonMode === 'json' ? 'json' : 'js'}
                            filename={claudeJsonMode === 'json' ? 'Add the env section into ~/.claude/setting.json' : 'Script to setup ~/.claude/settings.json'}
                            wrap={true}
                            onCopy={(code) => copyToClipboard(code, claudeJsonMode === 'json' ? 'settings.json' : 'script')}
                            maxHeight={180}
                            minHeight={180}
                            headerActions={
                                <ToggleButtonGroup
                                    value={claudeJsonMode}
                                    exclusive
                                    size="small"
                                    onChange={(_, value) => value && setClaudeJsonMode(value)}
                                    sx={{ bgcolor: 'grey.700', '& .MuiToggleButton-root': { color: 'grey.300', padding: '2px 8px', fontSize: '0.75rem' } }}
                                >
                                    <ToggleButton value="json" sx={{ '&.Mui-selected': { bgcolor: 'primary.main', color: 'white' } }}>
                                        JSON
                                    </ToggleButton>
                                    <ToggleButton value="script" sx={{ '&.Mui-selected': { bgcolor: 'primary.main', color: 'white' } }}>
                                        Script
                                    </ToggleButton>
                                </ToggleButtonGroup>
                            }
                        />
                    </Box>
                </Box>

                {/* .claude.json section */}
                <Box sx={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
                    <Box sx={{ mb: 1 }}>
                        <Typography variant="subtitle2" color="text.secondary">
                            {t('claudeCode.step2')}
                        </Typography>
                    </Box>
                    <Box sx={{ flex: 1, height: 400 }}>
                        <CodeBlock
                            code={claudeJsonMode === 'json' ? generateClaudeJsonConfig() : generateScript()}
                            language={claudeJsonMode === 'json' ? 'json' : 'js'}
                            filename={claudeJsonMode === 'json' ? 'Set hasCompletedOnboarding into ~/.claude.json' : 'Script to setup ~/.claude.json'}
                            wrap={true}
                            onCopy={(code) => copyToClipboard(code, claudeJsonMode === 'json' ? '.claude.json' : 'script')}
                            maxHeight={180}
                            minHeight={180}
                            headerActions={
                                <ToggleButtonGroup
                                    value={claudeJsonMode}
                                    exclusive
                                    size="small"
                                    onChange={(_, value) => value && setClaudeJsonMode(value)}
                                    sx={{ bgcolor: 'grey.700', '& .MuiToggleButton-root': { color: 'grey.300', padding: '2px 8px', fontSize: '0.75rem' } }}
                                >
                                    <ToggleButton value="json" sx={{ '&.Mui-selected': { bgcolor: 'primary.main', color: 'white' } }}>
                                        JSON
                                    </ToggleButton>
                                    <ToggleButton value="script" sx={{ '&.Mui-selected': { bgcolor: 'primary.main', color: 'white' } }}>
                                        Script
                                    </ToggleButton>
                                </ToggleButtonGroup>
                            }
                        />
                    </Box>
                </Box>
            </Box>
    );

    // Show empty state if no providers
    if (!providers.length) {
        return (
            <PageLayout loading={loadingRule}>
                <CardGrid>
                    <UnifiedCard title="Use Claude Code" size="full">
                        <EmptyStateGuide
                            title="No Providers Configured"
                            description="Add an API key or OAuth provider to get started"
                            onAddApiKeyClick={providerDialog.handleAddProviderClick}
                            onAddOAuthClick={() => setOAuthDialogOpen(true)}
                        />
                    </UnifiedCard>
                    <ProviderFormDialog
                        open={providerDialog.providerDialogOpen}
                        onClose={providerDialog.handleCloseDialog}
                        onSubmit={providerDialog.handleProviderSubmit}
                        data={providerDialog.providerFormData}
                        onChange={providerDialog.handleFieldChange}
                        mode="add"
                        isFirstProvider={providers.length === 0}
                    />
                    <OAuthDialog
                        open={oauthDialogOpen}
                        onClose={() => setOAuthDialogOpen(false)}
                    />
                </CardGrid>
            </PageLayout>
        );
    }

    return (
        <PageLayout loading={loadingRule}>
            <CardGrid>
                <UnifiedCard
                    title="Use Claude Code"
                    size="full"
                >
                    {header}
                </UnifiedCard>

                {/* Mode switch between header and rules */}
                <UnifiedCard size="full">
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', px: 2, py: 1 }}>
                        <Typography variant="subtitle2" color="text.secondary">
                            Configuration Mode
                        </Typography>
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
                            {CONFIG_MODES.map((mode) => (
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

                <ProviderFormDialog
                    open={providerDialog.providerDialogOpen}
                    onClose={providerDialog.handleCloseDialog}
                    onSubmit={providerDialog.handleProviderSubmit}
                    data={providerDialog.providerFormData}
                    onChange={providerDialog.handleFieldChange}
                    mode="add"
                    isFirstProvider={providers.length === 0}
                />
            </CardGrid>
        </PageLayout>
    );
};

export default UseClaudeCodePage;
