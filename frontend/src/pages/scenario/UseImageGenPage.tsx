import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box, Button } from '@mui/material';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import { useNavigate } from 'react-router-dom';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "imagegen";

const UseImageGenPageContent: React.FC = () => {
    const {
        isLoading,
        notification,
        copyToClipboard,
        baseUrl,
    } = useScenarioPageInternal(scenario);
    const navigate = useNavigate();

    return (
        <PageLayout loading={isLoading} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <span>Image Generation API Configuration</span>
                        </Box>
                    }
                    size="full"
                    rightAction={
                        <Button
                            onClick={() => navigate('/agent/playground')}
                            variant="contained"
                            size="small"
                            startIcon={<PlayArrowIcon />}
                        >
                            Open Playground
                        </Button>
                    }
                >
                    <ProviderConfigCard
                        title="Image Generation API Configuration"
                        baseUrlPath="/tingly/imagegen"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                    />
                </UnifiedCard>
                <TemplatePage
                    scenario={scenario}
                    title="Image Generation Models and Forwarding Rules"
                    collapsible={true}
                    allowDeleteRule={true}
                />
            </CardGrid>
        </PageLayout>
    );
};

const UseImageGenPage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UseImageGenPageContent />
        </ScenarioPageModalProvider>
    );
};

export default UseImageGenPage;
