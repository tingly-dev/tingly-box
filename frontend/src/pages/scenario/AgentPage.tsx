import { useState } from 'react';
import { Box, Button, Stack, Tooltip } from '@mui/material';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import ChoiceToggle from '@/components/ChoiceToggle';
import ConfigRow from '@/components/ConfigRow';
import PageHeader from '@/components/PageHeader';
import PageLayout from '@/components/PageLayout';
import ProviderConfigCard from '@/components/ProviderConfigCard.tsx';
import ScenarioLogDialog from '@/components/RuleLogDialog';
import type { EnvironmentMode } from '@/components/EnvironmentModeSwitcher';
import { transformUrlByMode } from '@/utils/tokenUtils';
import { type ClientConfigTool, useClientConfigStatus } from '@/hooks/useClientConfigStatus';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';
import { useContext1MToggle } from '@/pages/scenario/hooks/useContext1MToggle';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { useSlotRouting } from '@/pages/scenario/hooks/useSlotRouting';
import { useAgentObservation } from '@/pages/scenario/hooks/useAgentObservation';
import { EntryGuideDialog } from '@/components/tier/EntryGuideDialog';
import { AgentInstallRow } from './components/AgentInstallRow';
import AgentAccessSection, { useAccessExpanded } from './components/AgentAccessSection';
import AgentStatusRow, { AgentLogButton } from './components/AgentStatusRow';
import { type AgentApplyResult, type AgentInstallAction, hasModelOnAnyRule } from './components/agentSetup';
import { SCENARIO_HEADER_CONTENT_MAX_WIDTH, ScenarioCardHeader, agentPageHeaderSx } from './components/ScenarioCardHeader';
import PluginFeatures from '@/components/PluginFeatures';
import { usePluginsActive } from '@/hooks/usePluginsActive';
import ScenarioPageSkeleton from './components/ScenarioPageSkeleton';
import TemplatePage from './components/TemplatePage.tsx';

/**
 * How an agent's client gets pointed at the gateway. The header button's
 * label follows from it, not from the page (`.design/agent-page-redesign.md`
 * §3.3):
 * - `none`: nothing to configure on the client side (SDKs) — no button;
 * - `guide`: manual steps in a dialog — "Setup Guide";
 * - `auto`: the gateway writes the client's config files — "Auto Config".
 */
export type AgentSetup =
    | { kind: 'none' }
    | { kind: 'guide'; renderDialog: (slot: AgentPageSlot) => React.ReactNode }
    | {
        kind: 'auto';
        /** One-click apply with default settings (Quick Start, and dialogs that reuse it). */
        apply: (t: TFunction, ctx: AgentApplyContext) => Promise<AgentApplyResult>;
        /** Quick Start's second apply button, which also installs the status line (Claude Code). */
        applyWithStatusLine?: (t: TFunction, ctx: AgentApplyContext) => Promise<AgentApplyResult>;
        /**
         * Re-apply the client's config for a rule's 1M toggle, keeping what was
         * applied before. Present: the toggle takes effect directly with a toast.
         * Absent: the toggle opens the setup dialog.
         */
        applyContext1M?: (t: TFunction, ctx: AgentApplyContext, change: { enabled: boolean; ruleUuid?: string }) => Promise<AgentApplyResult>;
        renderDialog: (slot: AgentPageSlot) => React.ReactNode;
    };

/** What a one-click apply derives its settings from. */
export interface AgentApplyContext {
    rules: any[];
    /** Slot routing: slots with a rule of their own. */
    slots?: string[];
}

/** How the agent is installed, shown as the connection section's Install row. */
export interface AgentInstall {
    /** Shown as a copyable command; empty when installing is not a command. */
    installCommand?: string;
    installMirrorCommand?: string;
    installDescriptionKey?: string;
    installActions?: (t: TFunction) => AgentInstallAction[];
}

/** A link next to the header's config button (e.g. DSH's local Web UI). */
export interface AgentHeaderLink {
    labelKey: string;
    href: string;
}

/**
 * Everything that differs between agent pages, as data. One `AgentPage`
 * renders any of them: header (name, config status, links, config button) →
 * connection rows → routing rules → setup dialog.
 */
