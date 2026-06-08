import { useCallback, useEffect, useState } from 'react';
import { api } from '@/services/api';
import type { Provider, ProviderModelsDataByUuid, ProviderModelData } from '@/types/provider';
import type { ConfigRecord, FlagSpec, Rule, RuleFlags } from '@/components/RoutingGraphTypes';
import {
    useRuleCardExpanded,
    useRuleCardData,
    useRuleAutoSave,
    useRuleExport,
    useSmartRoutingHandlers,
} from '@/components/rule-card/useRuleCardHooks';
import { RuleCardDeleteDialog, RuleFlagEditDialog } from '@/components/rule-card/dialogs';
import UnifiedRoutingGraph from '@/components/UnifiedRoutingGraph';
import SmartRuleCatalogDialog from '@/components/rule-card/SmartRuleCatalogDialog';
import GraphSettingsMenu from '@/components/GraphSettingsMenu';
import RulePluginsCard from '@/components/rule-card/RulePluginsCard';
import FlagCatalogDialog from '@/components/rule-card/FlagCatalogDialog';
import { formatRuleFlags, parseRuleFlags } from '@/components/rule-card/utils';
import { getFlagValue, setFlagValue } from '@/components/rule-card/flagHelpers';

// Module-level cache so we only fetch the flag catalog once per session.
// `undefined` = never fetched; `[]` = fetched but empty (don't re-fetch).
let _flagRegistryCache: FlagSpec[] | undefined = undefined;
let _flagRegistryPromise: Promise<FlagSpec[]> | null = null;

async function loadFlagRegistry(): Promise<FlagSpec[]> {
    if (_flagRegistryCache !== undefined) return _flagRegistryCache;
    if (_flagRegistryPromise) return _flagRegistryPromise;
    _flagRegistryPromise = (async () => {
        try {
            const result = await api.getRuleFlagRegistry();
            const data: FlagSpec[] = Array.isArray(result?.data) ? result.data : [];
            _flagRegistryCache = data;
            return data;
        } catch {
            // Don't cache failures — allow retry on next mount.
            return [];
        } finally {
            _flagRegistryPromise = null;
        }
    })();
    return _flagRegistryPromise;
}

export interface RuleCardProps {
    rule: Rule;
    providers: Provider[];
    providerModelsByUuid: ProviderModelsDataByUuid;
    saving: boolean;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    onRuleChange?: (updatedRule: Rule) => void;
    onProviderModelsChange?: (providerUuid: string, models: ProviderModelData) => void;
    onRefreshProvider?: (providerUuid: string) => void;
    onModelSelectOpen: (ruleUuid: string, configRecord: ConfigRecord, mode: 'edit' | 'add', providerUuid?: string, addTier?: number) => void;
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
    const [flagDialogOpen, setFlagDialogOpen] = useState(false);
    const [flagInput, setFlagInput] = useState('');
    const [flagError, setFlagError] = useState<string | undefined>(undefined);

    // Catalog dialog state + registry
    const [catalogOpen, setCatalogOpen] = useState(false);
    const [flagRegistry, setFlagRegistry] = useState<FlagSpec[]>(_flagRegistryCache ?? []);
    const [registryLoaded, setRegistryLoaded] = useState(_flagRegistryCache !== undefined);
    const [registryLoading, setRegistryLoading] = useState(false);

