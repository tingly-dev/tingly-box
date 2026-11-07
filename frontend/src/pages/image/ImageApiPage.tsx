import CardGrid from "@/components/CardGrid.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import ImageGenQuickStartDialog from "./components/ImageGenQuickStartDialog";
import { Box, Button, Stack } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import PageLayout from '@/components/PageLayout';
import PageHeader from '@/components/PageHeader';
import ScenarioLogDialog from '@/components/RuleLogDialog';
import { AgentStatusRow, AgentLogButton } from '@/pages/scenario/components/AgentStatusRow';
import AgentAccessSection, { useAccessExpanded } from '@/pages/scenario/components/AgentAccessSection';
import { usePluginsActive } from '@/hooks/usePluginsActive';
import PluginFeatures from '@/components/PluginFeatures';
import { useAgentObservation } from '@/pages/scenario/hooks/useAgentObservation';
import ScenarioPageSkeleton from '@/pages/scenario/components/ScenarioPageSkeleton';
import TemplatePage from '@/pages/scenario/components/TemplatePage.tsx';
import { SCENARIO_HEADER_CONTENT_MAX_WIDTH, ScenarioCardHeader, agentPageHeaderSx } from '@/pages/scenario/components/ScenarioCardHeader';
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

    const [logOpen, setLogOpen] = useState(false);
    // Facts, not clicks: connected once a request has come through.
    const observation = useAgentObservation(scenario);
    const launched = !!observation.data?.lastRequestAt;
    const access = useAccessExpanded(scenario, !launched);
    // Whether any plugin is on, independent of the connection card's fold
    // state, so the folded card can surface the row only when it matters.
    const pluginsActive = usePluginsActive(scenario);
    const pluginsRow = <PluginFeatures scenario={scenario} />;

    const firstModel = rules.find((rule) => rule.active !== false && rule.request_model)?.request_model;

    return (
        <PageLayout loading={isLoading} loadingContent={<ScenarioPageSkeleton />} notification={notification}>
            <Stack spacing={2}>
                {/* Same shape as the Agent pages: status under the name, looks-in then the
                    key action (Quick Start) filled and rightmost; the connection folds to
                    one Base URL line once requests come in; plugins are their own card. */}
                <PageHeader
                    title={<ScenarioCardHeader title="Image API" tooltipKey="scenarioPage.tooltip.imagegen" />}
                    meta={(
                        <AgentStatusRow
                            observation={observation.data}
                            connected={launched}
                            configStatus={null}
                            unconnectedText={t('agentStatus.noRequestsYet')}
                        />
                    )}
                    actions={(
                        <>
                            <AgentLogButton onOpenLog={() => setLogOpen(true)} />
                            <Button onClick={() => navigate('/image/playground')} variant="outlined" size="small">
                                {t('image.openPlayground', { defaultValue: 'Try in Playground' })}
                            </Button>
                            <Button onClick={() => setQuickStartOpen(true)} variant="contained" size="small">
                                {t('scenarioPage.quickStart')}
                            </Button>
                        </>
                    )}
                    // One more button than an Agent page, so they share the title's row from lg up.
                    sx={agentPageHeaderSx('lg')}
                />
                <AgentAccessSection
                    expanded={access.expanded}
                    onToggle={access.toggle}
                    url={`${baseUrl}/tingly/${scenario}`}
                    foldedExtras={pluginsActive && pluginsRow}
                >
                    <Box sx={{ maxWidth: SCENARIO_HEADER_CONTENT_MAX_WIDTH }}>
                        <ProviderConfigCard
                            title="Image API"
                            baseUrlPath={`/tingly/${scenario}`}
                            baseUrl={baseUrl}
                            onCopy={copyToClipboard}
                            scenario={scenario}
                            showPlugins={false}
                        />
                        <Box sx={{ px: 2, pt: 1 }}>{pluginsRow}</Box>
                    </Box>
                </AgentAccessSection>
                <CardGrid>
                    <TemplatePage
                        scenario={scenario}
                        title={t('scenarioPage.imageGenModelRules')}
                        collapsible={true}
                        allowDeleteRule={true}
                        allowReorder={true}
                        // The header's Log is the way into this scenario's logs.
                        hideTroubleshoot
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
                    <ScenarioLogDialog open={logOpen} onClose={() => setLogOpen(false)} scenario={scenario} title={t('agentStatus.log')} />
                </CardGrid>
            </Stack>
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
