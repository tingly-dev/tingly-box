import { Box, Typography, ToggleButton, ToggleButtonGroup } from '@mui/material';
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
import EmptyStateGuide from '../components/EmptyStateGuide';
import ProviderFormDialog, { type EnhancedProviderFormData } from '../components/ProviderFormDialog';

const ruleId = "built-in-cc";

type ClaudeJsonMode = 'json' | 'script';

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
    const [rule, setRule] = React.useState<any>(null);
    const [defaultModel, setDefaultModel] = React.useState("");
    const [loadingRule, setLoadingRule] = React.useState(true);
    const [isDockerMode, setIsDockerMode] = React.useState(false);
    const [claudeJsonMode, setClaudeJsonMode] = React.useState<ClaudeJsonMode>('json');

    // Provider dialog state
    const [providerDialogOpen, setProviderDialogOpen] = React.useState(false);
    const [providerFormData, setProviderFormData] = React.useState<EnhancedProviderFormData>({
        name: '',
        apiBase: '',
        apiStyle: undefined,
        token: '',
        enabled: true,
        noKeyRequired: false,
    });

    const copyToClipboard = async (text: string, label: string) => {
        try {
            await navigator.clipboard.writeText(text);
            showNotification(`${label} copied to clipboard!`, 'success');
        } catch (err) {
            showNotification('Failed to copy to clipboard', 'error');
        }
    };

    // Provider dialog handlers
    const handleAddProviderClick = () => {
        setProviderFormData({
            name: '',
            apiBase: '',
            apiStyle: 'anthropic', // Default to Anthropic for Claude Code
            token: '',
            enabled: true,
            noKeyRequired: false,
        });
        setProviderDialogOpen(true);
    };

    const handleProviderSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        const providerData = {
            name: providerFormData.name,
            api_base: providerFormData.apiBase,
            api_style: providerFormData.apiStyle,
            token: providerFormData.token,
            no_key_required: providerFormData.noKeyRequired,
        };

        const result = await api.addProvider(providerData);

        if (result.success) {
            showNotification('API Key added successfully!', 'success');
            setProviderDialogOpen(false);
            // Reload providers through the hook
            window.location.reload();
        } else {
            showNotification(`Failed to add API Key: ${result.error}`, 'error');
        }
    };

    const loadData = async () => {
        const url = await getBaseUrl();
        setBaseUrl(url);

        // Fetch rule information
        const result = await api.getRule(ruleId);
        if (result.success) {
            setRule(result.data);
        }
        setLoadingRule(false);
    };

    React.useEffect(() => {
        loadData();
    }, []);

    // Update defaultModel when rule changes
    React.useEffect(() => {
        if (rule?.request_model) {
            setDefaultModel(rule.request_model);
        }
    }, [rule]);

    const toDockerUrl = (url: string): string => {
        return url.replace(/\/\/([^/:]+)(?::(\d+))?/, '//host.docker.internal:$2');
    };

    const getClaudeCodeBaseUrl = () => {
        const url = `${baseUrl}/tingly/claude_code`;
        return isDockerMode ? toDockerUrl(url) : url;
    };

    const generateSettingsConfig = () => {
        const claudeCodeBaseUrl = getClaudeCodeBaseUrl();
        return JSON.stringify({
            env: {
                DISABLE_TELEMETRY: "1",
                DISABLE_ERROR_REPORTING: "1",
                CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
                API_TIMEOUT_MS: "3000000",
                ANTHROPIC_AUTH_TOKEN: token,
                ANTHROPIC_BASE_URL: claudeCodeBaseUrl,
                ANTHROPIC_DEFAULT_HAIKU_MODEL: defaultModel,
                ANTHROPIC_DEFAULT_OPUS_MODEL: defaultModel,
                ANTHROPIC_DEFAULT_SONNET_MODEL: defaultModel,
                ANTHROPIC_MODEL: defaultModel
            },
        }, null, 2);
    };

    const generateSettingsScript = () => {
        const claudeCodeBaseUrl = getClaudeCodeBaseUrl();
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
        ANTHROPIC_DEFAULT_HAIKU_MODEL: "${defaultModel}",
        ANTHROPIC_DEFAULT_OPUS_MODEL: "${defaultModel}",
        ANTHROPIC_DEFAULT_SONNET_MODEL: "${defaultModel}",
        ANTHROPIC_MODEL: "${defaultModel}"
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
            <PageLayout>
                <CardGrid>
                    <UnifiedCard title="Use Claude Code" size="full">
                        <EmptyStateGuide
                            title="No API Keys Configured"
                            description="Add an Anthropic API key to get started with Claude Code"
                            buttonText="Add API Key"
                            onButtonClick={handleAddProviderClick}
                        />
                    </UnifiedCard>
                    <ProviderFormDialog
                        open={providerDialogOpen}
                        onClose={() => setProviderDialogOpen(false)}
                        onSubmit={handleProviderSubmit}
                        data={providerFormData}
                        onChange={(field, value) => setProviderFormData(prev => ({ ...prev, [field]: value }))}
                        mode="add"
                    />
                </CardGrid>
            </PageLayout>
        );
    }

    return (
        <PageLayout>
            <CardGrid>
                <UnifiedCard
                    title="Use Claude Code"
                    size="full"
                >
                    {header}
                </UnifiedCard>
                <TemplatePage
                    rules={[rule]}
                    showTokenModal={showTokenModal}
                    setShowTokenModal={setShowTokenModal}
                    token={token}
                    showNotification={showNotification}
                    providers={providers}
                    onRulesChange={(rules) => setRule(rules[0])}
                    allowToggleRule={false}
                />
                <ProviderFormDialog
                    open={providerDialogOpen}
                    onClose={() => setProviderDialogOpen(false)}
                    onSubmit={handleProviderSubmit}
                    data={providerFormData}
                    onChange={(field, value) => setProviderFormData(prev => ({ ...prev, [field]: value }))}
                    mode="add"
                />
            </CardGrid>
        </PageLayout>
    );
};

export default UseClaudeCodePage;
