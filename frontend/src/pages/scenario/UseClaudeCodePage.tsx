import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import type { AgentApplyResult } from './components/AgentSetupCard';
import ClaudeCodeConfigModal from './components/ClaudeCodeConfigModal';
import { type ClaudeCodeDefaultMode, derivePrefsFromRules } from './components/ClaudeCodeQuickConfig';
import { AgentPage, type AgentApplyContext, type AgentPageDescriptor, type AgentPageSlot } from './AgentPage';

// Normalize the backend's ApplyConfigResponse into AgentApplyResult. The
// richer fields (created/updated/backup) let the dialog show a detailed
// alert while the Quick Start card keeps using the flat `files` list.
const applyClaudeConfig = async (
    t: TFunction,
    prefs: Record<string, string>,
    installStatusLine: boolean,
    defaultMode: ClaudeCodeDefaultMode = 'acceptEdits',
    showThinkingSummaries: boolean = true,
): Promise<AgentApplyResult> => {
    try {
        const result = await api.applyClaudeConfig(prefs, installStatusLine, defaultMode, showThinkingSummaries);
        if (result?.success) {
            const created = result.createdFiles || [];
            const updated = result.updatedFiles || [];
            return {
                success: true,
                files: [...created, ...updated],
                createdFiles: created,
                updatedFiles: updated,
                backupPaths: result.backupPaths || [],
            };
        }
        return { success: false, error: result?.message || t('agentSetup.apply.failed') };
    } catch (e: any) {
        return { success: false, error: e?.message || t('claudeCode.modeChange.applyFailed') };
    }
};

// One-click apply uses prefs derived from the current rules and slot mode —
// the same defaults the dialog seeds with.
const derivedPrefs = ({ rules, slotMode, slots }: AgentApplyContext) =>
    derivePrefsFromRules({ rules, mode: slotMode ?? 'unified', slots }) as Record<string, string>;

const ClaudeCodeSetupDialog: React.FC<{ slot: AgentPageSlot }> = ({ slot }) => {
    const { t } = useTranslation();
    return (
        <ClaudeCodeConfigModal
            open={slot.dialogOpen}
            onClose={slot.closeDialog}
            configMode={slot.slotMode ?? 'unified'}
            slots={slot.slots}
            baseUrl={slot.baseUrl}
            rules={slot.rules}
            copyToClipboard={slot.copyToClipboard}
            onApplyWithPrefs={(prefs, installStatusLine, defaultMode, showThinkingSummaries) =>
                slot.runApply(() => applyClaudeConfig(t, prefs as Record<string, string>, installStatusLine, defaultMode, showThinkingSummaries))
            }
            isApplyLoading={slot.isApplyLoading}
            pendingContext1MChange={slot.pendingContext1MChange === null
                ? null
                : { enabled: slot.pendingContext1MChange, ruleUuid: slot.pendingContext1MRuleUuid }}
        />
    );
};

const claudeCode: AgentPageDescriptor = {
    scenario: 'claude_code',
    title: 'Claude Code',
    tooltipKey: 'scenarioPage.tooltip.claude_code',
    connection: { compact: true, apiKeyRow: true, baseUrlRow: true },
    clientConfigTool: 'claude',
    context1M: true,
    slotRouting: { unifiedRuleUuid: 'builtin:claude_code:cc' },
    setup: {
        kind: 'auto',
        apply: (t, ctx) => applyClaudeConfig(t, derivedPrefs(ctx), false),
        applyWithStatusLine: (t, ctx) => applyClaudeConfig(t, derivedPrefs(ctx), true),
        renderDialog: (slot) => <ClaudeCodeSetupDialog slot={slot} />,
    },
    quickStart: {
        installCommand: 'npm install -g @anthropic-ai/claude-code',
        installMirrorCommand: 'npm install -g @anthropic-ai/claude-code --registry=https://registry.npmmirror.com',
    },
};

const UseClaudeCodePage: React.FC = () => <AgentPage agent={claudeCode} />;

export default UseClaudeCodePage;