export interface AgentPageDescriptor {
    scenario: string;
    /** Product name; not translated. */
    title: string;
    tooltipKey?: string;
    connection?: {
        /** Title of the connection rows when it differs from `title`. */
        titleKey?: string;
        compact?: boolean;
        apiKeyRow?: boolean;
        baseUrlRow?: boolean;
    };
    /** The gateway can read this client's config back: show whether it is applied. */
    clientConfigTool?: ClientConfigTool;
    /** Rules offer the 1M-context toggle; toggling re-applies the config (`setup.applyContext1M`) or opens the setup dialog. */
    context1M?: boolean;
    setup: AgentSetup;
    install?: AgentInstall;
    headerLinks?: AgentHeaderLink[];
    /** Title of the routing rules card when it is not the default. */
    rulesTitleKey?: string;
    /**
     * Fixed model slots (Claude Code): a Unified / Separate switch in the
     * header decides which rules are shown; rules can't be added, removed or
     * switched off, since each one backs a slot.
     */
    slotRouting?: { unifiedRuleUuid: string };
}

/** What an agent's setup dialog gets from the page. */
export interface AgentPageSlot {
    scenario: string;
    baseUrl: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
    showNotification: ReturnType<typeof useScenarioPageInternal>['showNotification'];
    rules: any[];
    loadRules: (scenario: string) => Promise<void>;
    slots?: string[];
    dialogOpen: boolean;
    /** Close the setup dialog and drop a pending 1M-context change. */
    closeDialog: () => void;
    /** Set when a rule's 1M toggle opened the dialog; cleared when it closes. */
    pendingContext1MChange: boolean | null;
    /** The rule whose 1M toggle set pendingContext1MChange. */
    pendingContext1MRuleUuid?: string;
    /** `setup.apply` with its loading state tracked. */
    apply: () => Promise<AgentApplyResult>;
    /** Run another apply (e.g. a dialog's, with its own settings) under the same loading state. */
    runApply: (apply: () => Promise<AgentApplyResult>) => Promise<AgentApplyResult>;
    /** After a dialog's apply succeeds: close the dialog and toast which files were written. */
    closeApplied: (result: AgentApplyResult) => void;
    isApplyLoading: boolean;
}

/** One more row of the connection list: a ConfigRow whose content is a control, not a value. */
const ToggleRow: React.FC<{ label: string; children: React.ReactNode }> = ({ label, children }) => (
    <Box sx={{ py: 0.5 }}>
        <ConfigRow
            activeTab="row"
            onTabChange={() => undefined}
            tabs={[{ key: 'row', label, content: <Box sx={{ display: 'flex', alignItems: 'center' }}>{children}</Box> }]}
        />
    </Box>
);

