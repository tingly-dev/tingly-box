import { AgentPage, type AgentPageDescriptor } from './AgentPage';

const anthropic: AgentPageDescriptor = {
    scenario: 'anthropic',
    tooltipKey: 'scenarioPage.tooltip.anthropic',
    title: 'Anthropic SDK',
    setup: { kind: 'none' },
};

const UseAnthropicPage: React.FC = () => <AgentPage agent={anthropic} />;

export default UseAnthropicPage;
