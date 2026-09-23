import { useTranslation } from 'react-i18next';
import CursorConfigModal from './components/CursorConfigModal';
import { ScenarioConfigButton, ScenarioPage } from './ScenarioPage';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const UseCursorPage: React.FC = () => {
    const { t } = useTranslation();
    return (
        <ScenarioPageModalProvider>
            <ScenarioPage
                scenario="cursor"
                title="Cursor"
                tooltipKey="scenarioPage.tooltip.cursor"
                providerCard={{ compact: true, showApiKeyRow: true, showBaseUrlRow: true }}
                renderRightAction={(slot) => (
                    <ScenarioConfigButton
                        onClick={slot.openConfigModal}
                        label={t('scenarioPage.config')}
                    />
                )}
                renderConfigModal={(slot) => (
                    <CursorConfigModal
                        open={slot.configModalOpen}
                        onClose={slot.closeConfigModal}
                        baseUrl={slot.baseUrl}
                        copyToClipboard={slot.copyToClipboard}
                    />
                )}
            />
        </ScenarioPageModalProvider>
    );
};
export default UseCursorPage;
