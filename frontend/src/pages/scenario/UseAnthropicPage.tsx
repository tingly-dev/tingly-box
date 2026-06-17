import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box } from '@mui/material';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';
const scenario = "anthropic";
const UseAnthropicPageContent: React.FC = () => {
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
                            <span>Anthropic SDK</span>
                        </Box>
                    }
                    size="full"
                >
                    <ProviderConfigCard
                        title="Anthropic SDK"
                        baseUrlPath="/tingly/anthropic"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
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
const UseAnthropicPage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UseAnthropicPageContent />
        </ScenarioPageModalProvider>
    );
};
export default UseAnthropicPage;
