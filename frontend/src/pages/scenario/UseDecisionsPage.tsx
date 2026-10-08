import DecisionsGuideModal from './components/DecisionsGuideModal';
import { AgentPage, type AgentPageDescriptor } from './AgentPage';

const decisions: AgentPageDescriptor = {
    scenario: 'decisions',
    title: 'Decisions API',
    rulesTitleKey: 'scenarioPage.decisionsModelRules',
    connection: { compact: true, apiKeyRow: true, baseUrlRow: true },
    setup: {
        kind: 'guide',
        renderDialog: (slot) => (
            <DecisionsGuideModal
                open={slot.dialogOpen}
                onClose={slot.closeDialog}
                baseUrl={slot.baseUrl}
                model={slot.rules.find((r) => r.request_model && r.request_model !== '*')?.request_model}
                copyToClipboard={slot.copyToClipboard}
            />
        ),
    },
};

const UseDecisionsPage: React.FC = () => <AgentPage agent={decisions} />;

export default UseDecisionsPage;
