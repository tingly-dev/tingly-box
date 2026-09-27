import React from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Box, Typography } from '@mui/material';
import { ScenarioPage } from './ScenarioPage';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';
import UnifiedCard from '@/components/UnifiedCard';
import CodeBlock from '@/components/CodeBlock';

const UseDecisionPage: React.FC = () => {
    const { t } = useTranslation();
    return (
        <ScenarioPageModalProvider>
            <ScenarioPage
                scenario="decision"
                title="Decision API"
                tooltipKey="scenarioPage.tooltip.decision"
                templateTitle={t('scenarioPage.decisionModelRules')}
                children={(slot) => (
                    <UnifiedCard title={t('scenarioPage.decisionQuickStart')} size="full">
                        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                            <Typography variant="body2" color="text.secondary">
                                {t('scenarioPage.decisionQuickStartHint')}
                            </Typography>
                            <CodeBlock
                                language="bash"
                                code={`curl ${slot.baseUrl}/tingly/decision/v1/decisions \\
  -H "Authorization: Bearer $TINGLY_MODEL_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "jev-1",
    "state": { "task": "Choose a rollout" },
    "questions": {
      "route": {
        "type": "choice",
        "instructions": "Which rollout should we use?",
        "criteria": { "canary": "Start small", "full": "Deploy to everyone" }
      }
    }
  }'`}
                            />
                            <Alert severity="info" sx={{ py: 1 }}>
                                {t('scenarioPage.decisionAdvisory')}
                            </Alert>
                        </Box>
                    </UnifiedCard>
                )}
            />
        </ScenarioPageModalProvider>
    );
};

export default UseDecisionPage;
