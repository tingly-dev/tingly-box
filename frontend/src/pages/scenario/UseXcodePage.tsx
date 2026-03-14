import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box, Button, Tooltip, IconButton } from '@mui/material';
import InfoIcon from '@mui/icons-material/Info';
import { useEffect, useState } from 'react';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import XcodeConfigModal from '@/components/XcodeConfigModal';
import { useFunctionPanelData } from '@/hooks/useFunctionPanelData';
import { useRuleManagement } from '@/pages/scenario/hooks/useRuleManagement.ts';
import { useScenarioPageData } from '@/pages/scenario/hooks/useScenarioPageData.ts';

const scenario = "xcode";

const UseXcodePage: React.FC = () => {
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

    const { headerRef, baseUrl, headerHeight } = useScenarioPageData(providers);

    const handleOpenConfigModal = () => {
        setConfigModalOpen(true);
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
                            <span>Xcode SDK Configuration</span>
                            <Tooltip title={`Base URL: ${baseUrl}/tingly/xcode`}>
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
                            Config Guide
                        </Button>
                    }
                >
                    <ProviderConfigCard
                        title="Xcode SDK Configuration"
                        baseUrlPath="/tingly/xcode"
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

                <XcodeConfigModal
                    open={configModalOpen}
                    onClose={() => setConfigModalOpen(false)}
                    baseUrl={baseUrl}
                    token={token}
                    copyToClipboard={copyToClipboard}
                />
            </CardGrid>
        </PageLayout>
    );
};

export default UseXcodePage;
