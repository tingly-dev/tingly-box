import AgentSetupCard, { hasModelOnAnyRule, scrollToModelsCard } from './components/AgentSetupCard';
import PiConfigModal from './components/PiConfigModal';
import { useTranslation } from 'react-i18next';
import { ScenarioConfigButton, ScenarioPage } from './ScenarioPage';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "pi";
const PI_REPO_URL = 'https://github.com/earendil-works/pi';

const UsePiPage: React.FC = () => {
    const { t } = useTranslation();
    return (
        <ScenarioPageModalProvider>
            <ScenarioPage
                scenario={scenario}
                title="Pi"
                tooltipKey="scenarioPage.tooltip.pi"
                providerCard={{ compact: true, showApiKeyRow: true }}
                withConnectAI
                renderRightAction={(slot) => (
                    <ScenarioConfigButton
                        onClick={slot.openConfigModal}
                        label={t('scenarioPage.config')}
                    />
                )}
                renderConfigModal={(slot) => (
                    <PiConfigModal
                        open={slot.configModalOpen}
                        onClose={slot.closeConfigModal}
                    />
                )}
            >
                {(slot) => (
                    <AgentSetupCard
                        agentKey={scenario}
                        agentName="Pi"
                        installCommand=""
                        installStepDescription={t('scenarioPage.pi.installDescription')}
                        installActions={[
                            { label: t('scenarioPage.pi.viewRepo'), href: PI_REPO_URL, variant: 'outlined', external: true },
                        ]}
                        onViewConfig={slot.openConfigModal}
                        applyStepLabel={t('scenarioPage.pi.applyStepLabel')}
                        applyStepDescription={t('scenarioPage.pi.applyStepDescription')}
                        viewConfigButtonLabel={t('scenarioPage.pi.openGuide')}
                        hasModelSelected={hasModelOnAnyRule(slot.rules)}
                        onSelectModel={scrollToModelsCard}
                        onConnectProvider={slot.connectAI.handleConnectAIClick}
                    />
                )}
            </ScenarioPage>
        </ScenarioPageModalProvider>
    );
};
export default UsePiPage;