const AgentPageContent: React.FC<{ agent: AgentPageDescriptor }> = ({ agent }) => {
    const { t } = useTranslation();
    const { scenario, setup, install, headerLinks, slotRouting, connection } = agent;

    // Rules: the scenario's own, or — with slot routing — the set for the current mode.
    const slots = useSlotRouting(scenario, slotRouting?.unifiedRuleUuid ?? '', !!slotRouting);
    const slotList = slotRouting ? slots.slots : undefined;
    const internal = useScenarioPageInternal(scenario, { skipRules: !!slotRouting });
    const { notification, showNotification, copyToClipboard, baseUrl, loadRules } = internal;
    const rules = slotRouting ? slots.rules : internal.rules;
    const isLoading = internal.isLoading || (!!slotRouting && slots.loading);

    const [dialogOpen, setDialogOpen] = useState(false);
    const [isApplyLoading, setIsApplyLoading] = useState(false);
    // The agent's Log (requests, today's usage, system log) is the one look-in.
    const [logOpen, setLogOpen] = useState(false);
    const observation = useAgentObservation(scenario);
    const [guideOpen, setGuideOpen] = useState(false);
    const { status: clientConfigStatus } = useClientConfigStatus(agent.clientConfigTool ?? null, [rules, dialogOpen, slotList]);
    const configApplied = clientConfigStatus?.state === 'applied';
    const context1M = useContext1MToggle(() => setDialogOpen(true));
    // Auto-config agents apply a 1M toggle straight away: no dialog, a toast says so.
    const handleContext1MToggle = async (enabled: boolean, ruleUuid?: string) => {
        if (setup.kind !== 'auto' || !setup.applyContext1M) {
            context1M.handleContext1MToggle(enabled, ruleUuid);
            return;
        }
        const result = await runApply(() => setup.applyContext1M!(t, { rules, slots: slotList }, { enabled, ruleUuid }));
        if (result.success) {
            showNotification(t(enabled ? 'context1M.appliedOn' : 'context1M.appliedOff', { client: agent.title }), 'success');
        } else if (result.error) {
            showNotification(result.error, 'error');
        }
    };
    const runApply = async (apply: () => Promise<AgentApplyResult>) => {
        setIsApplyLoading(true);
        try {
            return await apply();
        } finally {
            setIsApplyLoading(false);
        }
    };
    const applyContext: AgentApplyContext = { rules, slots: slotList };
    const closeDialog = () => {
        setDialogOpen(false);
        context1M.clearPendingContext1MChange();
    };

    const slot: AgentPageSlot = {
        scenario,
        baseUrl,
        copyToClipboard,
        showNotification,
        rules,
        loadRules,
        slots: slotList,
        dialogOpen,
        closeDialog,
        pendingContext1MChange: context1M.pendingContext1MChange,
        pendingContext1MRuleUuid: context1M.pendingContext1MRuleUuid,
        apply: () => (setup.kind === 'auto'
            ? runApply(() => setup.apply(t, applyContext))
            : Promise.resolve({ success: false })),
        runApply,
        closeApplied: (result) => {
            const files = result.files ?? [];
            let message = files.length > 0
                ? t('agentSetup.apply.written', { files: files.join(', ') })
                : t('agentSetup.apply.success');
            if ((result.backupPaths?.length ?? 0) > 0) {
                message += ` (${t('agentSetup.apply.backupSaved', { paths: result.backupPaths!.join(', ') })})`;
            }
            showNotification(message, 'success');
            closeDialog();
        },
        isApplyLoading,
    };
    const openDialog = () => setDialogOpen(true);

    // Connected once the config is applied, a request came through, or a model is routed.
    const connected = configApplied || !!observation.data?.lastRequestAt || hasModelOnAnyRule(rules);
    // The connection section: open until connected, or while the client's config is
    // out of date; the user's own open / close (kept locally) wins over that.
    const [envMode, setEnvMode] = useState<EnvironmentMode>('local');
    // Whether any plugin is on, read independently of the connection card's
    // fold state, so the folded card can surface the row only when it matters.
    const pluginsActive = usePluginsActive(scenario);
    const pluginsRow = <PluginFeatures scenario={scenario} />;
    const accessUrl = transformUrlByMode(`${baseUrl}/tingly/${scenario}`, envMode);
    const access = useAccessExpanded(scenario, !connected || clientConfigStatus?.state === 'outdated');

    // The Log look-in leads; the key actions (Quick Start,
    // header links, Auto Config / Setup Guide) follow, with the config button
    // always filled and rightmost so the next step is where the eye lands.
    const configButton = setup.kind !== 'none' && (
        <Button onClick={openDialog} variant="contained" size="small">
            {t(setup.kind === 'auto' ? 'scenarioPage.autoConfig' : 'scenarioPage.setupGuide')}
        </Button>
    );
    const headerActions = (
        <>
            <AgentLogButton onOpenLog={() => setLogOpen(true)} />
            {install && (
                <Button onClick={() => setGuideOpen(true)} variant="outlined" size="small">
                    {t('agentSetup.quickStart')}
                </Button>
            )}
            {headerLinks?.map(link => (
                <Tooltip key={link.href} title={link.href}>
                    <Button href={link.href} target="_blank" rel="noopener noreferrer" variant="contained" size="small">
                        {t(link.labelKey)}
                    </Button>
                </Tooltip>
            ))}
            {configButton}
        </>
    );

    return (
        <PageLayout loading={isLoading} loadingContent={<ScenarioPageSkeleton />} notification={notification}>
            <Stack spacing={2}>
                <PageHeader
                    title={
                        <ScenarioCardHeader
                            title={agent.title}
                            tooltipKey={agent.tooltipKey}
                        />
                    }
                    meta={(
                        <AgentStatusRow
                            observation={observation.data}
                            connected={connected}
                            configStatus={clientConfigStatus}
                        />
                    )}
                    actions={headerActions}
                    sx={agentPageHeaderSx()}
                />
                {/* Connection: always here, between the status row and the rules; open
                    until the agent is connected (or while its config is out of date),
                    then folded to just the URL. Slots and plugins are part of the
                    connection; folded, they surface only when they deviate from the
                    default (a plugin on). Slots stay in the expanded card. */}
                <AgentAccessSection
                    expanded={access.expanded}
                    onToggle={access.toggle}
                    url={accessUrl}
                    foldedExtras={(
                        <>
                            {pluginsActive && pluginsRow}
                        </>
                    )}
                >
                    <Box sx={{ maxWidth: SCENARIO_HEADER_CONTENT_MAX_WIDTH }}>
                        <ProviderConfigCard
                            title={connection?.titleKey ? t(connection.titleKey) : agent.title}
                            baseUrlPath={`/tingly/${scenario}`}
                            baseUrl={baseUrl}
                            onCopy={copyToClipboard}
                            scenario={scenario}
                            showPlugins={false}
                            envMode={envMode}
                            onEnvModeChange={setEnvMode}
                            compact={connection?.compact}
                            showApiKeyRow={connection?.apiKeyRow}
                            showBaseUrlRow={connection?.baseUrlRow}
                        />
                        {install && (
                            <AgentInstallRow
                                command={install.installCommand}
                                mirrorCommand={install.installMirrorCommand}
                                actions={install.installActions?.(t)}
                                onCopy={copyToClipboard}
                            />
                        )}
                        {/* Slots and plugins live in the connection card: they are
                            per-connection settings, set alongside the URL. Rows
                            use pt:1 so the gaps match the card's edge rhythm. */}
                        {slotRouting && (
                            <Box sx={{ px: 2, pt: 1 }}>{slots.slotsRow}</Box>
                        )}
                        <Box sx={{ px: 2, pt: 1 }}>{pluginsRow}</Box>
                    </Box>
                </AgentAccessSection>

                <TemplatePage
                    scenario={scenario}
                    // One copy of the providers too: the rule toolbar's
                    // Connect AI and Quick Start's both refresh it.
                    providers={internal.providers}
                    onProvidersLoad={internal.loadProviders}
                    // One copy of the rules for the whole page: the rule list
                    // and the setup dialog (slot.rules) read the same state.
                    rules={rules}
                    {...(slotRouting
                        // Each slot rule backs a model slot: it can't be added, removed or switched off.
                        ? { onRulesChange: slots.setRules, allowAddRule: false, allowToggleRule: false, allowDeleteRule: false }
                        : {
                            loadRules,
                            onRulesChange: internal.handleRulesChange,
                            onRuleDelete: internal.handleRuleDelete,
                            newlyCreatedRuleUuids: internal.newlyCreatedRuleUuids,
                            allowDeleteRule: true,
                        })}
                    {...(agent.rulesTitleKey ? { title: t(agent.rulesTitleKey) } : {})}
                    collapsible={true}
                    hideTroubleshoot
                    {...(agent.context1M ? { onContext1MToggle: handleContext1MToggle } : {})}
                />

                <ScenarioLogDialog open={logOpen} onClose={() => setLogOpen(false)} scenario={scenario} title={t('agentStatus.log')} />
                {setup.kind !== 'none' && setup.renderDialog(slot)}
                <EntryGuideDialog open={guideOpen} onClose={() => setGuideOpen(false)} mode="direct" />
            </Stack>
        </PageLayout>
    );
};

/** An agent page (`/agent/<scenario>`) rendered from its descriptor. */
export const AgentPage: React.FC<{ agent: AgentPageDescriptor }> = ({ agent }) => (
    <ScenarioPageModalProvider>
        <AgentPageContent agent={agent} />
    </ScenarioPageModalProvider>
);