    useEffect(() => {
        if (registryLoaded) return;
        let cancelled = false;
        setRegistryLoading(true);
        loadFlagRegistry()
            .then((data) => {
                if (!cancelled) {
                    setFlagRegistry(data);
                    setRegistryLoaded(true);
                }
            })
            .finally(() => {
                if (!cancelled) setRegistryLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, [registryLoaded]);

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

    // Handler: Add provider button click — optional tier assigns the new service to a tier
    const handleAddServiceButtonClick = useCallback((tier?: number) => {
        if (configRecord) {
            onModelSelectOpen(rule.uuid, configRecord, 'add', undefined, tier);
        }
    }, [configRecord, rule.uuid, onModelSelectOpen]);

    // Handler: Update a service's tier. Setting any service's tier to > 0
    // flips the rule into "tier" tactic on save (handled in pickLbTactic).
    const handleProviderTierChange = useCallback(
        async (providerUuid: string, tier: number) => {
            if (!configRecord) return;
            const updated = configRecord.providers.map((p) =>
                p.uuid === providerUuid ? { ...p, tier } : p,
            );
            await updateField(configRecord, setConfigRecord, 'providers', updated);
        },
        [configRecord, updateField, setConfigRecord]
    );

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

    const handleOpenFlagEditor = useCallback(() => {
        if (!configRecord) return;
        const currentFlags = formatRuleFlags(configRecord.flags, flagRegistry);
        if (!currentFlags && configRecord.requestModel === 'cursor') {
            setFlagInput('cursor_compat=true');
        } else {
            setFlagInput(currentFlags);
        }
        setFlagError(undefined);
        setFlagDialogOpen(true);
    }, [configRecord, flagRegistry]);

    const handleSaveFlags = useCallback(async () => {
        if (!configRecord) return;
        const result = parseRuleFlags(flagInput, flagRegistry, configRecord.flags);
        if (result.error) {
            setFlagError(result.error);
            return;
        }
        const success = await updateField(configRecord, setConfigRecord, 'flags', result.flags);
        if (success) setFlagDialogOpen(false);
    }, [configRecord, flagInput, flagRegistry, updateField, setConfigRecord]);

    const handleSaveCatalogFlags = useCallback(async (next: RuleFlags) => {
        if (!configRecord) return;
        const success = await updateField(configRecord, setConfigRecord, 'flags', next);
        if (success) setCatalogOpen(false);
    }, [configRecord, updateField, setConfigRecord]);

    const handleToggleFlagFromCard = useCallback((key: string) => {
        if (!configRecord) return;
        const current = configRecord.flags || {};
        const next = setFlagValue(current, key, !getFlagValue(current, key));
        void updateField(configRecord, setConfigRecord, 'flags', next);
    }, [configRecord, updateField, setConfigRecord]);

    if (!configRecord) return null;

    const extensionsCard = (
        <RulePluginsCard
            flags={configRecord.flags}
            registry={flagRegistry}
            active={configRecord.active}
            onOpenCatalog={() => setCatalogOpen(true)}
            onToggleFlag={handleToggleFlagFromCard}
        />
    );

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
            onEditFlags={handleOpenFlagEditor}
            ruleUuid={rule.uuid}
            ruleName={rule.request_model || rule.uuid}
            scenario={rule.scenario}
            model={rule.request_model}
        />
    );

    return (
        <>
            <UnifiedRoutingGraph
                mode="auto"
                record={configRecord}
                providers={providers}
                active={configRecord.active}
                saving={saving}
                collapsible={collapsible}
                expanded={expanded}
                onToggleExpanded={handleToggleExpanded}
                extraActions={extraActions}
                extensionsCard={extensionsCard}
                onUpdateRecord={(field, value) => updateField(configRecord, setConfigRecord, field, value)}
                onProviderNodeClick={handleProviderNodeClick}
                onTierChange={handleProviderTierChange}
                onDeleteProvider={(providerUuid) => handleDeleteProvider(configRecord.uuid, providerUuid)}
                onAddService={handleAddServiceButtonClick}
                onAddSmartRule={smartHandlers.handleAddSmartRule}
                onEditSmartRule={smartHandlers.handleEditSmartRule}
                onDeleteSmartRule={smartHandlers.handleDeleteSmartRule}
                onMoveSmartRule={smartHandlers.handleMoveSmartRule}
                onAddServiceToSmartRule={handleAddServiceToSmartRuleByUuid}
                onDeleteServiceFromSmartRule={smartHandlers.handleDeleteServiceFromSmartRule}
                onSwitchRoutingMode={handleRoutingModeSwitch}
            />

            {/* Delete Confirmation Dialog */}
            <RuleCardDeleteDialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)} onConfirm={confirmDeleteRule} />

            {/* Flag Edit Dialog */}
            <RuleFlagEditDialog
                open={flagDialogOpen}
                value={flagInput}
                error={flagError}
                onChange={(value) => {
                    setFlagInput(value);
                    if (flagError) setFlagError(undefined);
                }}
                onClose={() => setFlagDialogOpen(false)}
                onSave={handleSaveFlags}
            />

            {/* Flag Catalog Dialog - rich UI for picking + configuring rule flags */}
            <FlagCatalogDialog
                open={catalogOpen}
                flags={configRecord.flags}
                registry={flagRegistry}
                loading={registryLoading}
                providers={providers}
                onClose={() => setCatalogOpen(false)}
                onSave={handleSaveCatalogFlags}
            />

            {/* Smart Rule Edit Dialog (catalog-style: conditions sidebar + detail pane) */}
            <SmartRuleCatalogDialog
                open={smartDialogState.open}
                smartRouting={smartDialogState.editingRule}
                onClose={smartHandlers.handleCancelSmartRuleEdit}
                onSave={smartHandlers.handleSaveSmartRule}
            />
        </>
    );
};

export default RuleCard;
