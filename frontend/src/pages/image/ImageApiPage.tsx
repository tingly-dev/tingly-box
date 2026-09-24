import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import ImageGenQuickStartDialog from "./components/ImageGenQuickStartDialog";
import { Button, Stack } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import PageLayout from '@/components/PageLayout';
import ScenarioPageSkeleton from '@/pages/scenario/components/ScenarioPageSkeleton';
import TemplatePage from '@/pages/scenario/components/TemplatePage.tsx';
import { SCENARIO_HEADER_CONTENT_MAX_WIDTH, ScenarioCardHeader } from '@/pages/scenario/ScenarioPage';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "imagegen";

// Image API: how a client reaches /tingly/imagegen (base URL, quick start)
// and which models it routes to. The Playground — the day-to-day work
// surface — lives on its own page under the same Image rail item and runs
// through exactly these rules. See .design/image-layout.md.
const ImageApiPageContent: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const [quickStartOpen, setQuickStartOpen] = useState(false);
    const {
        isLoading,
        notification,
        copyToClipboard,
        baseUrl,
        rules,
        showNotification,
        providers,
        loadProviders,
        handleRulesChange,
        handleRuleDelete,
        loadRules,
    } = useScenarioPageInternal(scenario);

    const firstModel = rules.find((rule) => rule.active !== false && rule.request_model)?.request_model;

    return (
        <PageLayout loading={isLoading} loadingContent={<ScenarioPageSkeleton />} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    titleHeadingLevel={1}
                    title={
                        <ScenarioCardHeader title="Image API" tooltipKey="scenarioPage.tooltip.imagegen" />
                    }
                    size="full"
                    contentMaxWidth={SCENARIO_HEADER_CONTENT_MAX_WIDTH}
                    rightAction={
                        <Stack direction="row" spacing={1}>
                            <Button
                                onClick={() => navigate('/image/playground')}
                                variant="outlined"
                                size="small"
                            >
                                {t('image.openPlayground', { defaultValue: 'Try in Playground' })}
                            </Button>
                            <Button
                                onClick={() => setQuickStartOpen(true)}
                                variant="contained"
                                size="small"
                            >
                                {t('scenarioPage.quickStart')}
                            </Button>
                        </Stack>
                    }
                >
                    <ProviderConfigCard
                        title="Image API"
                        baseUrlPath="/tingly/imagegen"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        scenario={scenario}
                    />
                </UnifiedCard>
                <TemplatePage
                    scenario={scenario}
                    title={t('scenarioPage.imageGenModelRules')}
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

const ImageApiPage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <ImageApiPageContent />
        </ScenarioPageModalProvider>
    );
};

export default ImageApiPage;
