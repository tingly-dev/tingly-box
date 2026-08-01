import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import AgentSetupCard, { hasModelOnAnyRule, scrollToModelsCard } from './components/AgentSetupCard';
import ConnectAIDialogs from '@/components/ConnectAIDialogs';
import {useProviderDialog} from '@/hooks/useProviderDialog';
import { Box, Button, Tooltip, IconButton } from '@mui/material';
import { Info as InfoIcon } from '@/components/icons';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import PageLayout from '@/components/PageLayout';
import ScenarioPageSkeleton from './components/ScenarioPageSkeleton';
import TemplatePage from './components/TemplatePage.tsx';
import VSCodeConfigModal from './components/VSCodeConfigModal';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';
const scenario = "vscode";
const MARKETPLACE_URL = 'https://marketplace.visualstudio.com/items?itemName=Tingly-Dev.vscode-tingly-box';
const VSCODE_INSTALL_URL = 'vscode:extension/Tingly-Dev.vscode-tingly-box';
const UseVSCodePageContent: React.FC = () => {
    const { t } = useTranslation();
    const {
        isLoading,
        notification,
        showNotification,
        copyToClipboard,
        baseUrl,
        rules,
    } = useScenarioPageInternal(scenario);
    const [configModalOpen, setConfigModalOpen] = useState(false);
    // Unified Connect AI add flow (picker + form/OAuth/paste/import dialogs).
    const connectAI = useProviderDialog(showNotification, {
        onProviderAdded: () => window.location.reload(),
    });
    const handleOpenConfigModal = () => {
        setConfigModalOpen(true);
    };
    return (
        <PageLayout loading={isLoading} loadingContent={<ScenarioPageSkeleton />} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    titleHeadingLevel={1}
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <span>VS Code</span>
                            <Tooltip title={t('scenarioPage.tooltip.vscode')}>
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
                            {t('scenarioPage.config')}
                        </Button>
                    }
                >
                    <ProviderConfigCard
                        title="VS Code"
                        baseUrlPath="/tingly/vscode"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        scenario={scenario}
                        showApiKeyRow={true}
                        showBaseUrlRow={true}
                        compact={true}
                    />
                </UnifiedCard>
                <AgentSetupCard
                    agentKey={scenario}
                    agentName="VS Code"
                    installCommand=""
                    installStepDescription={t('scenarioPage.vscode.installDescription')}
                    // Outlined, not contained: the step row's "I've installed it"
                    // is the one contained button in this step.
                    installActions={[
                        { label: t('scenarioPage.vscode.installInVSCode'), href: VSCODE_INSTALL_URL, variant: 'outlined' },
                        { label: t('scenarioPage.vscode.viewMarketplace'), href: MARKETPLACE_URL, variant: 'outlined', external: true },
                    ]}
                    onViewConfig={handleOpenConfigModal}
                    applyStepLabel={t('scenarioPage.vscode.applyStepLabel')}
                    applyStepDescription={t('scenarioPage.vscode.applyStepDescription')}
                    viewConfigButtonLabel={t('scenarioPage.vscode.openGuide')}
                    hasModelSelected={hasModelOnAnyRule(rules)}
                    onSelectModel={scrollToModelsCard}
                    onConnectProvider={connectAI.handleConnectAIClick}
                />
                <TemplatePage
                    scenario={scenario}
                    collapsible={true}
                    allowDeleteRule={true}
                />
                <VSCodeConfigModal
                    open={configModalOpen}
                    onClose={() => setConfigModalOpen(false)}
                />
                <ConnectAIDialogs flow={connectAI}/>
            </CardGrid>
        </PageLayout>
    );
};
const UseVSCodePage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UseVSCodePageContent />
        </ScenarioPageModalProvider>
    );
};
export default UseVSCodePage;
