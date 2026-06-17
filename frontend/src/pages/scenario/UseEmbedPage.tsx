import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box } from '@mui/material';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "embed";

const UseEmbedPageContent: React.FC = () => {
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
                            <span>Embed API</span>
                        </Box>
                    }
                    size="full"
                >
                    <ProviderConfigCard
                        title="Embed API"
                        baseUrlPath="/tingly/embed"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        scenario={scenario}
                    />
                </UnifiedCard>
                <TemplatePage
                    scenario={scenario}
                    title="Embedding Model Rules"
                    collapsible={true}
                    allowDeleteRule={true}
                />
            </CardGrid>
        </PageLayout>
    );
};

const UseEmbedPage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UseEmbedPageContent />
        </ScenarioPageModalProvider>
    );
};

export default UseEmbedPage;
