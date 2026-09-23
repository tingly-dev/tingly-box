import { useTranslation } from 'react-i18next';
import XcodeConfigModal from './components/XcodeConfigModal';
import { ScenarioConfigButton, ScenarioPage } from './ScenarioPage';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const UseXcodePage: React.FC = () => {
    const { t } = useTranslation();
    return (
        <ScenarioPageModalProvider>
            <ScenarioPage
                scenario="xcode"
                title="Xcode"
                tooltipKey="scenarioPage.tooltip.xcode"
                providerCard={{ compact: true, showApiKeyRow: true, showBaseUrlRow: true }}
                renderRightAction={(slot) => (
                    <ScenarioConfigButton
                        onClick={slot.openConfigModal}
                        label={t('scenarioPage.config')}
                    />
                )}
                renderConfigModal={(slot) => (
                    <XcodeConfigModal
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
export default UseXcodePage;
