import { api } from '@/services/api';
import CodexConfigModal from './components/CodexConfigModal';
import { defaultCodexPrefs } from './components/CodexQuickConfig';
import { AgentPage, type AgentPageDescriptor } from './AgentPage';

const codex: AgentPageDescriptor = {
    scenario: 'codex',
    title: 'Codex',
    tooltipKey: 'scenarioPage.tooltip.codex',
    connection: { titleKey: 'scenarioPage.codex.configTitle', compact: true },
    clientConfigTool: 'codex',
    context1M: true,
    setup: {
        kind: 'auto',
        apply: async (t) => {
            try {
                const result = await api.applyCodexConfig(defaultCodexPrefs() as Record<string, string>);
                if (result.success) {
                    const files: string[] = [];
                    if (result.configResult?.created || result.configResult?.updated) {
                        files.push('~/.codex/config.toml');
                    }
                    if (result.authResult?.created || result.authResult?.updated) {
                        files.push('~/.codex/auth.json');
                    }
                    return { success: true, files };
                }
                return { success: false, error: result.message || t('scenarioPage.unknownError') };
            } catch (err: any) {
                return { success: false, error: err?.message || t('scenarioPage.codex.applyFailed') };
            }
        },
        renderDialog: (slot) => (
            <CodexConfigModal
                open={slot.dialogOpen}
                onClose={slot.closeDialog}
                copyToClipboard={slot.copyToClipboard}
                showNotification={slot.showNotification}
                pendingContext1MChange={slot.pendingContext1MChange}
            />
        ),
    },
    install: {
        connectGuide: true,
        siteUrl: 'https://github.com/openai/codex',
        installCommand: 'npm install -g @openai/codex',
        installMirrorCommand: 'npm install -g @openai/codex --registry=https://registry.npmmirror.com',
    },
};

const UseCodexPage: React.FC = () => <AgentPage agent={codex} />;

export default UseCodexPage;
