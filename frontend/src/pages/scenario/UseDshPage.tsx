import { api } from '@/services/api';
import { defaultDshPrefs } from './components/DshQuickConfig';
import DshConfigModal from './components/DshConfigModal';
import { AgentPage, type AgentPageDescriptor } from './AgentPage';

const DSH_REPO_URL = 'https://github.com/deepseek-ai/deepseek-harness';
// dsh serves a local Web UI; tingly-box does not launch it (security), the
// frontend only offers a jump to the default address.
const DSH_WEB_UI_URL = 'http://127.0.0.1:3080';

const dsh: AgentPageDescriptor = {
    scenario: 'dsh',
    title: 'DeepSeek Harness',
    tooltipKey: 'scenarioPage.tooltip.dsh',
    connection: { compact: true, apiKeyRow: true },
    clientConfigTool: 'dsh',
    headerLinks: [{ labelKey: 'scenarioPage.dsh.openWebUi', href: DSH_WEB_UI_URL }],
    setup: {
        kind: 'auto',
        apply: async (t) => {
            try {
                const result = await api.applyDshConfig(defaultDshPrefs() as Record<string, string>);
                if (result.success) {
                    const files: string[] = [];
                    if (result.settingsResult?.created || result.settingsResult?.updated) {
                        files.push('$DSH_HOME/settings.yaml');
                    }
                    if (result.credentialsResult?.created || result.credentialsResult?.updated) {
                        files.push('$DSH_HOME/.credentials.yaml');
                    }
                    return { success: true, files };
                }
                return { success: false, error: result.message || t('scenarioPage.unknownError') };
            } catch (err: any) {
                return { success: false, error: err?.message || t('dshConfig.applyFailed') };
            }
        },
        renderDialog: (slot) => (
            <DshConfigModal
                open={slot.dialogOpen}
                onClose={slot.closeDialog}
                copyToClipboard={slot.copyToClipboard}
                showNotification={slot.showNotification}
            />
        ),
    },
    install: {
        installCommand: 'npx @deepseek-ai/dsh web',
        installDescriptionKey: 'scenarioPage.dsh.installDescription',
        installActions: (t) => [
            { label: t('scenarioPage.dsh.openWebUi'), href: DSH_WEB_UI_URL, variant: 'contained', external: true },
            { label: t('scenarioPage.dsh.viewRepo'), href: DSH_REPO_URL, variant: 'outlined', external: true },
        ],
    },
};

const UseDshPage: React.FC = () => <AgentPage agent={dsh} />;

export default UseDshPage;
