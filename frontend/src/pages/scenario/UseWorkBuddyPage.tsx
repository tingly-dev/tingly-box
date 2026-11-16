import { useEffect, useState } from 'react';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import WorkBuddyConfigModal from './components/WorkBuddyConfigModal';
import { AgentPage, type AgentPageDescriptor, type AgentPageSlot } from './AgentPage';
import type { AgentApplyResult } from './components/agentSetup';

const WORKBUDDY_DOCS_URL =
    'https://www.workbuddy.ai/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Model';
/** One installed edition's models.json (~/.workbuddy for China, ~/.workbuddy-ai for international). */
type WorkBuddyFileResult = { path: string; success: boolean; created?: boolean; updated?: boolean; backupPath?: string };

const pathsWhere = (files: WorkBuddyFileResult[], pick: (f: WorkBuddyFileResult) => unknown) => {
    const out = files.filter(pick).map((f) => f.path);
    return out.length ? out : undefined;
};

/** Merge the WorkBuddy rules' models into each installed edition's models.json; `onSkipped` reports ids the user already owns. */
const applyWorkBuddy = async (
    t: TFunction,
    onSkipped?: (ids: string[]) => void,
): Promise<AgentApplyResult> => {
    try {
        const result = await api.applyWorkBuddyConfig();
        if (!result.success) {
            return { success: false, error: result.message || t('scenarioPage.unknownError') };
        }
        if (result.skipped?.length) onSkipped?.(result.skipped);
        const files: WorkBuddyFileResult[] = result.files ?? [];
        return {
            success: true,
            files: files.map((f) => f.path),
            createdFiles: pathsWhere(files, (f) => f.created),
            updatedFiles: pathsWhere(files, (f) => f.updated),
            backupPaths: files.flatMap((f) => (f.backupPath ? [f.backupPath] : [])),
        };
    } catch {
        return { success: false, error: t('scenarioPage.workbuddy.applyFailed') };
    }
};

/** The dialog shows the entries Auto Config would write, fetched each time it opens. */
const WorkBuddySetupDialog: React.FC<{ slot: AgentPageSlot }> = ({ slot }) => {
    const { t } = useTranslation();
    const [previewJson, setPreviewJson] = useState('');
    const [isPreviewLoading, setIsPreviewLoading] = useState(false);
    const { dialogOpen: open, showNotification } = slot;

    useEffect(() => {
        if (!open) return;
        let cancelled = false;
        setIsPreviewLoading(true);
        api.getWorkBuddyConfigPreview().then((result) => {
            if (cancelled) return;
            setPreviewJson(result.success ? result.configJson : '// ' + (result.message || t('scenarioPage.workbuddy.previewFailed')));
            if (result.success && result.message) setPreviewJson(`// ${result.message}\n${result.configJson}`);
        }).catch(() => {
            if (!cancelled) setPreviewJson('// ' + t('scenarioPage.workbuddy.previewFailed'));
        }).finally(() => {
            if (!cancelled) setIsPreviewLoading(false);
        });
        return () => { cancelled = true; };
        // eslint-disable-next-line react-hooks/exhaustive-deps -- refetch per open, not per render
    }, [open, slot.rules]);

    return (
        <WorkBuddyConfigModal
            open={open}
            onClose={slot.closeDialog}
            baseUrl={slot.baseUrl}
            copyToClipboard={slot.copyToClipboard}
            previewJson={previewJson}
            isPreviewLoading={isPreviewLoading}
            onApply={async () => {
                const result = await slot.runApply(() => applyWorkBuddy(t, (ids) =>
                    showNotification(t('scenarioPage.workbuddy.skipped', { ids: ids.join(', ') }), 'warning')));
                if (result.success) {
                    showNotification(t('scenarioPage.workbuddy.applied', { path: result.files?.join(', ') }), 'success');
                    slot.closeDialog();
                } else {
                    showNotification(result.error || t('scenarioPage.workbuddy.applyFailed'), 'error');
                }
            }}
            isApplyLoading={slot.isApplyLoading}
        />
    );
};

// WorkBuddy is wired through its "custom model" setting (an OpenAI-compatible
// endpoint + key), not through a WorkBuddy account login — see
// .design/workbuddy-agent.md. Auto Config merges into the models.json of every
// installed edition: ~/.workbuddy (China) and/or ~/.workbuddy-ai (international).
const workbuddy: AgentPageDescriptor = {
    scenario: 'workbuddy',
    title: 'WorkBuddy',
    tooltipKey: 'scenarioPage.tooltip.workbuddy',
    connection: { compact: true, apiKeyRow: true, baseUrlRow: true },
    clientConfigTool: 'workbuddy',
    setup: {
        kind: 'auto',
        apply: (t) => applyWorkBuddy(t),
        renderDialog: (slot) => <WorkBuddySetupDialog slot={slot} />,
    },
    install: {
        installDescriptionKey: 'scenarioPage.workbuddy.installDescription',
        installActions: (t) => [
            { label: t('scenarioPage.workbuddy.viewDocs'), href: WORKBUDDY_DOCS_URL, variant: 'outlined', external: true },
        ],
    },
};

const UseWorkBuddyPage: React.FC = () => <AgentPage agent={workbuddy} />;

export default UseWorkBuddyPage;
