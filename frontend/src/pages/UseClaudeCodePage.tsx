import { Box, Typography, ToggleButton, ToggleButtonGroup, Switch, FormControlLabel } from '@mui/material';
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

const ruleId = "built-in-cc";

type ClaudeJsonMode = 'json' | 'script';
type ConfigMode = 'unified' | 'separate';

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
    const [defaultModel, setDefaultModel] = React.useState("");
    const [loadingRule, setLoadingRule] = React.useState(true);
    const [isDockerMode, setIsDockerMode] = React.useState(false);
    const [claudeJsonMode, setClaudeJsonMode] = React.useState<ClaudeJsonMode>('json');
    const [configMode, setConfigMode] = React.useState<ConfigMode>('unified');

    // Provider dialog hook
    const providerDialog = useProviderDialog(showNotification, {
        defaultApiStyle: 'anthropic',
        onProviderAdded: () => window.location.reload(),
    });

    // OAuth dialog state
    const [oauthDialogOpen, setOAuthDialogOpen] = React.useState(false);

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
            const result = await api.getRule(ruleId);
            if (result.success) {
                setRules([result.data]);
            }
        } else {
            // Load 4 separate rules for each model variant
            const modelVariants = ['haiku', 'sonnet', 'opus', 'main'];
            const loadedRules = await Promise.all(
                modelVariants.map(async (variant) => {
                    const result = await api.getRule(`built-in-cc-${variant}`);
                    if (result.success) {
                        return result.data;
                    }
                    return null;
                })
            );
            setRules(loadedRules.filter((r): r is any => r !== null));
        }

        setLoadingRule(false);
    };

    React.useEffect(() => {
        loadData();
    }, [configMode]);

    // Update defaultModel when rules change
    React.useEffect(() => {
        if (rules.length > 0 && rules[0]?.request_model) {
            setDefaultModel(rules[0].request_model);
        }
    }, [rules]);

    const toDockerUrl = (url: string): string => {
        return url.replace(/\/\/([^/:]+)(?::(\d+))?/, '//host.docker.internal:$2');
    };

    const getClaudeCodeBaseUrl = () => {
        const url = `${baseUrl}/tingly/claude_code`;
        return isDockerMode ? toDockerUrl(url) : url;
    };

    // Get model name for each variant based on config mode
    const getModelForVariant = (variant: string): string => {
        if (configMode === 'unified') {
            return defaultModel;
        }
        // In separate mode, find the corresponding rule
        const rule = rules.find(r => r?.id === `built-in-cc-${variant}`);
        return rule?.request_model || defaultModel;
    };

    const generateSettingsConfig = () => {
        const claudeCodeBaseUrl = getClaudeCodeBaseUrl();
        const haikuModel = getModelForVariant('haiku');
        const sonnetModel = getModelForVariant('sonnet');
        const opusModel = getModelForVariant('opus');
        const mainModel = getModelForVariant('main');

        return JSON.stringify({
            env: {
                DISABLE_TELEMETRY: "1",
                DISABLE_ERROR_REPORTING: "1",
                CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
                API_TIMEOUT_MS: "3000000",
                ANTHROPIC_AUTH_TOKEN: token,
                ANTHROPIC_BASE_URL: claudeCodeBaseUrl,
                ANTHROPIC_DEFAULT_HAIKU_MODEL: haikuModel,
                ANTHROPIC_DEFAULT_OPUS_MODEL: opusModel,
                ANTHROPIC_DEFAULT_SONNET_MODEL: sonnetModel,
                ANTHROPIC_MODEL: mainModel
            },
        }, null, 2);
    };

    const generateSettingsScript = () => {
        const claudeCodeBaseUrl = getClaudeCodeBaseUrl();
        const haikuModel = getModelForVariant('haiku');
        const sonnetModel = getModelForVariant('sonnet');
        const opusModel = getModelForVariant('opus');
        const mainModel = getModelForVariant('main');

        return `# Configure Claude Code settings
echo "Configuring Claude Code settings..."
mkdir -p ~/.claude
node --eval '
    const fs = require("fs");
    const path = require("path");
    const homeDir = os.homedir();
    const settingsPath = path.join(homeDir, ".claude", "settings.json");
    const env = {
        DISABLE_TELEMETRY: "1",
        DISABLE_ERROR_REPORTING: "1",
        CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
        API_TIMEOUT_MS: "3000000",
        ANTHROPIC_AUTH_TOKEN: "${token}",
        ANTHROPIC_BASE_URL: "${claudeCodeBaseUrl}",
        ANTHROPIC_DEFAULT_HAIKU_MODEL: "${haikuModel}",
        ANTHROPIC_DEFAULT_OPUS_MODEL: "${opusModel}",
        ANTHROPIC_DEFAULT_SONNET_MODEL: "${sonnetModel}",
        ANTHROPIC_MODEL: "${mainModel}"
    };
    if (fs.existsSync(settingsPath)) {
        const content = JSON.parse(fs.readFileSync(settingsPath, "utf-8"));
        fs.writeFileSync(settingsPath, JSON.stringify({ ...content, env }, null, 2), "utf-8");
    } else {
        fs.writeFileSync(settingsPath, JSON.stringify({ env }, null, 2), "utf-8");
    }
'`;
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
        <>
            {/* Top bar with switch */}
            <Box sx={{ px: 2, pt: 2, pb: 1, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="subtitle2" color="text.secondary">
                    {configMode === 'unified' ? t('claudeCode.unifiedConfig') : t('claudeCode.separateConfig')}
                </Typography>
                <FormControlLabel
                    control={
                        <Switch
                            checked={configMode === 'separate'}
                            onChange={(e) => setConfigMode(e.target.checked ? 'separate' : 'unified')}
                            size="small"
                        />
                    }
                    label={configMode === 'unified' ? t('claudeCode.switchToSeparate') : t('claudeCode.switchToUnified')}
                    sx={{ '& .MuiFormControlLabel-label': { fontSize: '0.875rem' } }}
                />
            </Box>
            {/* Settings.json and .claude.json sections */}
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
        </>
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
                <TemplatePage
                    rules={rules}
                    showTokenModal={showTokenModal}
                    setShowTokenModal={setShowTokenModal}
                    token={token}
                    showNotification={showNotification}
                    providers={providers}
                    onRulesChange={setRules}
                    allowToggleRule={false}
                    collapsible={true}
                />
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
