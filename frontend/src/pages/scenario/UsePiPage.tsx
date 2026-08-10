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
import PiConfigModal from './components/PiConfigModal';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';
const scenario = "pi";
const PI_REPO_URL = 'https://github.com/earendil-works/pi';
const UsePiPageContent: React.FC = () => {
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
                            <span>Pi</span>
                            <Tooltip title={t('scenarioPage.tooltip.pi')}>
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
                        title="Pi"
                        baseUrlPath="/tingly/pi"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        scenario={scenario}
                        showApiKeyRow={true}
                        compact={true}
                    />
                </UnifiedCard>
                <AgentSetupCard
                    agentKey={scenario}
                    agentName="Pi"
                    installCommand=""
                    installStepDescription={t('scenarioPage.pi.installDescription')}
                    installActions={[
                        { label: t('scenarioPage.pi.viewRepo'), href: PI_REPO_URL, variant: 'outlined', external: true },
                    ]}
                    onViewConfig={handleOpenConfigModal}
                    applyStepLabel={t('scenarioPage.pi.applyStepLabel')}
                    applyStepDescription={t('scenarioPage.pi.applyStepDescription')}
                    viewConfigButtonLabel={t('scenarioPage.pi.openGuide')}
                    hasModelSelected={hasModelOnAnyRule(rules)}
                    onSelectModel={scrollToModelsCard}
                    onConnectProvider={connectAI.handleConnectAIClick}
                />
                <TemplatePage
                    scenario={scenario}
                    collapsible={true}
                    allowDeleteRule={true}
                />
                <PiConfigModal
                    open={configModalOpen}
                    onClose={() => setConfigModalOpen(false)}
                />
                <ConnectAIDialogs flow={connectAI}/>
            </CardGrid>
        </PageLayout>
    );
};
const UsePiPage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UsePiPageContent />
        </ScenarioPageModalProvider>
    );
};
export default UsePiPage;
