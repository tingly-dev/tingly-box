import { useCallback, useEffect, useState } from 'react';
import { useFunctionPanelData } from '@/hooks/useFunctionPanelData';
import { useRuleManagement } from '@/pages/scenario/hooks/useRuleManagement';
import { useScenarioPageData } from '@/pages/scenario/hooks/useScenarioPageData';

/**
 * Combined hook for scenario pages that encapsulates common data fetching and state management.
 *
 * This hook combines three hooks to reduce code duplication across scenario pages:
 * - useFunctionPanelData: Provides token, providers, notifications, etc.
 * - useRuleManagement: Handles rule CRUD operations and loading
 * - useScenarioPageData: Provides header ref, base URL, header height
 *
 * It also automatically loads rules for the specified scenario when mounted.
 *
 * @example
 * ```tsx
 * const UseOpenAIPage: React.FC = () => {
 *     const {
 *         showTokenModal, setShowTokenModal, token,
 *         isLoading, notification, copyToClipboard,
 *         baseUrl,
 *     } = useScenarioPageInternal("openai");
 *
 *     return (
 *         <PageLayout loading={isLoading} notification={notification}>
 *             <TemplatePage scenario="openai" />
 *         </PageLayout>
 *     );
 * };
 * ```
 *
 * @param scenario - The scenario identifier (e.g., "agent", "openai", "anthropic", "codex", "vscode", "xcode", "opencode")
 * @returns All the data and handlers needed by TemplatePage and scenario pages
 *
 * @returns {boolean} showTokenModal - Whether the API key modal is open
 * @returns {(show: boolean) => void} setShowTokenModal - Function to toggle API key modal
 * @returns {string} token - The authentication token
 * @returns {(message: string, severity: 'success' | 'info' | 'warning' | 'error') => void} showNotification - Function to show notifications
 * @returns {Provider[]} providers - List of configured providers
 * @returns {boolean} loading - Whether providers are loading
 * @returns {{open: boolean, message: string, severity: string}} notification - Current notification state
 * @returns {() => Promise<void>} loadProviders - Function to reload providers
 * @returns {(text: string, label: string) => Promise<void>} copyToClipboard - Function to copy text to clipboard
 *
 * @returns {any[]} rules - List of routing rules for the scenario
 * @returns {boolean} loadingRule - Whether rules are loading
 * @returns {Set<string>} newlyCreatedRuleUuids - Set of newly created rule UUIDs
 * @returns {(uuid: string) => void} handleRuleDelete - Function to delete a rule
 * @returns {(rules: any[]) => void} handleRulesChange - Function to update rules
 * @returns {(scenario: string) => Promise<void>} loadRules - Function to reload rules
 *
 * @returns {string} baseUrl - The base URL for the scenario API
 *
 * @returns {boolean} isLoading - Combined loading state (providers OR rules)
 */
export const useScenarioPageInternal = (scenario: string) => {
    // Function panel data (token, providers, notifications, etc.)
    const functionPanelData = useFunctionPanelData();

    // Rule management (rules loading, CRUD operations)
    const ruleManagement = useRuleManagement();

    // Scenario page data (header ref, base URL, header height)
    const scenarioPageData = useScenarioPageData(functionPanelData.providers);

    // Load rules for the specified scenario
    useEffect(() => {
        ruleManagement.loadRules(scenario);
    }, [scenario, ruleManagement.loadRules]);

    // Combined loading state
    const isLoading = functionPanelData.providersLoading || ruleManagement.loadingRule;

    // Return all data in a structured way
    return {
        // From useFunctionPanelData
        showTokenModal: functionPanelData.showTokenModal,
        setShowTokenModal: functionPanelData.setShowTokenModal,
        token: functionPanelData.token,
        showNotification: functionPanelData.showNotification,
        providers: functionPanelData.providers,
        loading: functionPanelData.providersLoading,
        notification: functionPanelData.notification,
        loadProviders: functionPanelData.loadProviders,
        copyToClipboard: functionPanelData.copyToClipboard,

        // From useRuleManagement
        rules: ruleManagement.rules,
        loadingRule: ruleManagement.loadingRule,
        newlyCreatedRuleUuids: ruleManagement.newlyCreatedRuleUuids,
        handleRuleDelete: ruleManagement.handleRuleDelete,
        handleRulesChange: ruleManagement.handleRulesChange,
        loadRules: ruleManagement.loadRules,

        // From useScenarioPageData
        baseUrl: scenarioPageData.baseUrl,

        // Combined
        isLoading,
    };
};
