import { AgentPage, type AgentPageDescriptor } from './AgentPage';

const decisions: AgentPageDescriptor = {
    scenario: 'decisions',
    title: 'Decisions API',
    rulesTitleKey: 'scenarioPage.decisionsModelRules',
    setup: { kind: 'none' },
};

const UseDecisionsPage: React.FC = () => <AgentPage agent={decisions} />;

export default UseDecisionsPage;
