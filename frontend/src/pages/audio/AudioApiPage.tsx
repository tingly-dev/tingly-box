import type { ReactNode } from 'react';
import { useState } from 'react';
import { Box, Button, Stack } from '@mui/material';
import { useTranslation } from 'react-i18next';
import PageLayout from '@/components/PageLayout';
import PageHeader from '@/components/PageHeader';
import ProviderConfigCard from '@/components/ProviderConfigCard';
import ScenarioLogDialog from '@/components/RuleLogDialog';
import TemplatePage from '@/pages/scenario/components/TemplatePage';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

interface Props {
    scenario: string;
    title: string;
    children: (models: string[]) => ReactNode;
}

const AudioApiPageContent = ({ scenario, title, children }: Props) => {
    const { t } = useTranslation();
    const [logOpen, setLogOpen] = useState(false);
    const internal = useScenarioPageInternal(scenario);
    const models = internal.rules
        .filter(rule => rule.active !== false && rule.request_model)
        .map(rule => rule.request_model!);
    return (
        <PageLayout loading={internal.isLoading} notification={internal.notification}>
            <Stack spacing={2}>
                <PageHeader title={title} actions={<Button onClick={() => setLogOpen(true)}>{t('agentStatus.log')}</Button>} />
                <Box>
                    <ProviderConfigCard title={title} scenario={scenario} baseUrlPath={`/tingly/${scenario}`}
                        baseUrl={internal.baseUrl} onCopy={internal.copyToClipboard} showPlugins={false} />
                </Box>
                {children(models)}
                <TemplatePage scenario={scenario} title={t('audio.modelRules')} collapsible allowDeleteRule hideTroubleshoot
                    rules={internal.rules} providers={internal.providers} showNotification={internal.showNotification}
                    onRulesChange={internal.handleRulesChange} onProvidersLoad={internal.loadProviders}
                    onRuleDelete={internal.handleRuleDelete} loadRules={internal.loadRules} />
                <ScenarioLogDialog open={logOpen} onClose={() => setLogOpen(false)} scenario={scenario} title={t('agentStatus.log')} />
            </Stack>
        </PageLayout>
    );
};

export default function AudioApiPage(props: Props) {
    return <ScenarioPageModalProvider><AudioApiPageContent {...props} /></ScenarioPageModalProvider>;
}
