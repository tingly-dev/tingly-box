import { useState } from 'react';
import { Box, Button, IconButton, Stack, Tab, Tabs, Tooltip } from '@mui/material';
import { Rule as SetupStepsIcon } from '@/components/icons';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import PageHeader from '@/components/PageHeader';
import { ClientConfigStatusChip } from '@/components/ClientConfigStatusChip';
import ConnectAIDialogs from '@/components/ConnectAIDialogs';
import PageLayout from '@/components/PageLayout';
import ProviderConfigCard from '@/components/ProviderConfigCard.tsx';
import UnifiedCard from '@/components/UnifiedCard.tsx';
import { type ClientConfigTool, useClientConfigStatus } from '@/hooks/useClientConfigStatus';
import { useProviderDialog } from '@/hooks/useProviderDialog';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';
import { useContext1MToggle } from '@/pages/scenario/hooks/useContext1MToggle';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { type SlotMode, useSlotRouting } from '@/pages/scenario/hooks/useSlotRouting';
import AgentActivityPanel from './components/AgentActivityPanel';
import AgentSetupCard, {
    type AgentApplyResult,
    type AgentInstallAction,
    hasModelOnAnyRule,
    scrollToModelsCard,
} from './components/AgentSetupCard';
import { SCENARIO_HEADER_CONTENT_MAX_WIDTH, ScenarioCardHeader } from './components/ScenarioCardHeader';
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
        renderDialog: (slot: AgentPageSlot) => React.ReactNode;
    };

/** What a one-click apply derives its settings from. */
export interface AgentApplyContext {
    rules: any[];
    /** Set for agents with slot routing. */
    slotMode?: SlotMode;
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
    /** Rules offer the 1M-context toggle; toggling opens the setup dialog. */
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

export type AgentTab = 'connection' | 'rules' | 'activity';
const tabPrefKey = (scenario: string) => `agent-tab-${scenario}`;
const readTabPref = (scenario: string): AgentTab | null => {
    try {
        const v = localStorage.getItem(tabPrefKey(scenario));
        return v === 'connection' || v === 'rules' || v === 'activity' ? v : null;
    } catch {
        return null;
    }
};

/** What an agent's setup dialog gets from the page. */
export interface AgentPageSlot {
    scenario: string;
    baseUrl: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
    showNotification: ReturnType<typeof useScenarioPageInternal>['showNotification'];
    rules: any[];
    loadRules: (scenario: string) => Promise<void>;
    slotMode?: SlotMode;
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
    isApplyLoading: boolean;
}

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
    const [setupDone, setSetupDone] = useState(false);
    // Which tab is on show. Until the user picks one it follows the agent: one
    // that already routes a model opens on its rules, a new one on its
    // connection (where the setup steps are). Their pick is remembered.
    const [tabPref, setTabPref] = useState<AgentTab | null>(() => readTabPref(scenario));
    const [setupReopenKey, setSetupReopenKey] = useState(0);
    const { status: clientConfigStatus } = useClientConfigStatus(agent.clientConfigTool ?? null, [rules, dialogOpen, slotMode]);
    const context1M = useContext1MToggle(() => setDialogOpen(true));
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
    const applyContext: AgentApplyContext = { rules, slotMode };

    const slot: AgentPageSlot = {
        scenario,
        baseUrl,
        copyToClipboard,
        showNotification,
        rules,
        loadRules,
        slotMode,
        dialogOpen,
        closeDialog: () => {
            setDialogOpen(false);
            context1M.clearPendingContext1MChange();
        },
        pendingContext1MChange: context1M.pendingContext1MChange,
        pendingContext1MRuleUuid: context1M.pendingContext1MRuleUuid,
        apply: () => (setup.kind === 'auto'
            ? runApply(() => setup.apply(t, applyContext))
            : Promise.resolve({ success: false })),
        runApply,
        isApplyLoading,
    };
    const openDialog = () => setDialogOpen(true);

