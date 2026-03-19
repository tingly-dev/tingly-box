import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box, Button, Tooltip, IconButton } from '@mui/material';
import InfoIcon from '@mui/icons-material/Info';
import { useState } from 'react';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import XcodeConfigModal from '@/components/XcodeConfigModal';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';

const scenario = "xcode";

const UseXcodePage: React.FC = () => {
    const {
        showTokenModal,
        setShowTokenModal,
        token,
        isLoading,
        notification,
        copyToClipboard,
        baseUrl,
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
                            <span>Xcode Configuration</span>
                            <Tooltip title={`Base URL: ${baseUrl}/tingly/xcode`}>
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
                            Config Guide
                        </Button>
                    }
                >
                    <ProviderConfigCard
                        title="Xcode Configuration"
                        baseUrlPath="/tingly/xcode"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        token={token}
                        onShowTokenModal={() => setShowTokenModal(true)}
                        scenario={scenario}
                        showApiKeyRow={true}
                    />
                </UnifiedCard>

                <TemplatePage
                    scenario={scenario}
                    title="Models and Forwarding Rules"
                    collapsible={true}
                    allowDeleteRule={true}
                />

                <XcodeConfigModal
                    open={configModalOpen}
                    onClose={() => setConfigModalOpen(false)}
                    baseUrl={baseUrl}
                    token={token}
                    copyToClipboard={copyToClipboard}
                />
            </CardGrid>
        </PageLayout>
    );
};

export default UseXcodePage;
