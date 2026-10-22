import { useCallback, useRef, useState } from 'react';
import { Box, Button, Stack, Tooltip } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import ChoiceToggle from '@/components/ChoiceToggle';
import ConfigRow from '@/components/ConfigRow';
import PageHeader from '@/components/PageHeader';
import ConnectAIDialogs from '@/components/ConnectAIDialogs';
import PageLayout from '@/components/PageLayout';
import ProviderConfigCard from '@/components/ProviderConfigCard.tsx';
import ScenarioLogDialog from '@/components/RuleLogDialog';
import type { EnvironmentMode } from '@/components/EnvironmentModeSwitcher';
import { transformUrlByMode } from '@/utils/tokenUtils';
import { type ClientConfigTool, useClientConfigStatus } from '@/hooks/useClientConfigStatus';
import { useProviderDialog } from '@/hooks/useProviderDialog';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';
import { useContext1MToggle } from '@/pages/scenario/hooks/useContext1MToggle';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { type SlotMode, useSlotRouting } from '@/pages/scenario/hooks/useSlotRouting';
import { useAgentObservation } from '@/pages/scenario/hooks/useAgentObservation';
import AgentAccessSection, { useAccessExpanded } from './components/AgentAccessSection';
import AgentActivityDialog from './components/AgentActivityDialog';
import AgentStatusRow, { AgentLooksIn } from './components/AgentStatusRow';
import AgentSetupCard, {
    type AgentApplyResult,
    type AgentInstallAction,
    hasModelOnAnyRule,
    resetSetupProgress,
    scrollToModelsCard,
} from './components/AgentSetupCard';
import { SCENARIO_HEADER_CONTENT_MAX_WIDTH, ScenarioCardHeader, agentPageHeaderSx } from './components/ScenarioCardHeader';
import AgentPluginsCard from './components/AgentPluginsCard';
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
    /** Set for agents with slot routing. */
    slotMode?: SlotMode;
    /** Slot routing, unified mode: slots with a rule of their own. */
    slots?: string[];
}

/** The Quick Start card: install → configure → pick a model. */
export interface AgentQuickStart {
    /** Shown as a copyable command; empty when installing is not a command. */
    installCommand?: string;
    installMirrorCommand?: string;
    installDescriptionKey?: string;
    installActions?: (t: TFunction) => AgentInstallAction[];
    applyStepLabelKey?: string;
    applyStepDescriptionKey?: string;
    /** Label of the step's button that opens the setup dialog. */
    openDialogLabelKey?: string;
}

/** A link next to the header's config button (e.g. DSH's local Web UI). */
export interface AgentHeaderLink {
    labelKey: string;
    href: string;
}

