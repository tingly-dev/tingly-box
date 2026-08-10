import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box } from '@mui/material';
import PageLayout from '@/components/PageLayout';
import ScenarioPageSkeleton from './components/ScenarioPageSkeleton';
import TemplatePage from './components/TemplatePage.tsx';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "custom";

const UseCustomPageContent: React.FC = () => {
    const {
        isLoading,
        notification,
        copyToClipboard,
        baseUrl,
    } = useScenarioPageInternal(scenario);

    return (
        <PageLayout loading={isLoading} loadingContent={<ScenarioPageSkeleton />} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    titleHeadingLevel={1}
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <span>Custom</span>
                        </Box>
                    }
                    size="full"
                >
                    <ProviderConfigCard
                        title="Custom"
                        baseUrlPath="/tingly/custom"
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

const UseCustomPage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UseCustomPageContent />
        </ScenarioPageModalProvider>
    );
};

export default UseCustomPage;
