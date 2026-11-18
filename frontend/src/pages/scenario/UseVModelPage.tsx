import { AgentPage, type AgentPageDescriptor } from './AgentPage';

const vmodel: AgentPageDescriptor = {
    scenario: 'vmodel',
    title: 'VModel',
    tooltipKey: 'layout.virtualModelsTooltip',
    connection: { compact: true, apiKeyRow: true },
    setup: { kind: 'none' },
    directModels: true,
};

const UseVModelPage: React.FC = () => <AgentPage agent={vmodel} />;

export default UseVModelPage;
