import AgentSetupCard, { type AgentApplyResult, hasModelOnAnyRule, scrollToModelsCard } from './components/AgentSetupCard';
import OpenCodeConfigModal from './components/OpenCodeConfigModal';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import { ScenarioConfigButton, ScenarioPage } from './ScenarioPage';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "opencode";

const UseOpenCodePage: React.FC = () => {
    const { t } = useTranslation();
    const [isApplyLoading, setIsApplyLoading] = useState(false);
    const [configJson, setConfigJson] = useState('');
    const [scriptWindows, setScriptWindows] = useState('');
    const [scriptUnix, setScriptUnix] = useState('');
    const [isConfigLoading, setIsConfigLoading] = useState(false);

    const fetchConfigPreview = async (showNotification: (message: string, severity: 'error') => void) => {
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
                setScriptUnix('// Error: Failed to connect to server');
                showNotification(t('scenarioPage.opencode.previewFailed', { reason: result.message || t('scenarioPage.unknownError') }), 'error');
            }
        } catch (err) {
            console.error('Failed to fetch config preview:', err);
            setConfigJson('// Error: Failed to connect to server');
            setScriptWindows('// Error: Failed to connect to server');
            setScriptUnix('// Error: Failed to connect to server');
            showNotification(t('scenarioPage.opencode.previewFailedGeneric'), 'error');
        } finally {
            setIsConfigLoading(false);
        }
    };
    const openConfigWithPreview = async (
        openModal: () => void,
        showNotification: (message: string, severity: 'error') => void,
    ) => {
        setConfigJson('// Loading...');
        setScriptWindows('// Loading...');
        setScriptUnix('// Loading...');
        await fetchConfigPreview(showNotification);
        openModal();
    };
    const handleApply = async (): Promise<AgentApplyResult> => {
        try {
            setIsApplyLoading(true);
            const result = await api.applyOpenCodeConfig();
            if (result.success) {
                return {
                    success: true,
                    files: ['~/.config/opencode/opencode.json'],
                };
            }
            return { success: false, error: result.message || t('scenarioPage.unknownError') };
        } catch {
            return { success: false, error: t('scenarioPage.opencode.applyFailed') };
        } finally {
            setIsApplyLoading(false);
        }
    };
    return (
        <ScenarioPageModalProvider>
            <ScenarioPage
                scenario={scenario}
                title="OpenCode"
                tooltipKey="scenarioPage.tooltip.opencode"
                providerCard={{ title: t('scenarioPage.opencode.configTitle'), compact: true, showApiKeyRow: true }}
                withConnectAI
                renderRightAction={(slot) => (
                    <ScenarioConfigButton
                        onClick={() => void openConfigWithPreview(slot.openConfigModal, slot.showNotification)}
                        label={t('scenarioPage.autoConfig')}
                    />
                )}
                renderConfigModal={(slot) => (
                    <OpenCodeConfigModal
                        open={slot.configModalOpen}
                        onClose={slot.closeConfigModal}
                        generateConfigJson={() => configJson}
                        generateScriptWindows={() => scriptWindows}
                        generateScriptUnix={() => scriptUnix}
                        copyToClipboard={slot.copyToClipboard}
                        onApply={async () => { await handleApply(); }}
                        isApplyLoading={isApplyLoading}
                        isLoading={isConfigLoading}
                    />
                )}
            >
                {(slot) => (
                    <AgentSetupCard
                        agentKey={scenario}
                        agentName="OpenCode"
                        installCommand="npm install -g opencode-ai"
                        installMirrorCommand="npm install -g opencode-ai --registry=https://registry.npmmirror.com"
                        onApply={handleApply}
                        isApplyLoading={isApplyLoading}
                        onViewConfig={() => void openConfigWithPreview(slot.openConfigModal, slot.showNotification)}
                        hasModelSelected={hasModelOnAnyRule(slot.rules)}
                        onSelectModel={scrollToModelsCard}
                        onConnectProvider={slot.connectAI.handleConnectAIClick}
                    />
                )}
            </ScenarioPage>
        </ScenarioPageModalProvider>
    );
};
export default UseOpenCodePage;
