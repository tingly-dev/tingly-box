import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box } from '@mui/material';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "agent";

const UseAgentPageContent: React.FC = () => {
    const {
        isLoading,
        notification,
        copyToClipboard,
        baseUrl,
    } = useScenarioPageInternal(scenario);

    return (
        <PageLayout loading={isLoading} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <span>Claw | Agent</span>
                        </Box>
                    }
                    size="full"
                >
                    <ProviderConfigCard
                        title="Claw | Agent"
                        baseUrlPath="/tingly/agent"
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
        </PageLayout>
    );
};

const UseAgentPage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UseAgentPageContent />
        </ScenarioPageModalProvider>
    );
};

export default UseAgentPage;
