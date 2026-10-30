import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import OpenCodeConfigModal from './components/OpenCodeConfigModal';
import { AgentPage, type AgentPageDescriptor, type AgentPageSlot } from './AgentPage';

/** OpenCode's dialog shows the config the gateway would write, fetched each time it opens. */
const OpenCodeSetupDialog: React.FC<{ slot: AgentPageSlot }> = ({ slot }) => {
    const { t } = useTranslation();
    const [configJson, setConfigJson] = useState('');
    const [scriptWindows, setScriptWindows] = useState('');
    const [scriptUnix, setScriptUnix] = useState('');
    const [isConfigLoading, setIsConfigLoading] = useState(false);
    const { dialogOpen: open, showNotification } = slot;

    useEffect(() => {
        if (!open) return;
        let cancelled = false;
        const show = (json: string, win: string, unix: string) => {
            if (cancelled) return;
            setConfigJson(json);
            setScriptWindows(win);
            setScriptUnix(unix);
        };
        show('// Loading...', '// Loading...', '// Loading...');
        setIsConfigLoading(true);
        api.getOpenCodeConfigPreview().then((result) => {
            if (result.success) {
                show(result.configJson, result.scriptWindows, result.scriptUnix);
            } else {
                show('// Error: ' + (result.message || 'Failed to load config'), '// Error loading config', '// Error: Failed to connect to server');
                showNotification(t('scenarioPage.opencode.previewFailed', { reason: result.message || t('scenarioPage.unknownError') }), 'error');
            }
        }).catch((err) => {
            console.error('Failed to fetch config preview:', err);
            show('// Error: Failed to connect to server', '// Error: Failed to connect to server', '// Error: Failed to connect to server');
            showNotification(t('scenarioPage.opencode.previewFailedGeneric'), 'error');
        }).finally(() => {
            if (!cancelled) setIsConfigLoading(false);
        });
        return () => { cancelled = true; };
        // eslint-disable-next-line react-hooks/exhaustive-deps -- refetch per open, not per render
    }, [open]);

    return (
        <OpenCodeConfigModal
            open={open}
            onClose={slot.closeDialog}
            generateConfigJson={() => configJson}
            generateScriptWindows={() => scriptWindows}
            generateScriptUnix={() => scriptUnix}
            copyToClipboard={slot.copyToClipboard}
            onApply={async () => {
                const result = await slot.apply();
                if (result.success) {
                    slot.closeApplied(result);
                } else {
                    slot.showNotification(result.error || t('scenarioPage.opencode.applyFailed'), 'error');
                }
            }}
            isApplyLoading={slot.isApplyLoading}
            isLoading={isConfigLoading}
        />
    );
};

const opencode: AgentPageDescriptor = {
    scenario: 'opencode',
    title: 'OpenCode',
    tooltipKey: 'scenarioPage.tooltip.opencode',
    connection: { titleKey: 'scenarioPage.opencode.configTitle', compact: true, apiKeyRow: true },
    setup: {
        kind: 'auto',
        apply: async (t) => {
            try {
                const result = await api.applyOpenCodeConfig();
                if (result.success) {
                    return { success: true, files: ['~/.config/opencode/opencode.json'] };
                }
                return { success: false, error: result.message || t('scenarioPage.unknownError') };
            } catch {
                return { success: false, error: t('scenarioPage.opencode.applyFailed') };
            }
        },
        renderDialog: (slot) => <OpenCodeSetupDialog slot={slot} />,
    },
    install: {
        installCommand: 'npm install -g opencode-ai',
        installMirrorCommand: 'npm install -g opencode-ai --registry=https://registry.npmmirror.com',
    },
};

const UseOpenCodePage: React.FC = () => <AgentPage agent={opencode} />;

export default UseOpenCodePage;