    // Only the action that matters now is filled: a header link (DSH's Web UI),
    // else Auto Config until the client's config reads back as applied.
    const configApplied = clientConfigStatus?.state === 'applied';
    const configButton = setup.kind !== 'none' && (
        <Button
            onClick={openDialog}
            // One filled button at a time: while the setup row is on the page it carries
            // the step to do (Auto Config at the apply step), so this one steps back.
            variant={headerLinks?.length || configApplied || (quickStart && !setupDone) ? 'outlined' : 'contained'}
            size="small"
        >
            {t(setup.kind === 'auto' ? 'scenarioPage.autoConfig' : 'scenarioPage.setupGuide')}
        </Button>
    );
    const headerActions = (
        <>
            {slotRouting && slots.modeSwitch}
            {headerLinks?.map(link => (
                <Tooltip key={link.href} title={link.href}>
                    <Button href={link.href} target="_blank" rel="noopener noreferrer" variant="contained" size="small">
                        {t(link.labelKey)}
                    </Button>
                </Tooltip>
            ))}
            {configButton}
            {/* The setup row leaves the page once its steps are done; this brings it back. */}
            {quickStart && setupDone && (
                <Tooltip title={t('agentSetup.showSteps', { defaultValue: 'Setup steps' })}>
                    <IconButton onClick={() => { setSetupReopenKey((k) => k + 1); selectTab('connection'); }} size="small" aria-label={t('agentSetup.showSteps', { defaultValue: 'Setup steps' })} sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
                        <SetupStepsIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
            )}
        </>
    );

    const tab: AgentTab = tabPref ?? (hasModelOnAnyRule(rules) ? 'rules' : 'connection');
    const selectTab = (next: AgentTab) => {
        setTabPref(next);
        try { localStorage.setItem(tabPrefKey(scenario), next); } catch { /* per-session only */ }
    };
    // "Choose a model" lives on the rules tab: show it first, then point at it.
    const chooseModel = () => {
        selectTab('rules');
        window.setTimeout(scrollToModelsCard, 150);
    };
    const panelSx = (name: AgentTab) => (tab === name ? undefined : { display: 'none' });

    return (
        <PageLayout loading={isLoading} loadingContent={<ScenarioPageSkeleton />} notification={notification}>
            <Stack spacing={2}>
                <PageHeader
                    title={
                        <ScenarioCardHeader
                            title={agent.title}
                            tooltipKey={agent.tooltipKey}
                            addon={agent.clientConfigTool && <ClientConfigStatusChip status={clientConfigStatus} onApply={openDialog} />}
                        />
                    }
                    actions={headerActions}
                    // Title, status and actions share a row only where they fit; below that
                    // the actions drop under the title instead of wrapping it.
                    sx={{
                        pb: 1.5,
                        borderBottom: 0,
                        flexDirection: { xs: 'column', lg: 'row' },
                        alignItems: { xs: 'flex-start', lg: 'center' },
                        '& h1': { whiteSpace: 'nowrap' },
                    }}
                />
                {/* One section at a time: how the tool connects, how requests are
                    routed, how it is doing. Panels stay mounted (only hidden) so
                    switching keeps their state, except the activity one, which
                    loads only while it is on show. */}
                <Tabs
                    value={tab}
                    onChange={(_, next: AgentTab) => selectTab(next)}
                    sx={{ borderBottom: '1px solid', borderColor: 'divider', minHeight: 40, '& .MuiTab-root': { textTransform: 'none', minHeight: 40 } }}
                >
                    <Tab value="connection" label={t('scenarioPage.tabs.connection', { defaultValue: 'Connection' })} />
                    <Tab value="rules" label={agent.rulesTitleKey ? t(agent.rulesTitleKey) : t('scenarioPage.modelRules')} />
                    <Tab value="activity" label={t('agentActivity.title')} />
                </Tabs>

                <Box role="tabpanel" sx={panelSx('connection')}>
                    <UnifiedCard size="full" contentMaxWidth={SCENARIO_HEADER_CONTENT_MAX_WIDTH}>
                        {quickStart && (
                            <AgentSetupCard
                                inline
                                reopenKey={setupReopenKey}
                                onAllDoneChange={setSetupDone}
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
                                onSelectModel={chooseModel}
                                onConnectProvider={connectAI.handleConnectAIClick}
                                providers={internal.providers}
                                providersLoading={internal.loading}
                                configApplied={configApplied}
                            />
                        )}
                        <ProviderConfigCard
                            title={connection?.titleKey ? t(connection.titleKey) : agent.title}
                            baseUrlPath={`/tingly/${scenario}`}
                            baseUrl={baseUrl}
                            onCopy={copyToClipboard}
                            scenario={scenario}
                            compact={connection?.compact}
                            showApiKeyRow={connection?.apiKeyRow}
                            showBaseUrlRow={connection?.baseUrlRow}
                        />
                    </UnifiedCard>
                </Box>

                <Box role="tabpanel" sx={panelSx('rules')}>
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
                        {...(agent.context1M ? { onContext1MToggle: context1M.handleContext1MToggle } : {})}
                    />
                </Box>

                <Box role="tabpanel" sx={panelSx('activity')}>
                    <AgentActivityPanel scenario={scenario} active={tab === 'activity'} />
                </Box>

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
