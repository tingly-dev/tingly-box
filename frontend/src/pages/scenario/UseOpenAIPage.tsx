import { ScenarioPage } from './ScenarioPage';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const UseOpenAIPage: React.FC = () => (
    <ScenarioPageModalProvider>
        <ScenarioPage scenario="openai" title="OpenAI SDK" />
    </ScenarioPageModalProvider>
);

export default UseOpenAIPage;
