import { ScenarioPage } from './ScenarioPage';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const UseCustomPage: React.FC = () => (
    <ScenarioPageModalProvider>
        <ScenarioPage scenario="custom" title="Custom" providerCard={{ compact: true }} />
    </ScenarioPageModalProvider>
);

export default UseCustomPage;
