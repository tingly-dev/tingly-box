import { useCallback, useState } from 'react';
import { api } from '@/services/api';
import type { Provider, ProviderModelsDataByUuid, ProviderModelData } from '@/types/provider';
import type { ConfigRecord, Rule } from '@/components/RoutingGraphTypes';
import {
    useRuleCardExpanded,
    useRuleCardData,
    useRuleAutoSave,
    useRuleExport,
    useSmartRoutingHandlers,
} from '@/components/rule-card/useRuleCardHooks';
import { RuleCardDeleteDialog } from '@/components/rule-card/dialogs';
import RoutingGraph from '@/components/RoutingGraph';
import SmartRoutingGraph from '@/components/SmartRoutingGraph';
import SmartRuleEditDialog from '@/components/SmartRuleEditDialog';
import GraphSettingsMenu from '@/components/GraphSettingsMenu';

export interface RuleCardProps {
    rule: Rule;
    providers: Provider[];
    providerModelsByUuid: ProviderModelsDataByUuid;
    saving: boolean;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    onRuleChange?: (updatedRule: Rule) => void;
    onProviderModelsChange?: (providerUuid: string, models: ProviderModelData) => void;
    onRefreshProvider?: (providerUuid: string) => void;
    onModelSelectOpen: (ruleUuid: string, configRecord: ConfigRecord, mode: 'edit' | 'add', providerUuid?: string) => void;
    collapsible?: boolean;
    initiallyExpanded?: boolean;
    allowDeleteRule?: boolean;
    onRuleDelete?: (ruleUuid: string) => void;
    allowToggleRule?: boolean;
    onToggleExpanded?: () => void;
}

