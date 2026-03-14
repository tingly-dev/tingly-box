import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box, Button, Tooltip, IconButton } from '@mui/material';
import InfoIcon from '@mui/icons-material/Info';
import { useEffect, useState } from 'react';
import ExperimentalFeatures from '@/components/ExperimentalFeatures.tsx';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import OpenCodeConfigModal from '@/components/OpenCodeConfigModal';
import { useFunctionPanelData } from '@/hooks/useFunctionPanelData';
import { useRuleManagement } from '@/pages/scenario/hooks/useRuleManagement.ts';
import { useScenarioPageData } from '@/pages/scenario/hooks/useScenarioPageData.ts';
import { api } from '@/services/api';

const scenario = "opencode";

const UseOpenCodePage: React.FC = () => {
    const {
        showTokenModal,
        setShowTokenModal,
        token,
        showNotification,
        providers,
        loading: providersLoading,
        notification,
        loadProviders,
        copyToClipboard,
    } = useFunctionPanelData();

    const {
        rules,
        loadingRule,
        newlyCreatedRuleUuids,
        handleRuleDelete,
        handleRulesChange,
        loadRules,
    } = useRuleManagement();

    const [configModalOpen, setConfigModalOpen] = useState(false);
    const [isApplyLoading, setIsApplyLoading] = useState(false);
    // Config preview state
    const [configJson, setConfigJson] = useState('');
    const [scriptWindows, setScriptWindows] = useState('');
    const [scriptUnix, setScriptUnix] = useState('');
    const [isConfigLoading, setIsConfigLoading] = useState(false);

    const { headerRef, baseUrl, headerHeight } = useScenarioPageData(providers);

    // Fetch OpenCode config preview from backend
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
                setScriptUnix('// Error loading config');
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

    // Handle opening config modal - fetch preview first
    const handleOpenConfigModal = async () => {
        // Reset config state
        setConfigJson('// Loading...');
        setScriptWindows('// Loading...');
        setScriptUnix('// Loading...');
        await fetchConfigPreview();
        setConfigModalOpen(true);
    };

    // Apply handler for OpenCode config - calls backend to generate and write config
    const handleApply = async () => {
        try {
            setIsApplyLoading(true);
            const result = await api.applyOpenCodeConfig();

            if (result.success) {
                const configPath = '~/.config/opencode/opencode.json';
                let successMsg = `Configuration file written: ${configPath}`;
                if (result.created) {
                    successMsg += ' (created)';
                } else if (result.updated) {
                    successMsg += ' (updated)';
                }
                if (result.backupPath) {
                    successMsg += `\nBackup created: ${result.backupPath}`;
                }
                showNotification(successMsg, 'success');
            } else {
                showNotification(`Failed to apply opencode.json: ${result.message || 'Unknown error'}`, 'error');
            }
        } catch (err) {
            showNotification('Failed to apply OpenCode config', 'error');
        } finally {
            setIsApplyLoading(false);
        }
    };

    useEffect(() => {
        loadRules(scenario);
    }, [scenario, loadRules]);

    const isLoading = providersLoading || loadingRule;

    return (
        <PageLayout loading={isLoading} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    ref={headerRef}
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <span>OpenCode SDK Configuration</span>
                            <Tooltip title={`Base URL: ${baseUrl}/tingly/opencode`}>
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
                            Quick Config
                        </Button>
                    }
                >
                    <ProviderConfigCard
                        title="OpenCode SDK Configuration"
                        baseUrlPath="/tingly/opencode"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        token={token}
                        onShowTokenModal={() => setShowTokenModal(true)}
                        scenario={scenario}
                        showApiKeyRow={true}
                    />
                </UnifiedCard>

                <TemplatePage
                    title="Models and Forwarding Rules"
                    scenario={scenario}
                    rules={rules}
                    collapsible={true}
                    showTokenModal={showTokenModal}
                    setShowTokenModal={setShowTokenModal}
                    token={token}
                    showNotification={showNotification}
                    providers={providers}
                    onRulesChange={handleRulesChange}
                    onProvidersLoad={loadProviders}
                    newlyCreatedRuleUuids={newlyCreatedRuleUuids}
                    allowDeleteRule={true}
                    onRuleDelete={handleRuleDelete}
                    headerHeight={headerHeight}
                />

                <OpenCodeConfigModal
                    open={configModalOpen}
                    onClose={() => setConfigModalOpen(false)}
                    generateConfigJson={() => configJson}
                    generateScriptWindows={() => scriptWindows}
                    generateScriptUnix={() => scriptUnix}
                    copyToClipboard={copyToClipboard}
                    onApply={handleApply}
                    isApplyLoading={isApplyLoading}
                    isLoading={isConfigLoading}
                />
            </CardGrid>
        </PageLayout>
    );
};

export default UseOpenCodePage;
