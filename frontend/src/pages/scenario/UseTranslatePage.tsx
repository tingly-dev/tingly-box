import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import TranslateQuickStartCard from "@/components/TranslateQuickStartCard.tsx";
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "translate";

const UseTranslatePageContent: React.FC = () => {
    const {
        isLoading,
        notification,
        copyToClipboard,
        baseUrl,
        rules,
    } = useScenarioPageInternal(scenario);

    const firstModel = rules?.find((r: any) => !r?.disabled && r?.request_model)?.request_model;

    return (
        <PageLayout loading={isLoading} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    title="Translation API Configuration"
                    size="full"
                >
                    <ProviderConfigCard
                        title="Translation API Configuration"
                        baseUrlPath="/tingly/translate"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                    />
                </UnifiedCard>
                <TranslateQuickStartCard
                    baseUrl={baseUrl}
                    model={firstModel || 'Helsinki-NLP/opus-mt-en-zh'}
                    onCopy={copyToClipboard}
                />
                <TemplatePage
                    scenario={scenario}
                    title="Translation Models and Forwarding Rules"
                    collapsible={true}
                    allowDeleteRule={true}
                />
            </CardGrid>
        </PageLayout>
    );
};

const UseTranslatePage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UseTranslatePageContent />
        </ScenarioPageModalProvider>
    );
};

export default UseTranslatePage;
