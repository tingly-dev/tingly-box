import { useState } from 'react';
import AgentSetupCard, { type AgentApplyResult, hasModelOnAnyRule, scrollToModelsCard } from './components/AgentSetupCard';
import { defaultDshPrefs } from './components/DshQuickConfig';
import DshConfigModal from './components/DshConfigModal';
import { api } from '@/services/api';
import { Box, Button, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { ScenarioPage } from './ScenarioPage';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "dsh";
const DSH_REPO_URL = 'https://github.com/deepseek-ai/deepseek-harness';
// dsh serves a local Web UI; tingly-box does not launch it (security), the
// frontend only offers a jump to the default address.
const DSH_WEB_UI_URL = 'http://127.0.0.1:3080';

const UseDshPage: React.FC = () => {
    const { t } = useTranslation();
    const [isApplyLoading, setIsApplyLoading] = useState(false);

    const handleApply = async (): Promise<AgentApplyResult> => {
        try {
            setIsApplyLoading(true);
            const result = await api.applyDshConfig(defaultDshPrefs() as Record<string, string>);
            if (result.success) {
                const files: string[] = [];
                if (result.settingsResult?.created || result.settingsResult?.updated) {
                    files.push('$DSH_HOME/settings.yaml');
                }
                if (result.credentialsResult?.created || result.credentialsResult?.updated) {
                    files.push('$DSH_HOME/.credentials.yaml');
                }
                return { success: true, files };
            }
            return { success: false, error: result.message || t('scenarioPage.unknownError') };
        } catch (err: any) {
            return { success: false, error: err?.message || t('dshConfig.applyFailed') };
        } finally {
            setIsApplyLoading(false);
        }
    };
    return (
        <ScenarioPageModalProvider>
            <ScenarioPage
                scenario={scenario}
                title="DeepSeek Harness"
                tooltipKey="scenarioPage.tooltip.dsh"
                providerCard={{ compact: true, showApiKeyRow: true }}
                withConnectAI
                renderRightAction={(slot) => (
                    <Box sx={{ display: 'flex', gap: 1 }}>
                        <Tooltip title={DSH_WEB_UI_URL}>
                            <Button
                                href={DSH_WEB_UI_URL}
                                target="_blank"
                                rel="noopener noreferrer"
                                variant="contained"
                                size="small"
                            >
                                {t('scenarioPage.dsh.openWebUi')}
                            </Button>
                        </Tooltip>
                        <Button
                            onClick={slot.openConfigModal}
                            variant="outlined"
                            size="small"
                        >
                            {t('scenarioPage.autoConfig')}
                        </Button>
                    </Box>
                )}
                renderConfigModal={(slot) => (
                    <DshConfigModal
                        open={slot.configModalOpen}
                        onClose={slot.closeConfigModal}
                        copyToClipboard={slot.copyToClipboard}
                        showNotification={slot.showNotification}
                    />
                )}
            >
                {(slot) => (
                    <AgentSetupCard
                        agentKey={scenario}
                        agentName="DeepSeek Harness"
                        installCommand="npx @deepseek-ai/dsh web"
                        installStepDescription={t('scenarioPage.dsh.installDescription')}
                        installActions={[
                            { label: t('scenarioPage.dsh.openWebUi'), href: DSH_WEB_UI_URL, variant: 'contained', external: true },
                            { label: t('scenarioPage.dsh.viewRepo'), href: DSH_REPO_URL, variant: 'outlined', external: true },
                        ]}
                        onApply={handleApply}
                        isApplyLoading={isApplyLoading}
                        onViewConfig={slot.openConfigModal}
                        applyStepLabel={t('scenarioPage.dsh.applyStepLabel')}
                        applyStepDescription={t('scenarioPage.dsh.applyStepDescription')}
                        viewConfigButtonLabel={t('scenarioPage.dsh.openGuide')}
                        hasModelSelected={hasModelOnAnyRule(slot.rules)}
                        onSelectModel={scrollToModelsCard}
                        onConnectProvider={slot.connectAI.handleConnectAIClick}
                    />
                )}
            </ScenarioPage>
        </ScenarioPageModalProvider>
    );
};
export default UseDshPage;
