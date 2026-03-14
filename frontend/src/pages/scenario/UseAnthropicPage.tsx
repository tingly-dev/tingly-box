import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box } from '@mui/material';
import { useEffect } from 'react';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import { useFunctionPanelData } from '@/hooks/useFunctionPanelData';
import { useRuleManagement } from '@/pages/scenario/hooks/useRuleManagement.ts';
import { useScenarioPageData } from '@/pages/scenario/hooks/useScenarioPageData.ts';

const scenario = "anthropic";

const UseAnthropicPage: React.FC = () => {
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

    const { headerRef, baseUrl, headerHeight } = useScenarioPageData(providers);

    useEffect(() => {
        loadRules(scenario);
    }, [scenario, loadRules]);

    const isLoading = providersLoading || loadingRule;

    return (
        <PageLayout loading={isLoading} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <span>Anthropic SDK Configuration</span>
                        </Box>
                    }
                    size="full"
                >
                    <ProviderConfigCard
                        headerRef={headerRef}
                        title="Anthropic SDK Configuration"
                        baseUrlPath="/tingly/anthropic"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        token={token}
                        onShowTokenModal={() => setShowTokenModal(true)}
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
            </CardGrid>
        </PageLayout>
    );
};

export default UseAnthropicPage;
