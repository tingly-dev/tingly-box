import CardGrid from "@/components/CardGrid.tsx";
import AgentSetupCard, { type AgentApplyResult, hasModelOnAnyRule, scrollToModelsCard } from './components/AgentSetupCard';
import CodexConfigModal from "./components/CodexConfigModal";
import ConnectAIDialogs from '@/components/ConnectAIDialogs';
import {useProviderDialog} from '@/hooks/useProviderDialog';
import { defaultCodexPrefs } from "./components/CodexQuickConfig";
import { api } from '@/services/api';
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Button } from '@mui/material';
import { Refresh as RestartIcon } from '@/components/icons';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import PageLayout from '@/components/PageLayout';
import ScenarioPageSkeleton from './components/ScenarioPageSkeleton';
import TemplatePage from './components/TemplatePage.tsx';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { useContext1MToggle } from '@/pages/scenario/hooks/useContext1MToggle';
import { SCENARIO_HEADER_CONTENT_MAX_WIDTH, ScenarioCardHeader } from './ScenarioPage';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';
const scenario = "codex";
const UseCodexPageContent: React.FC = () => {
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
    const [isApplyLoading, setIsApplyLoading] = useState(false);
    // Unified Connect AI add flow (picker + form/OAuth/paste/import dialogs).
    const connectAI = useProviderDialog(showNotification, {
        onProviderAdded: () => window.location.reload(),
    });
    // Context-1M toggle plumbing shared with ScenarioPage (hooks/useContext1MToggle).
    const context1M = useContext1MToggle(() => setConfigModalOpen(true));
    const handleApply = async (): Promise<AgentApplyResult> => {
        try {
            setIsApplyLoading(true);
            const result = await api.applyCodexConfig(defaultCodexPrefs() as Record<string, string>);
            if (result.success) {
                // Extract files from config and auth results
                const files: string[] = [];
                if (result.configResult?.created) {
                    files.push('~/.codex/config.toml');
                } else if (result.configResult?.updated) {
                    files.push('~/.codex/config.toml');
                }
                if (result.authResult?.created) {
                    files.push('~/.codex/auth.json');
                } else if (result.authResult?.updated) {
                    files.push('~/.codex/auth.json');
                }
                return { success: true, files };
            }
            return { success: false, error: result.message || t('scenarioPage.unknownError') };
        } catch (err: any) {
            return { success: false, error: err?.message || t('scenarioPage.codex.applyFailed') };
        } finally {
            setIsApplyLoading(false);
        }
    };
    return (
        <PageLayout loading={isLoading} loadingContent={<ScenarioPageSkeleton />} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    titleHeadingLevel={1}
                    title={
                        <ScenarioCardHeader title="Codex" tooltipKey="scenarioPage.tooltip.codex" />
                    }
                    size="full"
                    contentMaxWidth={SCENARIO_HEADER_CONTENT_MAX_WIDTH}
                    rightAction={
                        <Button
                            onClick={() => setConfigModalOpen(true)}
                            variant="contained"
                            size="small"
                        >
                            {t('scenarioPage.autoConfig')}
                        </Button>
                    }
                >
                    <ProviderConfigCard
                        title={t('scenarioPage.codex.configTitle')}
                        baseUrlPath="/tingly/codex"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        scenario={scenario}
                        compact={true}
                    />
                </UnifiedCard>
                <AgentSetupCard
                    agentKey={scenario}
                    agentName="Codex"
                    installCommand="npm install -g @openai/codex"
                    installMirrorCommand="npm install -g @openai/codex --registry=https://registry.npmmirror.com"
                    onApply={handleApply}
                    isApplyLoading={isApplyLoading}
                    onViewConfig={() => setConfigModalOpen(true)}
                    hasModelSelected={hasModelOnAnyRule(rules)}
                    onSelectModel={scrollToModelsCard}
                    onConnectProvider={connectAI.handleConnectAIClick}
                />
                <TemplatePage
                    scenario={scenario}
                    collapsible={true}
                    allowDeleteRule={true}
                    onContext1MToggle={context1M.handleContext1MToggle}
                />
                <CodexConfigModal
                    open={configModalOpen}
                    onClose={() => {
                        setConfigModalOpen(false);
                        context1M.clearPendingContext1MChange();
                    }}
                    copyToClipboard={copyToClipboard}
                    showNotification={showNotification}
                    pendingContext1MChange={context1M.pendingContext1MChange}
                />
                <ConnectAIDialogs flow={connectAI}/>
            </CardGrid>
        </PageLayout>
    );
};
const UseCodexPage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UseCodexPageContent />
        </ScenarioPageModalProvider>
    );
};
export default UseCodexPage;
