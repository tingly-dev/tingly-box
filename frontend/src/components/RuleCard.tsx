import { useCallback, useState } from 'react';
import { api } from '@/services/api';
import type { Provider, ProviderModelsDataByUuid, ProviderModelData } from '@/types/provider';
import type { ConfigRecord, Rule } from '@/components/RoutingGraphTypes';
import {
    useRuleCardExpanded,
    useRuleCardData,
    useRuleAutoSave,
    useRuleProbe,
    useRuleExport,
    useSmartRoutingHandlers,
} from '@/components/rule-card/useRuleCardHooks';
import { RuleCardProbeDialog, RuleCardDeleteDialog } from '@/components/rule-card/dialogs';
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
    const { updateField } = useRuleAutoSave({
        rule,
        onRuleChange,
        showNotification,
    });

    // Probe functionality
    const probeState = useRuleProbe(configRecord);

    // Export functionality
    const { handleExport } = useRuleExport({ rule, showNotification });

    // Smart routing handlers
    const { dialogState: smartDialogState, handlers: smartHandlers } = useSmartRoutingHandlers({
        configRecord,
        setConfigRecord,
        autoSave: async () => {
            // Minimal auto-save implementation for smart routing handlers
            // In a future refactor, this could be consolidated
            return true;
        },
        ruleUuid: rule.uuid,
        onModelSelectOpen,
        showNotification,
    });

    // Delete confirmation state
    const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

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
            smartEnabled={isSmartMode ?? false}
            canProbe={!!configRecord.providers[0]?.provider && !!configRecord.providers[0]?.model}
            isProbing={probeState.isProbing}
            allowDeleteRule={allowDeleteRule}
            active={configRecord.active}
            allowToggleRule={allowToggleRule}
            saving={saving}
            onToggleSmartRouting={() => updateField(configRecord, setConfigRecord, 'smartEnabled', !isSmartMode)}
            onProbe={probeState.handleProbe}
            onExport={handleExport}
            onDelete={handleDeleteButtonClick}
            onToggleActive={() => updateField(configRecord, setConfigRecord, 'active', !configRecord.active)}
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
                />
            )}

            {/* Probe Result Dialog */}
            <RuleCardProbeDialog
                open={probeState.dialogOpen}
                onClose={probeState.handleCloseDialog}
                configRecord={configRecord}
                isProbing={probeState.isProbing}
                probeResult={probeState.probeResult}
                detailsExpanded={probeState.detailsExpanded}
                providerName={probeState.providerName}
                onToggleDetails={probeState.handleToggleDetails}
            />

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
