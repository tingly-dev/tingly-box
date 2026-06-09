import type { Rule } from '@/components/RoutingGraphTypes.ts';
import type { Provider } from '@/types/provider';

/**
 * Props for using TemplatePage with internally-managed state (recommended).
 * Only provide `scenario` and TemplatePage will handle all data fetching internally.
 */
export interface TemplatePageInternalProps {
    scenario: string;
    title?: string | React.ReactNode;
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
