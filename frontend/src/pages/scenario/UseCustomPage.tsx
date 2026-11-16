import { AgentPage, type AgentPageDescriptor } from './AgentPage';

const custom: AgentPageDescriptor = {
    scenario: 'custom',
    tooltipKey: 'scenarioPage.tooltip.custom',
    title: 'Custom',
    connection: { compact: true },
    setup: { kind: 'none' },
};

const UseCustomPage: React.FC = () => <AgentPage agent={custom} />;

export default UseCustomPage;
