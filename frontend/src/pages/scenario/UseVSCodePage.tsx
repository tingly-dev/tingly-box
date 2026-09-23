import AgentSetupCard, { hasModelOnAnyRule, scrollToModelsCard } from './components/AgentSetupCard';
import VSCodeConfigModal from './components/VSCodeConfigModal';
import { useTranslation } from 'react-i18next';
import { ScenarioConfigButton, ScenarioPage } from './ScenarioPage';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "vscode";
const MARKETPLACE_URL = 'https://marketplace.visualstudio.com/items?itemName=Tingly-Dev.vscode-tingly-box';
const VSCODE_INSTALL_URL = 'vscode:extension/Tingly-Dev.vscode-tingly-box';

const UseVSCodePage: React.FC = () => {
    const { t } = useTranslation();
    return (
        <ScenarioPageModalProvider>
            <ScenarioPage
                scenario={scenario}
                title="VS Code"
                tooltipKey="scenarioPage.tooltip.vscode"
                providerCard={{ compact: true, showApiKeyRow: true, showBaseUrlRow: true }}
                withConnectAI
                renderRightAction={(slot) => (
                    <ScenarioConfigButton
                        onClick={slot.openConfigModal}
                        label={t('scenarioPage.config')}
                    />
                )}
                renderConfigModal={(slot) => (
                    <VSCodeConfigModal
                        open={slot.configModalOpen}
                        onClose={slot.closeConfigModal}
                    />
                )}
            >
                {(slot) => (
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
                        onViewConfig={slot.openConfigModal}
                        applyStepLabel={t('scenarioPage.vscode.applyStepLabel')}
                        applyStepDescription={t('scenarioPage.vscode.applyStepDescription')}
                        viewConfigButtonLabel={t('scenarioPage.vscode.openGuide')}
                        hasModelSelected={hasModelOnAnyRule(slot.rules)}
                        onSelectModel={scrollToModelsCard}
                        onConnectProvider={slot.connectAI.handleConnectAIClick}
                    />
                )}
            </ScenarioPage>
        </ScenarioPageModalProvider>
    );
};
export default UseVSCodePage;
