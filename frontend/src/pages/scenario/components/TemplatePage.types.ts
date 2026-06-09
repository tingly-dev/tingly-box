import type { Rule } from '@/components/RoutingGraphTypes.ts';
import type { Provider } from '@/types/provider';

/**
 * Props for using TemplatePage with internally-managed state (recommended).
 * Only provide `scenario` and TemplatePage will handle all data fetching internally.
 */
// How the per-rule "1M context window" switch behaves for this page. Only the
// three scenarios that actually use 1M pass it; everything else omits it and
// the switch stays hidden. 'rename' carries [1m] in the model name (Claude
// Code / Claude Desktop client convention); 'flag' sets the context_1m flag
// (Codex catalog context window). See .design/one-m-context.md.
export type OneMMode = 'rename' | 'flag';

export interface TemplatePageInternalProps {
    scenario: string;
    title?: string | React.ReactNode;
    oneMMode?: OneMMode;
    collapsible?: boolean;
    allowDeleteRule?: boolean;
    allowToggleRule?: boolean;
    allowAddRule?: boolean;
    showAddApiKeyButton?: boolean;
    showCreateRuleButton?: boolean;
    showExpandCollapseButton?: boolean;
    showImportButton?: boolean;
    rightAction?: React.ReactNode;
    showEmptyState?: boolean;
    emptyStateTitle?: string;
    emptyStateDescription?: string;
    onAddApiKeyClick?: () => void;
}

/**
 * Props for using TemplatePage with externally-managed state (legacy pattern).
 * All state must be provided by the parent component.
 *
 * @deprecated Use TemplatePageInternalProps instead (just pass `scenario`)
 */
export interface TemplatePageExternalProps {
    title?: string | React.ReactNode;
    rules: Rule[];
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    providers: Provider[];
    onRulesChange?: (updatedRules: Rule[]) => void;
    onProvidersLoad?: () => Promise<void>;
    collapsible?: boolean;
    allowDeleteRule?: boolean;
    onRuleDelete?: (ruleUuid: string) => void;
    allowToggleRule?: boolean;
    allowAddRule?: boolean;
    newlyCreatedRuleUuids?: Set<string>; // @deprecated - not used, kept for API compatibility
    scenario?: string;
    oneMMode?: OneMMode;
    showAddApiKeyButton?: boolean;
    showCreateRuleButton?: boolean;
    showExpandCollapseButton?: boolean;
    showImportButton?: boolean;
    rightAction?: React.ReactNode;
    headerHeight?: number;
    loadRules?: (scenario: string) => Promise<void>;
    showEmptyState?: boolean;
    emptyStateTitle?: string;
    emptyStateDescription?: string;
    onAddApiKeyClick?: () => void;
}

/**
 * Union type discriminated by whether `rules` is provided.
 * - If `rules` is provided → external mode (legacy)
 * - If `scenario` is provided (and `rules` is not) → internal mode (recommended)
 */
export type TabTemplatePageProps = TemplatePageInternalProps & Partial<TemplatePageExternalProps>;
