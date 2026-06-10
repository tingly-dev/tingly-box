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
    const [context1MDialogOpen, setContext1MDialogOpen] = useState(false);
    const [pendingContext1MState, setPendingContext1MState] = useState<boolean | null>(null);

    const handleOpenConfigModal = () => {
        setConfigModalOpen(true);
    };

    const handleContext1MToggle = (newState: boolean) => {
        setPendingContext1MState(newState);
        setContext1MDialogOpen(true);
    };

    const confirmContext1MChange = () => {
        setContext1MDialogOpen(false);
        const message = pendingContext1MState
            ? '1M context window enabled. Model names have been updated with [1m] suffix. Please reapply the configuration to Claude Desktop and restart Claude Desktop for the changes to take effect.'
            : '1M context window disabled. Model names have been updated. Please reapply the configuration to Claude Desktop and restart Claude Desktop for the changes to take effect.';
        showNotification(message, 'success');

        // Auto-open the config panel so user can immediately reapply
        setConfigModalOpen(true);

        setPendingContext1MState(null);
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
                    title="Models and Forwarding Rules"
                    collapsible={true}
                    allowDeleteRule={true}
                    onContext1MToggle={handleContext1MToggle}
                />

                <ClaudeDesktopConfigModal
                    open={configModalOpen}
                    onClose={() => setConfigModalOpen(false)}
                    baseUrl={baseUrl}
                    copyToClipboard={copyToClipboard}
                    rules={rules}
                    onRulesRefresh={loadRules}
                />

                {/* 1M Context Window Toggle Confirmation Dialog */}
                <Dialog open={context1MDialogOpen} onClose={() => setContext1MDialogOpen(false)} maxWidth="sm" fullWidth>
                    <DialogTitle>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <RestartIcon color="primary" />
                            <span>1M Context Window Change</span>
                        </Box>
                    </DialogTitle>
                    <DialogContent>
                        <Alert severity="info" sx={{ mb: 2 }}>
                            {pendingContext1MState
                                ? 'Enabling 1M context window will update model names with [1m] suffix.'
                                : 'Disabling 1M context window will remove [1m] suffix from model names.'}
                        </Alert>
                        <Typography variant="body1" sx={{ mb: 1 }}>
                            You are about to {pendingContext1MState ? 'enable' : 'disable'} the 1M context window feature.
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            This will update model names and requires you to:
                            1. Reapply the configuration to Claude Desktop
                            2. Restart Claude Desktop for changes to take effect
                        </Typography>
                    </DialogContent>
                    <DialogActions sx={{ px: 3, pb: 2, gap: 1, justifyContent: 'flex-end' }}>
                        <Button onClick={() => setContext1MDialogOpen(false)} color="inherit">
                            Cancel
                        </Button>
                        <Button onClick={confirmContext1MChange} variant="contained">
                            Confirm
                        </Button>
                    </DialogActions>
                </Dialog>
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
