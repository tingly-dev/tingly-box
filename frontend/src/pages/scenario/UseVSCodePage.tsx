import VSCodeConfigModal from './components/VSCodeConfigModal';
import { AgentPage, type AgentPageDescriptor } from './AgentPage';

const MARKETPLACE_URL = 'https://marketplace.visualstudio.com/items?itemName=Tingly-Dev.vscode-tingly-box';
const VSCODE_INSTALL_URL = 'vscode:extension/Tingly-Dev.vscode-tingly-box';

const vscode: AgentPageDescriptor = {
    scenario: 'vscode',
    title: 'VS Code',
    tooltipKey: 'scenarioPage.tooltip.vscode',
    connection: { compact: true, apiKeyRow: true, baseUrlRow: true },
    setup: {
        kind: 'guide',
        renderDialog: (slot) => <VSCodeConfigModal open={slot.dialogOpen} onClose={slot.closeDialog} />,
    },
    install: {
        installDescriptionKey: 'scenarioPage.vscode.installDescription',
        // Outlined, not contained: the step row's "I've installed it" is the
        // one contained button in this step.
        installActions: (t) => [
            { label: t('scenarioPage.vscode.installInVSCode'), href: VSCODE_INSTALL_URL, variant: 'outlined' },
            { label: t('scenarioPage.vscode.viewMarketplace'), href: MARKETPLACE_URL, variant: 'outlined', external: true },
        ],
    },
};

const UseVSCodePage: React.FC = () => <AgentPage agent={vscode} />;

export default UseVSCodePage;
