import { AgentPage, type AgentPageDescriptor } from './AgentPage';

const openai: AgentPageDescriptor = {
    scenario: 'openai',
    tooltipKey: 'scenarioPage.tooltip.openai',
    title: 'OpenAI SDK',
    setup: { kind: 'none' },
};

const UseOpenAIPage: React.FC = () => <AgentPage agent={openai} />;

export default UseOpenAIPage;
