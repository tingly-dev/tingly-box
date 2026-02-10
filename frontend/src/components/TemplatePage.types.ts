import type { Rule } from './RoutingGraphTypes.ts';
import type { Provider } from '../types/provider';

export interface TabTemplatePageProps {
    title?: string | React.ReactNode;
    rules: Rule[];
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    providers: Provider[];
    onRulesChange?: (updatedRules: Rule[]) => void;
    collapsible?: boolean;
    allowDeleteRule?: boolean;
    onRuleDelete?: (ruleUuid: string) => void;
    allowToggleRule?: boolean;
    newlyCreatedRuleUuids?: Set<string>; // @deprecated - not used, kept for API compatibility
    // Unified action buttons props
    scenario?: string;
    showAddApiKeyButton?: boolean;
    showCreateRuleButton?: boolean;
    showExpandCollapseButton?: boolean;
    showImportButton?: boolean;
    // Allow custom rightAction for backward compatibility
    rightAction?: React.ReactNode;
    // Header height from parent component for calculating available space
    headerHeight?: number;
}
