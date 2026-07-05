import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box, Button } from '@mui/material';
import { Key as IconKey } from '@/components/icons';
import { useState } from 'react';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import SharingKeysDialog from './components/SharingKeysDialog.tsx';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "team";

const UseTeamPageContent: React.FC = () => {
    const {
        isLoading,
        notification,
        copyToClipboard,
        baseUrl,
    } = useScenarioPageInternal(scenario);

    const [sharingKeysOpen, setSharingKeysOpen] = useState(false);

    return (
        <PageLayout loading={isLoading} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <span>Team</span>
                        </Box>
                    }
                    size="full"
                    rightAction={
                        <Button
                            variant="contained"
                            size="small"
                            startIcon={<IconKey sx={{ fontSize: 18 }} />}
                            onClick={() => setSharingKeysOpen(true)}
                        >
                            Sharing Keys
                        </Button>
                    }
                >
                    <ProviderConfigCard
                        title="Team"
                        baseUrlPath="/tingly/team"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        compact={true}
                        scenario={scenario}
                    />
                </UnifiedCard>
                <TemplatePage
                    scenario={scenario}
                    collapsible={true}
                    allowDeleteRule={true}
                />
            </CardGrid>

            <SharingKeysDialog
                open={sharingKeysOpen}
                onClose={() => setSharingKeysOpen(false)}
            />
        </PageLayout>
    );
};

const UseTeamPage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UseTeamPageContent />
        </ScenarioPageModalProvider>
    );
};

export default UseTeamPage;
