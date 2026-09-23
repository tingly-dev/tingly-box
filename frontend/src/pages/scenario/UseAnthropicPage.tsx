import { ScenarioPage } from './ScenarioPage';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const UseAnthropicPage: React.FC = () => (
    <ScenarioPageModalProvider>
        <ScenarioPage scenario="anthropic" title="Anthropic SDK" />
    </ScenarioPageModalProvider>
);

export default UseAnthropicPage;