/**
 * Everything that differs between agent pages, as data. One `AgentPage`
 * renders any of them: header (name, config status, links, config button) →
 * connection rows → Quick Start → routing rules → setup dialog.
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
    quickStart?: AgentQuickStart;
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
    slotMode?: SlotMode;
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
    const { scenario, setup, quickStart, headerLinks, slotRouting, connection } = agent;

    // Rules: the scenario's own, or — with slot routing — the set for the current mode.
    const slots = useSlotRouting(scenario, slotRouting?.unifiedRuleUuid ?? '', !!slotRouting);
    const slotMode = slotRouting ? slots.mode : undefined;
    const internal = useScenarioPageInternal(scenario, { skipRules: !!slotRouting });
    const { notification, showNotification, copyToClipboard, baseUrl, loadRules } = internal;
    const rules = slotRouting ? slots.rules : internal.rules;
    const isLoading = internal.isLoading || (!!slotRouting && slots.loading);

    const [dialogOpen, setDialogOpen] = useState(false);
    const [isApplyLoading, setIsApplyLoading] = useState(false);
    // Whether every Quick Start step is done; null until the card has worked it out.
    const [setupDone, setSetupDone] = useState<boolean | null>(null);
    // Requests (a dialog) and Usage (the Dashboard, filtered to this agent) are looks-in
    // from the status row.
    const [requestsOpen, setRequestsOpen] = useState(false);
    const [logOpen, setLogOpen] = useState(false);
    const navigate = useNavigate();
    const observation = useAgentObservation(scenario);
    // A finished Quick Start leaves the page; its button in the status row brings it back
    // (and resets it). `reopened` keeps it on show until it is finished again.
    const [quickStartReopened, setQuickStartReopened] = useState(false);
    const [quickStartResetKey, setQuickStartResetKey] = useState(0);
    const wasAllDone = useRef<boolean | null>(null);
    const onSetupDoneChange = useCallback((done: boolean) => {
        // Finished again after a reopen: it hides itself once more.
        if (wasAllDone.current === false && done) setQuickStartReopened(false);
        wasAllDone.current = done;
        setSetupDone(done);
    }, []);
    const { status: clientConfigStatus } = useClientConfigStatus(agent.clientConfigTool ?? null, [rules, dialogOpen, slotMode]);
    const context1M = useContext1MToggle(() => setDialogOpen(true));
    // Auto-config agents apply a 1M toggle straight away: no dialog, a toast says so.
    const handleContext1MToggle = async (enabled: boolean, ruleUuid?: string) => {
        if (setup.kind !== 'auto' || !setup.applyContext1M) {
            context1M.handleContext1MToggle(enabled, ruleUuid);
            return;
        }
        const result = await runApply(() => setup.applyContext1M!(t, { rules, slotMode }, { enabled, ruleUuid }));
        if (result.success) {
            showNotification(t(enabled ? 'context1M.appliedOn' : 'context1M.appliedOff', { client: agent.title }), 'success');
        } else if (result.error) {
            showNotification(result.error, 'error');
        }
    };
    // Unified Connect AI add flow (picker + form/OAuth/paste/import dialogs), offered by Quick Start.
    // A new provider refreshes the page's list in place: Quick Start's first
    // step ticks over and the flow carries on to "pick a model" instead of
    // the page reloading under the user.
    const connectAI = useProviderDialog(showNotification, {
        onProviderAdded: () => { void internal.loadProviders(); },
    });

    const runApply = async (apply: () => Promise<AgentApplyResult>) => {
        setIsApplyLoading(true);
        try {
            return await apply();
        } finally {
            setIsApplyLoading(false);
        }
    };
    const slotList = slotRouting ? slots.slots : undefined;
    const applyContext: AgentApplyContext = { rules, slotMode, slots: slotList };
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
        slotMode,
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

    // Until progress is known, an agent that already routes a model counts as done.
    const setupFinished = setupDone ?? hasModelOnAnyRule(rules);
    const quickStartHidden = !quickStart || (setupFinished && !quickStartReopened);
    const slotModeLabel = t('scenarioPage.slotMode', { defaultValue: 'Model mode' });
    const restartQuickStart = () => {
        resetSetupProgress(scenario);
        wasAllDone.current = null;
        setQuickStartReopened(true);
        setQuickStartResetKey((k) => k + 1); // remount: the card re-reads its (now empty) progress
    };
    // "Connected" is the signal Quick Start already uses (every step done); pages
    // without a Quick Start have no steps, so for them it is "has sent a request".
    const connected = quickStart ? setupFinished : !!observation.data?.lastRequestAt;
    // The connection section: open until connected, or while the client's config is
    // out of date; the user's own open / close (kept locally) wins over that.
    const [envMode, setEnvMode] = useState<EnvironmentMode>('local');
    const accessUrl = transformUrlByMode(`${baseUrl}/tingly/${scenario}`, envMode);
    const access = useAccessExpanded(scenario, !connected || clientConfigStatus?.state === 'outdated');

    const configApplied = clientConfigStatus?.state === 'applied';
    // Looks-in (Requests / Usage / Log) lead; the key actions (Quick Start,
    // header links, Auto Config / Setup Guide) follow, with the config button
    // always filled and rightmost so the next step is where the eye lands.
    const configButton = setup.kind !== 'none' && (
        <Button onClick={openDialog} variant="contained" size="small">
            {t(setup.kind === 'auto' ? 'scenarioPage.autoConfig' : 'scenarioPage.setupGuide')}
        </Button>
    );
    const headerActions = (
        <>
            <AgentLooksIn
                onOpenRequests={() => setRequestsOpen(true)}
                onOpenLog={() => setLogOpen(true)}
                // The Dashboard already opens filtered to one agent (?scenario=).
                onOpenUsage={() => navigate(`/dashboard/today?scenario=${encodeURIComponent(scenario)}`)}
            />
            {quickStart && quickStartHidden && (
                // A finished Quick Start leaves the page; this brings it back (reset).
                <Tooltip title={t('agentSetup.restartTooltip', { defaultValue: 'Run Quick Start again' })}>
                    <Button onClick={restartQuickStart} variant="outlined" size="small">
                        {t('agentSetup.quickStart')}
                    </Button>
                </Tooltip>
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
                    then folded to just the URL. Page-level Plugins are their own card. */}
                <AgentAccessSection expanded={access.expanded} onToggle={access.toggle} url={accessUrl}>
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
                        {slotRouting && (
                            <Box sx={{ px: 2, py: 0.5 }}>
                                <ToggleRow label={slotModeLabel}>
                                    <ChoiceToggle<SlotMode>
                                        value={slots.mode}
                                        options={slots.modeOptions}
                                        onChange={slots.requestMode}
                                        optionWidth={112}
                                        flush
                                        ariaLabel={slotModeLabel}
                                    />
                                </ToggleRow>
                                {slots.slotsRow && <Box sx={{ py: 0.5 }}>{slots.slotsRow}</Box>}
                            </Box>
                        )}
                    </Box>
                </AgentAccessSection>

                <AgentPluginsCard scenario={scenario} />

                {quickStart && (
                    // Embedded where it always was, above the rules; gone once done.
                    <AgentSetupCard
                        key={quickStartResetKey}
                        hidden={quickStartHidden}
                        onDismiss={() => setQuickStartReopened(false)}
                        onAllDoneChange={onSetupDoneChange}
                        agentKey={scenario}
                        agentName={agent.title}
                        installCommand={quickStart.installCommand ?? ''}
                        installMirrorCommand={quickStart.installMirrorCommand}
                        installStepDescription={quickStart.installDescriptionKey && t(quickStart.installDescriptionKey)}
                        installActions={quickStart.installActions?.(t)}
                        onApply={setup.kind === 'auto' ? slot.apply : undefined}
                        onApplyWithStatusLine={setup.kind === 'auto' && setup.applyWithStatusLine
                            ? () => runApply(() => setup.applyWithStatusLine!(t, applyContext))
                            : undefined}
                        isApplyLoading={isApplyLoading}
                        onViewConfig={openDialog}
                        applyStepLabel={quickStart.applyStepLabelKey && t(quickStart.applyStepLabelKey)}
                        applyStepDescription={quickStart.applyStepDescriptionKey && t(quickStart.applyStepDescriptionKey)}
                        viewConfigButtonLabel={quickStart.openDialogLabelKey && t(quickStart.openDialogLabelKey)}
                        hasModelSelected={hasModelOnAnyRule(rules)}
                        onSelectModel={scrollToModelsCard}
                        onConnectProvider={connectAI.handleConnectAIClick}
                        providers={internal.providers}
                        providersLoading={internal.loading}
                        configApplied={configApplied}
                    />
                )}
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
                <AgentActivityDialog scenario={scenario} open={requestsOpen} onClose={() => setRequestsOpen(false)} />
                {slotRouting && slots.modeDialog}
                {setup.kind !== 'none' && setup.renderDialog(slot)}
                {quickStart && <ConnectAIDialogs flow={connectAI} />}
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
