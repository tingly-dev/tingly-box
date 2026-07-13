import CardGrid from "@/components/CardGrid.tsx";
import AgentSetupCard, { type AgentApplyResult, hasModelOnAnyRule, scrollToModelsCard } from './components/AgentSetupCard';
import CodexConfigModal from "./components/CodexConfigModal";
import ConnectProviderFlow from '@/components/ConnectProviderFlow';
import { defaultCodexPrefs } from "./components/CodexQuickConfig";
import { api } from '@/services/api';
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box, Button, IconButton, Tooltip, Dialog, DialogActions, DialogContent, DialogTitle, Typography, Alert } from '@mui/material';
import { Info as InfoIcon, Refresh as RestartIcon } from '@/components/icons';
import { useState } from 'react';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';
const scenario = "codex";
const UseCodexPageContent: React.FC = () => {
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
    const [connectProviderOpen, setConnectProviderOpen] = useState(false);
    const [pendingContext1MChange, setPendingContext1MChange] = useState<boolean | null>(null);
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
            return { success: false, error: result.message || 'Unknown error' };
        } catch (err: any) {
            return { success: false, error: err?.message || 'Failed to apply Codex config' };
        } finally {
            setIsApplyLoading(false);
        }
    };
    const handleContext1MToggle = (newState: boolean) => {
        // Store the pending change and directly open config panel
        setPendingContext1MChange(newState);
        setConfigModalOpen(true);
    };
    return (
        <PageLayout loading={isLoading} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <span>Codex</span>
                            <Tooltip title="OpenAI Codex AI coding assistant with Tingly Box proxy">
                                <IconButton size="small" sx={{ ml: 0.5 }}>
                                    <InfoIcon fontSize="small" sx={{ color: 'text.secondary' }} />
                                </IconButton>
                            </Tooltip>
                        </Box>
                    }
                    size="full"
                    rightAction={
                        <Button
                            onClick={() => setConfigModalOpen(true)}
                            variant="contained"
                            size="small"
                        >
                            Auto Config
                        </Button>
                    }
                >
                    <ProviderConfigCard
                        title="Codex Configuration"
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
                    onConnectProvider={() => setConnectProviderOpen(true)}
                />
                <TemplatePage
                    scenario={scenario}
                    collapsible={true}
                    allowDeleteRule={true}
                    onContext1MToggle={handleContext1MToggle}
                />
                <CodexConfigModal
                    open={configModalOpen}
                    onClose={() => {
                        setConfigModalOpen(false);
                        setPendingContext1MChange(null);
                    }}
                    copyToClipboard={copyToClipboard}
                    showNotification={showNotification}
                    pendingContext1MChange={pendingContext1MChange}
                />
                <ConnectProviderFlow
                    open={connectProviderOpen}
                    onClose={() => setConnectProviderOpen(false)}
                    showNotification={showNotification}
                    onProviderAdded={() => window.location.reload()}
                />
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
