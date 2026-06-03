import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import ImageGenQuickStartCard from "./components/ImageGenQuickStartCard";
import { Box, Button, Tooltip, IconButton } from '@mui/material';
import { PlayArrow as PlayArrowIcon, Info as InfoIcon } from '@/components/icons';
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
        rules,
    } = useScenarioPageInternal(scenario);
    const navigate = useNavigate();

    const firstModel = rules?.find((r: any) => !r?.disabled && r?.request_model)?.request_model;

    return (
        <PageLayout loading={isLoading} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <span>Image Generation API</span>
                            <Tooltip title="AI-powered image generation through Tingly Box proxy with multiple model support">
                                <IconButton size="small" sx={{ ml: 0.5 }}>
                                    <InfoIcon fontSize="small" sx={{ color: 'text.secondary' }} />
                                </IconButton>
                            </Tooltip>
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
                        title="Image Generation API"
                        baseUrlPath="/tingly/imagegen"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        scenario={scenario}
                    />
                </UnifiedCard>
                <ImageGenQuickStartCard
                    baseUrl={baseUrl}
                    model={firstModel || 'gpt-image-1'}
                    onCopy={copyToClipboard}
                />
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
