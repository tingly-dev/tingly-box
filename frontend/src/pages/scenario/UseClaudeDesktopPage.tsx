import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box, Button, Tooltip, IconButton, Dialog, DialogActions, DialogContent, DialogTitle, Typography, Alert } from '@mui/material';
import { Info as InfoIcon, Refresh as RestartIcon } from '@/components/icons';
import { useState } from 'react';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import ClaudeDesktopConfigModal from './components/ClaudeDesktopConfigModal';
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
        showNotification,
    } = useScenarioPageInternal(scenario);
    const [configModalOpen, setConfigModalOpen] = useState(false);
    const [pendingContext1MChange, setPendingContext1MChange] = useState<boolean | null>(null);
    const handleOpenConfigModal = () => {
        setConfigModalOpen(true);
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
                            <span>Claude Desktop</span>
                            <Tooltip title="Claude Desktop app API proxy for AI assistance in desktop environment">
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
                        title="Claude Desktop"
                        baseUrlPath="/tingly/claude_desktop"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        scenario={scenario}
                        showApiKeyRow={true}
                        showBaseUrlRow={true}
                        compact={true}
                    />
                </UnifiedCard>
                <TemplatePage
                    scenario={scenario}
                    collapsible={true}
                    allowDeleteRule={true}
                    onContext1MToggle={handleContext1MToggle}
                />
                <ClaudeDesktopConfigModal
                    open={configModalOpen}
                    onClose={() => {
                        setConfigModalOpen(false);
                        setPendingContext1MChange(null);
                    }}
                    baseUrl={baseUrl}
                    copyToClipboard={copyToClipboard}
                    rules={rules}
                    onRulesRefresh={loadRules}
                    pendingContext1MChange={pendingContext1MChange}
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
