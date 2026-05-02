import CardGrid from "@/components/CardGrid.tsx";
import AgentSetupCard, { type AgentApplyResult } from '@/components/AgentSetupCard';
import CodexConfigModal from "@/components/CodexConfigModal.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box, Button, IconButton, Tooltip } from '@mui/material';
import InfoIcon from '@mui/icons-material/Info';
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
    } = useScenarioPageInternal(scenario);

    const [configModalOpen, setConfigModalOpen] = useState(false);

    // Codex has no backend apply — copy config.toml to clipboard as a convenience
    const handleApply = async (): Promise<AgentApplyResult> => {
        const codexBaseUrl = `${baseUrl}/tingly/codex`;
        const config = `model = "tingly-codex"\nmodel_provider = "tingly-box"\n\n[model_providers.tingly-box]\nname = "OpenAI using Tingly Box"\nbase_url = "${codexBaseUrl}"\npreferred_auth_method = "apikey"\nwire_api = "responses"`;
        await navigator.clipboard.writeText(config);
        return {
            success: true,
            files: ['~/.codex/config.toml (copied to clipboard — paste manually)'],
        };
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
                            Config Codex
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
                    onViewConfig={() => setConfigModalOpen(true)}
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
                    baseUrl={baseUrl}
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
