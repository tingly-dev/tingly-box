import { useTranslation } from 'react-i18next';
import { ScenarioPage } from './ScenarioPage';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const UseEmbedPage: React.FC = () => {
    const { t } = useTranslation();
    return (
        <ScenarioPageModalProvider>
            <ScenarioPage
                scenario="embed"
                title="Embed API"
                templateTitle={t('scenarioPage.embedModelRules')}
            />
        </ScenarioPageModalProvider>
    );
};

export default UseEmbedPage;
