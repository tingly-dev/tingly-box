import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import AgentSetupCard, { type AgentApplyResult, hasModelOnAnyRule, scrollToModelsCard } from '@/components/AgentSetupCard';
import OpenCodeConfigModal from '@/components/OpenCodeConfigModal';
import { Box, Button, IconButton, Tooltip } from '@mui/material';
import { Info as InfoIcon } from '@/components/icons';
import { useState } from 'react';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { api } from '@/services/api';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "opencode";

const UseOpenCodePageContent: React.FC = () => {
    const {
        isLoading,
        notification,
        showNotification,
        copyToClipboard,
        baseUrl,
        rules,
    } = useScenarioPageInternal(scenario);

    const [isApplyLoading, setIsApplyLoading] = useState(false);
    const [configModalOpen, setConfigModalOpen] = useState(false);
    const [configJson, setConfigJson] = useState('');
    const [scriptWindows, setScriptWindows] = useState('');
    const [scriptUnix, setScriptUnix] = useState('');
    const [isConfigLoading, setIsConfigLoading] = useState(false);

    const fetchConfigPreview = async () => {
        setIsConfigLoading(true);
        try {
            const result = await api.getOpenCodeConfigPreview();
            if (result.success) {
                setConfigJson(result.configJson);
                setScriptWindows(result.scriptWindows);
                setScriptUnix(result.scriptUnix);
            } else {
                setConfigJson('// Error: ' + (result.message || 'Failed to load config'));
                setScriptWindows('// Error loading config');
                setScriptUnix('// Error: Failed to connect to server');
                showNotification('Failed to load config preview: ' + (result.message || 'Unknown error'), 'error');
            }
        } catch (err) {
            console.error('Failed to fetch config preview:', err);
            setConfigJson('// Error: Failed to connect to server');
            setScriptWindows('// Error: Failed to connect to server');
            setScriptUnix('// Error: Failed to connect to server');
            showNotification('Failed to load config preview', 'error');
        } finally {
            setIsConfigLoading(false);
        }
    };

    const handleOpenConfigModal = async () => {
        setConfigJson('// Loading...');
        setScriptWindows('// Loading...');
        setScriptUnix('// Loading...');
        await fetchConfigPreview();
        setConfigModalOpen(true);
    };

    const handleApply = async (): Promise<AgentApplyResult> => {
        try {
            setIsApplyLoading(true);
            const result = await api.applyOpenCodeConfig();
            if (result.success) {
                return {
                    success: true,
                    files: ['~/.config/opencode/opencode.json'],
                };
            }
            return { success: false, error: result.message || 'Unknown error' };
        } catch {
            return { success: false, error: 'Failed to apply OpenCode config' };
        } finally {
            setIsApplyLoading(false);
        }
    };

    return (
        <PageLayout loading={isLoading} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <span>OpenCode</span>
                            <Tooltip title="OpenCode AI development environment with BYOK support">
                                <IconButton size="small" sx={{ ml: 0.5 }}>
                                    <InfoIcon fontSize="small" sx={{ color: 'text.secondary' }} />
                                </IconButton>
                            </Tooltip>
                        </Box>
                    }
                    size="full"
                    rightAction={
                        <Button
                            onClick={handleOpenConfigModal}
                            variant="contained"
                            size="small"
                        >
                            Auto Config
                        </Button>
                    }
                >
                    <ProviderConfigCard
                        title="OpenCode Configuration"
                        baseUrlPath="/tingly/opencode"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        scenario={scenario}
                        showApiKeyRow={true}
                        compact={true}
                    />
                </UnifiedCard>

                <AgentSetupCard
                    agentKey={scenario}
                    agentName="OpenCode"
                    installCommand="npm install -g opencode-ai"
                    installMirrorCommand="npm install -g opencode-ai --registry=https://registry.npmmirror.com"
                    onApply={handleApply}
                    isApplyLoading={isApplyLoading}
                    onViewConfig={handleOpenConfigModal}
                    hasModelSelected={hasModelOnAnyRule(rules)}
                    onSelectModel={scrollToModelsCard}
                />

                <TemplatePage
                    scenario={scenario}
                    title="Models and Forwarding Rules"
                    collapsible={true}
                    allowDeleteRule={true}
                />

                <OpenCodeConfigModal
                    open={configModalOpen}
                    onClose={() => setConfigModalOpen(false)}
                    generateConfigJson={() => configJson}
                    generateScriptWindows={() => scriptWindows}
                    generateScriptUnix={() => scriptUnix}
                    copyToClipboard={copyToClipboard}
                    onApply={async () => { await handleApply(); }}
                    isApplyLoading={isApplyLoading}
                    isLoading={isConfigLoading}
                />
            </CardGrid>
        </PageLayout>
    );
};

const UseOpenCodePage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UseOpenCodePageContent />
        </ScenarioPageModalProvider>
    );
};

export default UseOpenCodePage;
