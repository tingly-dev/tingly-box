import { useTranslation } from 'react-i18next';
import ClaudeDesktopConfigModal from './components/ClaudeDesktopConfigModal';
import { ScenarioConfigButton, ScenarioPage } from './ScenarioPage';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "claude_desktop";
const UseClaudeDesktopPage: React.FC = () => {
    const { t } = useTranslation();
    return (
        <ScenarioPageModalProvider>
            <ScenarioPage
                scenario={scenario}
                title="Claude Desktop"
                tooltipKey="scenarioPage.tooltip.claude_desktop"
                providerCard={{ compact: true, showApiKeyRow: true, showBaseUrlRow: true }}
                context1M
                renderRightAction={(slot) => (
                    <ScenarioConfigButton
                        onClick={slot.openConfigModal}
                        label={t('scenarioPage.config')}
                    />
                )}
                renderConfigModal={(slot) => (
                    <ClaudeDesktopConfigModal
                        open={slot.configModalOpen}
                        onClose={() => {
                            slot.closeConfigModal();
                            slot.clearPendingContext1MChange();
                        }}
                        baseUrl={slot.baseUrl}
                        copyToClipboard={slot.copyToClipboard}
                        rules={slot.rules}
                        onRulesRefresh={() => slot.loadRules(scenario)}
                        pendingContext1MChange={slot.pendingContext1MChange}
                    />
                )}
            />
        </ScenarioPageModalProvider>
    );
};
export default UseClaudeDesktopPage;
