import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box, Button, Tooltip, IconButton } from '@mui/material';
import { Info as InfoIcon } from '@/components/icons';
import { useState } from 'react';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import ClaudeDesktopConfigModal from '@/components/ClaudeDesktopConfigModal';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "claude_desktop";

const UseClaudeDesktopPageContent: React.FC = () => {
    const {
        isLoading,
        notification,
        copyToClipboard,
        baseUrl,
        rules,
        loadRules,
    } = useScenarioPageInternal(scenario);

    const [configModalOpen, setConfigModalOpen] = useState(false);

    const handleOpenConfigModal = () => {
        setConfigModalOpen(true);
    };

    return (
        <PageLayout loading={isLoading} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <span>Claude Desktop Configuration</span>
                            <Tooltip title={`Base URL: ${baseUrl}/tingly/claude_desktop`}>
                                <IconButton size="small" sx={{ ml: 0.5 }}>
                                    <InfoIcon fontSize="small" sx={{ color: 'text.secondary' }} />
                                </IconButton>
                            </Tooltip>
                        </Box>
                    }
                    size="full"
                    rightAction={
                        <Button
                            onClick={handleOpenConfigModal}
                            variant="contained"
                            size="small"
                        >
                            Config
                        </Button>
                    }
                >
                    <ProviderConfigCard
                        title="Claude Desktop Configuration"
                        baseUrlPath="/tingly/claude_desktop"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        scenario={scenario}
                        showApiKeyRow={true}
                        compact={false}
                    />
                </UnifiedCard>

                <TemplatePage
                    scenario={scenario}
                    title="Models and Forwarding Rules"
                    collapsible={true}
                    allowDeleteRule={true}
                />

                <ClaudeDesktopConfigModal
                    open={configModalOpen}
                    onClose={() => setConfigModalOpen(false)}
                    baseUrl={baseUrl}
                    copyToClipboard={copyToClipboard}
                    rules={rules}
                    onRulesRefresh={loadRules}
                />
            </CardGrid>
        </PageLayout>
    );
};

const UseClaudeDesktopPage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UseClaudeDesktopPageContent />
        </ScenarioPageModalProvider>
    );
};

export default UseClaudeDesktopPage;
