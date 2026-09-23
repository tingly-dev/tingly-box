import CardGrid from '@/components/CardGrid';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { Alert, Box, Typography } from '@mui/material';
import TemplatePage from './components/TemplatePage';
import ScenarioPageSkeleton from './components/ScenarioPageSkeleton';
import { ScenarioPageModalProvider } from './context/ScenarioPageContext';
import { useScenarioPageInternal } from './hooks/useScenarioPageInternal';

const scenario = 'decision';

function UseDecisionPageContent() {
    const { isLoading, notification, baseUrl } = useScenarioPageInternal(scenario);
    const endpoint = `${baseUrl}/tingly/decision/v1/decisions`;

    return (
        <PageLayout loading={isLoading} loadingContent={<ScenarioPageSkeleton />} notification={notification}>
            <CardGrid>
                <UnifiedCard title="Decision API" titleHeadingLevel={1} size="full">
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                        <Typography>
                            Send Jev-compatible typed choice, score, and noul questions without converting them to chat messages.
                        </Typography>
                        <Alert severity="info">
                            Decision probabilities are advisory and do not replace permissions or human approval for irreversible actions.
                        </Alert>
                        <Box component="pre" sx={{ overflowX: 'auto', p: 2, m: 0, borderRadius: 1, bgcolor: 'action.hover' }}>
{`curl ${endpoint} \\
  -H "Authorization: Bearer $TINGLY_MODEL_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "typesafe/jev-1.13",
    "state": {"task": "Choose a safe rollout"},
    "questions": {
      "route": {
        "type": "choice",
        "instructions": "Which rollout should we use?",
        "criteria": {"canary": "Start small", "full": "Deploy to everyone"}
      }
    }
  }'`}
                        </Box>
                    </Box>
                </UnifiedCard>
                <TemplatePage scenario={scenario} title="Decision model rules" collapsible allowDeleteRule />
            </CardGrid>
        </PageLayout>
    );
}

export default function UseDecisionPage() {
    return <ScenarioPageModalProvider><UseDecisionPageContent /></ScenarioPageModalProvider>;
}
