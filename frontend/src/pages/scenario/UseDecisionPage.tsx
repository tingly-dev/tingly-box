import React from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Box, Typography } from '@mui/material';
import { AgentPage, type AgentPageDescriptor } from './AgentPage';
import UnifiedCard from '@/components/UnifiedCard';
import CodeBlock from '@/components/CodeBlock';

/** The try-it curl example: the decision protocol is not spoken by agent SDKs,
 * so the page has to show the request shape itself. */
const DecisionExampleCard: React.FC<{ baseUrl: string }> = ({ baseUrl }) => {
    const { t } = useTranslation();
    return (
        <UnifiedCard title={t('scenarioPage.decisionQuickStart')} size="full">
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <Typography variant="body2" color="text.secondary">
                    {t('scenarioPage.decisionQuickStartHint')}
                </Typography>
                <CodeBlock
                    language="bash"
                    code={`curl ${baseUrl}/tingly/decision/v1/decisions \\
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
    );
};

const decision: AgentPageDescriptor = {
    scenario: 'decision',
    title: 'Decision API',
    tooltipKey: 'scenarioPage.tooltip.decision',
    rulesTitleKey: 'scenarioPage.decisionModelRules',
    setup: { kind: 'none' },
    extraContent: DecisionExampleCard,
};

const UseDecisionPage: React.FC = () => <AgentPage agent={decision} />;

export default UseDecisionPage;
