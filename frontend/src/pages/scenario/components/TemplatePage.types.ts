import type { Rule } from '@/components/RoutingGraphTypes.ts';
import type { Provider } from '@/types/provider';

/**
 * Props for TemplatePage component.
 *
 * INTERNAL MODE (recommended):
 * Just provide `scenario` and TemplatePage will handle all data fetching internally.
 * <TemplatePage scenario="agent" />
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
    loadRules?: (scenario: string) => Promise<void>;
}
