import CardGrid from "@/components/CardGrid.tsx";
import AgentSetupCard, { type AgentApplyResult, hasModelOnAnyRule, scrollToModelsCard } from '@/components/AgentSetupCard';
import CodexConfigModal from "@/components/CodexConfigModal.tsx";
import { api } from '@/services/api';
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box, Button, IconButton, Tooltip } from '@mui/material';
import { Info as InfoIcon } from '@/components/icons';
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
        copyToClipboard,
        baseUrl,
        rules,
    } = useScenarioPageInternal(scenario);

    const [configModalOpen, setConfigModalOpen] = useState(false);
    const [isApplyLoading, setIsApplyLoading] = useState(false);

    const handleApply = async (): Promise<AgentApplyResult> => {
        try {
            setIsApplyLoading(true);
            const result = await api.applyCodexConfig();
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

    return (
        <PageLayout loading={isLoading} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <span>Codex Configuration</span>
                            <Tooltip title={`Base URL: ${baseUrl}/tingly/codex`}>
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
                />

                <TemplatePage
                    scenario={scenario}
                    title="Models and Forwarding Rules"
                    collapsible={true}
                    allowDeleteRule={true}
                />

                <CodexConfigModal
                    open={configModalOpen}
                    onClose={() => setConfigModalOpen(false)}
                    copyToClipboard={copyToClipboard}
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
