import { AgentPage, type AgentPageDescriptor } from './AgentPage';

const embed: AgentPageDescriptor = {
    scenario: 'embed',
    tooltipKey: 'scenarioPage.tooltip.embed',
    title: 'Embed API',
    rulesTitleKey: 'scenarioPage.embedModelRules',
    setup: { kind: 'none' },
};

const UseEmbedPage: React.FC = () => <AgentPage agent={embed} />;

export default UseEmbedPage;
