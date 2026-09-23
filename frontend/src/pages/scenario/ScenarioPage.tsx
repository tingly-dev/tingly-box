import { useState } from 'react';
import { Box, Button, IconButton, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Info as InfoIcon } from '@/components/icons';
import CardGrid from '@/components/CardGrid.tsx';
import ConnectAIDialogs from '@/components/ConnectAIDialogs';
import PageLayout from '@/components/PageLayout';
import ProviderConfigCard from '@/components/ProviderConfigCard.tsx';
import UnifiedCard from '@/components/UnifiedCard.tsx';
import { useProviderDialog } from '@/hooks/useProviderDialog';
import { useContext1MToggle } from '@/pages/scenario/hooks/useContext1MToggle';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import ScenarioPageSkeleton from './components/ScenarioPageSkeleton';
import TemplatePage from './components/TemplatePage.tsx';

/**
 * UnifiedCard header title block shared by every scenario page: the card
 * title plus an optional i18n-keyed info tooltip. Pages that keep their own
 * structure (Codex, ImageGen) reuse this instead of re-rolling the Box.
 */
export const ScenarioCardHeader: React.FC<{ title: string; tooltipKey?: string }> = ({ title, tooltipKey }) => {
    const { t } = useTranslation();
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <span>{title}</span>
            {tooltipKey && (
                <Tooltip title={t(tooltipKey)}>
                    <IconButton size="small" sx={{ ml: 0.5 }}>
                        <InfoIcon fontSize="small" sx={{ color: 'text.secondary' }} />
                    </IconButton>
                </Tooltip>
            )}
        </Box>
    );
};

/** Standard "Config" / "Auto Config" button in the UnifiedCard rightAction slot. */
export const ScenarioConfigButton: React.FC<{
    onClick: () => void;
    label: React.ReactNode;
    variant?: 'contained' | 'outlined';
}> = ({ onClick, label, variant = 'contained' }) => (
    <Button onClick={onClick} variant={variant} size="small">
        {label}
    </Button>
);

/**
 * Everything a scenario page's bespoke pieces (rightAction, extra cards,
 * config modal) may need from the shared skeleton.
 */
export interface ScenarioPageSlot {
    scenario: string;
    isLoading: boolean;
    baseUrl: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
    showNotification: ReturnType<typeof useScenarioPageInternal>['showNotification'];
    rules: any[];
    loadRules: (scenario: string) => Promise<void>;
    configModalOpen: boolean;
    openConfigModal: () => void;
    closeConfigModal: () => void;
    /** Set by the context-1M toggle; cleared via clearPendingContext1MChange when the modal closes. */
    pendingContext1MChange: boolean | null;
    clearPendingContext1MChange: () => void;
    connectAI: ReturnType<typeof useProviderDialog>;
}

export interface ScenarioPageProps {
    scenario: string;
    /** UnifiedCard title text (also the default ProviderConfigCard title). */
    title: string;
    /** i18n key for the info tooltip next to the title. */
    tooltipKey?: string;
    /** UnifiedCard rightAction slot (static node). */
    rightAction?: React.ReactNode;
    /**
     * UnifiedCard rightAction slot when it needs page internals (config modal
     * open state, Connect AI flow, ...). Named `render*` — a function prop
     * returning JSX — so it reads as a render prop.
     */
    renderRightAction?: (slot: ScenarioPageSlot) => React.ReactNode;
    /** ProviderConfigCard overrides (title defaults to `title`). */
    providerCard?: {
        title?: string;
        compact?: boolean;
        showApiKeyRow?: boolean;
        showBaseUrlRow?: boolean;
    };
    /** Optional title override for the TemplatePage rules card. */
    templateTitle?: React.ReactNode;
    /** Wire TemplatePage's onContext1MToggle to the shared pending-change plumbing. */
    context1M?: boolean;
    /** Render <ConnectAIDialogs> for the shared Connect AI add flow. */
    withConnectAI?: boolean;
    /** Extra cards between the UnifiedCard and TemplatePage (e.g. AgentSetupCard). */
    children?: React.ReactNode | ((slot: ScenarioPageSlot) => React.ReactNode);
    /** Config modal rendered after TemplatePage; owns nothing — read open state from the slot. */
    renderConfigModal?: (slot: ScenarioPageSlot) => React.ReactNode;
}

/**
 * Shared scenario page skeleton: ScenarioPageModalProvider + useScenarioPageInternal
 * + PageLayout(ScenarioPageSkeleton) + CardGrid[UnifiedCard → extra cards →
 * TemplatePage → config modal → ConnectAIDialogs].
 *
 * Trivial pages reduce to pure props; pages with bespoke pieces (custom apply
 * flows, extra cards) pass them via the render-prop slots. Genuinely bespoke
 * pages (Claude Code, Team) keep their own structure.
 */
export const ScenarioPage: React.FC<ScenarioPageProps> = ({
    scenario,
    title,
    tooltipKey,
    rightAction,
    renderRightAction,
    providerCard,
    templateTitle,
    context1M = false,
    withConnectAI = false,
    children,
    renderConfigModal,
}) => {
    const {
        isLoading,
        notification,
        showNotification,
        copyToClipboard,
        baseUrl,
        rules,
        loadRules,
    } = useScenarioPageInternal(scenario);

    const [configModalOpen, setConfigModalOpen] = useState(false);
    // Context-1M toggle plumbing for pages whose TemplatePage wires it up.
    const context1MState = useContext1MToggle(() => setConfigModalOpen(true));
    // Unified Connect AI add flow (picker + form/OAuth/paste/import dialogs).
    const connectAI = useProviderDialog(showNotification, {
        onProviderAdded: () => window.location.reload(),
    });

    const slot: ScenarioPageSlot = {
        scenario,
        isLoading,
        baseUrl,
        copyToClipboard,
        showNotification,
        rules,
        loadRules,
        configModalOpen,
        openConfigModal: () => setConfigModalOpen(true),
        closeConfigModal: () => setConfigModalOpen(false),
        pendingContext1MChange: context1MState.pendingContext1MChange,
        clearPendingContext1MChange: context1MState.clearPendingContext1MChange,
        connectAI,
    };

    return (
        <PageLayout loading={isLoading} loadingContent={<ScenarioPageSkeleton />} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    titleHeadingLevel={1}
                    title={<ScenarioCardHeader title={title} tooltipKey={tooltipKey} />}
                    size="full"
                    rightAction={renderRightAction ? renderRightAction(slot) : rightAction}
                >
                    <ProviderConfigCard
                        title={providerCard?.title ?? title}
                        baseUrlPath={`/tingly/${scenario}`}
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        scenario={scenario}
                        compact={providerCard?.compact}
                        showApiKeyRow={providerCard?.showApiKeyRow}
                        showBaseUrlRow={providerCard?.showBaseUrlRow}
                    />
                </UnifiedCard>
                {typeof children === 'function' ? children(slot) : children}
                <TemplatePage
                    scenario={scenario}
                    {...(templateTitle !== undefined ? { title: templateTitle } : {})}
                    collapsible={true}
                    allowDeleteRule={true}
                    {...(context1M ? { onContext1MToggle: context1MState.handleContext1MToggle } : {})}
                />
                {renderConfigModal?.(slot)}
                {withConnectAI && <ConnectAIDialogs flow={connectAI} />}
            </CardGrid>
        </PageLayout>
    );
};
