import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import AgentSetupCard, { hasModelOnAnyRule, scrollToModelsCard } from '@/components/AgentSetupCard';
import { Box, Button, Tooltip, IconButton } from '@mui/material';
import { Info as InfoIcon } from '@/components/icons';
import { useState } from 'react';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import VSCodeConfigModal from '@/components/VSCodeConfigModal';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const scenario = "vscode";

const MARKETPLACE_URL = 'https://marketplace.visualstudio.com/items?itemName=Tingly-Dev.vscode-tingly-box';
const VSCODE_INSTALL_URL = 'vscode:extension/Tingly-Dev.vscode-tingly-box';

const UseVSCodePageContent: React.FC = () => {
    const {
        isLoading,
        notification,
        copyToClipboard,
        baseUrl,
        rules,
    } = useScenarioPageInternal(scenario);

    const [configModalOpen, setConfigModalOpen] = useState(false);

    const handleOpenConfigModal = () => {
        setConfigModalOpen(true);
    };

    return (
        <PageLayout loading={isLoading} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    title="VS Code Copilot"
                    size="full"
                    rightAction={
                        <Button
                            onClick={handleOpenConfigModal}
                            variant="contained"
                            size="small"
                        >
                            Config
                        </Button>
                    }
                >
                    <ProviderConfigCard
                        title="VSCode Copliot Chat"
                        baseUrlPath="/tingly/vscode"
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        scenario={scenario}
                        showApiKeyRow={true}
                        compact={false}
                    />
                </UnifiedCard>

                <AgentSetupCard
                    agentKey={scenario}
                    agentName="VS Code"
                    installCommand=""
                    installStepDescription="Install the Tingly Box extension from VS Code or the Marketplace."
                    installActions={[
                        { label: 'Install in VS Code', href: VSCODE_INSTALL_URL, variant: 'contained' },
                        { label: 'View Marketplace', href: MARKETPLACE_URL, variant: 'outlined', external: true },
                    ]}
                    onViewConfig={handleOpenConfigModal}
                    applyStepLabel="Follow VS Code Guide"
                    applyStepDescription="Open the Tingly Box extension in VS Code and follow its built-in setup guide."
                    viewConfigButtonLabel="Open Guide"
                    hasModelSelected={hasModelOnAnyRule(rules)}
                    onSelectModel={scrollToModelsCard}
                />

                <TemplatePage
                    scenario={scenario}
                    title="Models and Forwarding Rules"
                    collapsible={true}
                    allowDeleteRule={true}
                />

                <VSCodeConfigModal
                    open={configModalOpen}
                    onClose={() => setConfigModalOpen(false)}
                />
            </CardGrid>
        </PageLayout>
    );
};

const UseVSCodePage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UseVSCodePageContent />
        </ScenarioPageModalProvider>
    );
};

export default UseVSCodePage;
