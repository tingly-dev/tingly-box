import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import ImageGenQuickStartDialog from "./components/ImageGenQuickStartDialog";
import ImageGenPlaygroundCard from "./components/ImageGenPlaygroundCard";
import { Box, Button, Tooltip, IconButton } from '@mui/material';
import { Info as InfoIcon } from '@/components/icons';
import { useState } from 'react';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "imagegen";

const UseImageGenPageContent: React.FC = () => {
    const [quickStartOpen, setQuickStartOpen] = useState(false);
    const {
        isLoading,
        notification,
        copyToClipboard,
        baseUrl,
        rules,
        loadingRule,
        showNotification,
        providers,
        loadProviders,
        handleRulesChange,
        handleRuleDelete,
        loadRules,
    } = useScenarioPageInternal(scenario);

    const firstModel = rules.find((rule) => rule.active !== false && rule.request_model)?.request_model;

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
                            onClick={() => setQuickStartOpen(true)}
                            variant="contained"
                            size="small"
                        >
                            Quick Start
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
                <ImageGenPlaygroundCard
                    rules={rules}
                    loadingRules={loadingRule}
                    showNotification={showNotification}
                />
                <TemplatePage
                    scenario={scenario}
                    title="Image Generation Model Rules"
                    collapsible={true}
                    allowDeleteRule={true}
                    rules={rules}
                    providers={providers}
                    showNotification={showNotification}
                    onRulesChange={handleRulesChange}
                    onProvidersLoad={loadProviders}
                    onRuleDelete={handleRuleDelete}
                    loadRules={loadRules}
                />
                <ImageGenQuickStartDialog
                    open={quickStartOpen}
                    onClose={() => setQuickStartOpen(false)}
                    baseUrl={baseUrl}
                    model={firstModel || 'gpt-image-1'}
                    onCopy={copyToClipboard}
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
