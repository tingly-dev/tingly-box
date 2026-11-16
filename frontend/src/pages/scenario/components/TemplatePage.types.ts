import type { Rule } from '@/components/RoutingGraphTypes.ts';
import type { Provider } from '@/types/provider';

/**
 * Props for TemplatePage component.
 *
 * INTERNAL MODE (recommended):
 * Just provide `scenario` and TemplatePage will handle all data fetching internally.
 * <TemplatePage scenario="custom" />
 *
 * PROFILE MODE:
 * For profile-specific rules, provide the suffixed scenario.
 * <TemplatePage scenario="claude_code:p1" />
 *
 * HYBRID MODE (for custom logic):
 * Provide `scenario` plus override specific data props for custom behavior.
 * <TemplatePage scenario="custom" rules={customRules} onRulesChange={customHandler} />
 */
export interface TemplatePageProps {
    scenario: string;
    title?: string | React.ReactNode;
    collapsible?: boolean;
    allowDeleteRule?: boolean;
    allowToggleRule?: boolean;
    allowAddRule?: boolean;
    /**
     * Hard ordering: show pin/move controls on each rule card. The order is
     * persisted via POST /rules/reorder and drives /v1/models listing, match
     * priority and client config generation. Slot pages (claude_code) keep it
     * off — their order is the product-defined slot precedence.
     */
    allowReorder?: boolean;
    /** The page offers its own way into the scenario's logs (the Agent page's status row): no Troubleshoot in the "..." menu. */
    hideTroubleshoot?: boolean;
    showAddApiKeyButton?: boolean;
    showCreateRuleButton?: boolean;
    showExpandCollapseButton?: boolean;
    showImportButton?: boolean;
    rightAction?: React.ReactNode;
    showEmptyState?: boolean;
    emptyStateTitle?: string;
    emptyStateDescription?: string;
    onAddApiKeyClick?: () => void;
    onContext1MToggle?: (newState: boolean, ruleUuid?: string) => void;

    // Optional overrides for custom logic (hybrid mode)
    rules?: Rule[];
    showNotification?: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    providers?: Provider[];
    onRulesChange?: (updatedRules: Rule[]) => void;
    onProvidersLoad?: () => Promise<void>;
    onRuleDelete?: (ruleUuid: string) => void;
    /** Rules created in this session (highlighted); pass with `rules` when the parent owns them. */
    newlyCreatedRuleUuids?: Set<string>;
    loadRules?: (scenario: string) => Promise<void>;
}