export const RuleCard: React.FC<RuleCardProps> = ({
    rule,
    providers,
    providerModelsByUuid,
    saving,
    showNotification,
    onRuleChange,
    onProviderModelsChange,
    onRefreshProvider,
    onModelSelectOpen,
    collapsible = false,
    initiallyExpanded = !collapsible,
    allowDeleteRule = false,
    onRuleDelete,
    allowToggleRule = true,
    onToggleExpanded,
}) => {
    // Expansion state management
    const { expanded, handleToggleExpanded } = useRuleCardExpanded({
        collapsible,
        initiallyExpanded,
        onToggleExpanded,
    });

    // ConfigRecord state management
    const { configRecord, setConfigRecord } = useRuleCardData({ rule, providers });

    // Auto-save functionality
    const { autoSave, updateField } = useRuleAutoSave({
        rule,
        onRuleChange,
        showNotification,
    });

    // Export functionality
    const { handleExport, handleExportAsJsonlToClipboard, handleExportAsBase64ToClipboard } = useRuleExport({ rule, showNotification });

    // Smart routing handlers
    const { dialogState: smartDialogState, handlers: smartHandlers } = useSmartRoutingHandlers({
        configRecord,
        setConfigRecord,
        autoSave,
        ruleUuid: rule.uuid,
        onModelSelectOpen,
        showNotification,
    });

    // Delete confirmation state
    const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

    // Handler: Switch routing mode (simple toggle, preserves data)
    const handleRoutingModeSwitch = useCallback(async () => {
        if (!configRecord) return;

        // Simply toggle the smartEnabled flag, preserve all data
        await updateField(configRecord, setConfigRecord, 'smartEnabled', !configRecord.smartEnabled);
    }, [configRecord, updateField]);

    // Handler: Delete provider
    const handleDeleteProvider = useCallback(
        async (_recordId: string, providerId: string) => {
            if (configRecord) {
                const updated = {
                    ...configRecord,
                    providers: configRecord.providers.filter((p) => p.uuid !== providerId),
                };
                await updateField(configRecord, setConfigRecord, 'providers', updated.providers);
            }
        },
        [configRecord, updateField]
    );

    // Handler: Provider node click
    const handleProviderNodeClick = useCallback(
        (providerUuid: string) => {
            if (configRecord) {
                onModelSelectOpen(rule.uuid, configRecord, 'edit', providerUuid);
            }
        },
        [configRecord, rule.uuid, onModelSelectOpen]
    );

    // Handler: Add provider button click
    const handleAddProviderButtonClick = useCallback(() => {
        if (configRecord) {
            onModelSelectOpen(rule.uuid, configRecord, 'add');
        }
    }, [configRecord, rule.uuid, onModelSelectOpen]);

    // Adapter: Convert ruleUuid to ruleIndex for smart routing handlers
    const handleAddServiceToSmartRuleByUuid = useCallback(
        (ruleUuid: string) => {
            const index = configRecord?.smartRouting?.findIndex((r) => r.uuid === ruleUuid) ?? -1;
            if (index >= 0) {
                smartHandlers.handleAddServiceToSmartRule(index);
            }
        },
        [configRecord, smartHandlers]
    );

    // Handler: Delete button click
    const handleDeleteButtonClick = useCallback(() => {
        setDeleteDialogOpen(true);
    }, []);

    // Handler: Confirm delete rule
    const confirmDeleteRule = useCallback(async () => {
        if (!onRuleDelete || !rule.uuid) {
            setDeleteDialogOpen(false);
            return;
        }

        try {
            const result = await api.deleteRule(rule.uuid);
            if (result.success) {
                showNotification('Rule deleted successfully!', 'success');
                onRuleDelete(rule.uuid);
            } else {
                showNotification(`Failed to delete rule: ${result.error || 'Unknown error'}`, 'error');
            }
        } catch (error) {
            console.error('Error deleting rule:', error);
            showNotification('Failed to delete routing rule', 'error');
        } finally {
            setDeleteDialogOpen(false);
        }
    }, [rule.uuid, onRuleDelete, showNotification]);

    if (!configRecord) return null;

    const isSmartMode = rule.smart_enabled;

    // Extra actions menu - shared between RoutingGraph and SmartRoutingGraph
    const extraActions = (
        <GraphSettingsMenu
            allowDeleteRule={allowDeleteRule}
            active={configRecord.active}
            allowToggleRule={allowToggleRule}
            saving={saving}
            onExport={handleExport}
            onExportAsJsonlToClipboard={handleExportAsJsonlToClipboard}
            onExportAsBase64ToClipboard={handleExportAsBase64ToClipboard}
            onDelete={handleDeleteButtonClick}
            onToggleActive={() => updateField(configRecord, setConfigRecord, 'active', !configRecord.active)}
            ruleUuid={rule.uuid}
            ruleName={rule.request_model || rule.uuid}
            scenario={rule.scenario}
            model={rule.request_model}
        />
    );

    return (
        <>
            {isSmartMode ? (
                <SmartRoutingGraph
                    record={configRecord}
                    providers={providers}
                    active={configRecord.active}
                    saving={saving}
                    collapsible={collapsible}
                    allowToggleRule={allowToggleRule}
                    expanded={expanded}
                    onToggleExpanded={handleToggleExpanded}
                    extraActions={extraActions}
                    onUpdateRecord={(field, value) => updateField(configRecord, setConfigRecord, field, value)}
                    onAddSmartRule={smartHandlers.handleAddSmartRule}
                    onEditSmartRule={smartHandlers.handleEditSmartRule}
                    onDeleteSmartRule={smartHandlers.handleDeleteSmartRule}
                    onAddServiceToSmartRule={smartHandlers.handleAddServiceToSmartRule}
                    onDeleteServiceFromSmartRule={smartHandlers.handleDeleteServiceFromSmartRule}
                    onAddDefaultProvider={handleAddProviderButtonClick}
                    onDeleteDefaultProvider={smartHandlers.handleDeleteDefaultProvider}
                    onProviderNodeClick={handleProviderNodeClick}
                    onSwitchRoutingMode={handleRoutingModeSwitch}
                />
            ) : (
                <RoutingGraph
                    record={configRecord}
                    recordUuid={configRecord.uuid}
                    providers={providers}
                    saving={saving}
                    expanded={expanded}
                    collapsible={collapsible}
                    allowToggleRule={allowToggleRule}
                    onUpdateRecord={(field, value) => updateField(configRecord, setConfigRecord, field, value)}
                    onDeleteProvider={handleDeleteProvider}
                    onToggleExpanded={handleToggleExpanded}
                    onProviderNodeClick={handleProviderNodeClick}
                    onAddProviderButtonClick={handleAddProviderButtonClick}
                    extraActions={extraActions}
                    onAddSmartRule={smartHandlers.handleAddSmartRule}
                    onEditSmartRule={smartHandlers.handleEditSmartRule}
                    onDeleteSmartRule={smartHandlers.handleDeleteSmartRule}
                    onAddServiceToSmartRule={handleAddServiceToSmartRuleByUuid}
                    onDeleteServiceFromSmartRule={smartHandlers.handleDeleteServiceFromSmartRule}
                    onSwitchRoutingMode={handleRoutingModeSwitch}
                />
            )}

            {/* Delete Confirmation Dialog */}
            <RuleCardDeleteDialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)} onConfirm={confirmDeleteRule} />

            {/* Smart Rule Edit Dialog */}
            <SmartRuleEditDialog
                open={smartDialogState.open}
                smartRouting={smartDialogState.editingRule}
                onSave={smartHandlers.handleSaveSmartRule}
                onCancel={smartHandlers.handleCancelSmartRuleEdit}
            />
        </>
    );
};

export default RuleCard;
