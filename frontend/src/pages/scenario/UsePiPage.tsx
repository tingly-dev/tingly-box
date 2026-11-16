import PiConfigModal from './components/PiConfigModal';
import { AgentPage, type AgentPageDescriptor } from './AgentPage';

const PI_REPO_URL = 'https://github.com/earendil-works/pi';

const pi: AgentPageDescriptor = {
    scenario: 'pi',
    title: 'Pi',
    tooltipKey: 'scenarioPage.tooltip.pi',
    connection: { compact: true, apiKeyRow: true },
    setup: {
        kind: 'guide',
        renderDialog: (slot) => <PiConfigModal open={slot.dialogOpen} onClose={slot.closeDialog} />,
    },
    install: {
        installDescriptionKey: 'scenarioPage.pi.installDescription',
        installActions: (t) => [
            { label: t('scenarioPage.pi.viewRepo'), href: PI_REPO_URL, variant: 'outlined', external: true },
        ],
    },
};

const UsePiPage: React.FC = () => <AgentPage agent={pi} />;

export default UsePiPage;
