import React, { useCallback, useState, useRef, useEffect } from 'react';
import { Add as AddIcon, Key as KeyIcon } from '@mui/icons-material';
import { Button, Stack, Tooltip, Box } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { v4 as uuidv4 } from 'uuid';
import ApiKeyModal from '@/components/ApiKeyModal';
import RuleCard from './RuleCard.tsx';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '../services/api';
import type { Provider, ProviderModelsDataByUuid } from '../types/provider';
import type { Rule } from './RoutingGraphTypes.ts';
import { useModelSelectDialog } from '../hooks/useModelSelectDialog';

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
    newlyCreatedRuleUuids?: Set<string>;
    // Unified action buttons props
    scenario?: string;
    showAddApiKeyButton?: boolean;
    showCreateRuleButton?: boolean;
    // Allow custom rightAction for backward compatibility
    rightAction?: React.ReactNode;
}

const TemplatePage: React.FC<TabTemplatePageProps> = ({
    rules,
    showTokenModal,
    setShowTokenModal,
    token,
    showNotification,
    providers,
    onRulesChange,
    title="",
    collapsible = false,
    allowDeleteRule = false,
    onRuleDelete,
    allowToggleRule = true,
    newlyCreatedRuleUuids,
    scenario,
    showAddApiKeyButton = true,
    showCreateRuleButton = true,
    rightAction: customRightAction,
}) => {
    const navigate = useNavigate();
    const [providerModelsByUuid, setProviderModelsByUuid] = useState<ProviderModelsDataByUuid>({});
    const [refreshingProviders, setRefreshingProviders] = useState<string[]>([]);
    const scrollContainerRef = useRef<HTMLDivElement>(null);
    const lastRuleRef = useRef<HTMLDivElement>(null);
    const [newRuleUuid, setNewRuleUuid] = useState<string | null>(null);

    const handleRuleChange = useCallback((updatedRule: Rule) => {
        if (onRulesChange) {
            const updatedRules = rules.map(r =>
                r.uuid === updatedRule.uuid ? updatedRule : r
            );
            onRulesChange(updatedRules);
        }
    }, [rules, onRulesChange]);

    const handleProviderModelsChange = useCallback((providerUuid: string, models: any) => {
        setProviderModelsByUuid((prev: any) => ({
            ...prev,
            [providerUuid]: models,
        }));
    }, []);

    const handleRefreshModels = useCallback(async (providerUuid: string) => {
        if (!providerUuid) return;

        try {
            setRefreshingProviders(prev => [...prev, providerUuid]);
            const result = await api.updateProviderModelsByUUID(providerUuid);
            if (result.success && result.data) {
                setProviderModelsByUuid((prev: any) => ({
                    ...prev,
                    [providerUuid]: result.data,
                }));
                showNotification(`Models refreshed successfully!`, 'success');
            } else {
                showNotification(`Failed to refresh models: ${result.message}`, 'error');
            }
        } catch (error) {
            console.error('Error refreshing models:', error);
            showNotification(`Error refreshing models`, 'error');
        } finally {
            setRefreshingProviders(prev => prev.filter(p => p !== providerUuid));
        }
    }, [showNotification]);

    // Unified action handlers
    const handleAddApiKeyClick = useCallback(() => {
        navigate('/api-keys?dialog=add');
    }, [navigate]);

    const handleCreateRule = useCallback(async () => {
        if (!scenario) return;

        try {
            const newRuleData = {
                scenario: scenario,
                request_model: `model-${uuidv4().slice(0, 8)}`,
                response_model: '',
                active: true,
                services: []
            };
            const result = await api.createRule('', newRuleData);
            if (result.success && result.data?.uuid) {
                // Set new rule UUID for scrolling
                setNewRuleUuid(result.data.uuid);
                // Trigger parent to reload rules and add to newlyCreatedRuleUuids
                onRulesChange?.([...rules, result.data]);
                showNotification('Routing rule created successfully!', 'success');
            } else {
                showNotification(`Failed to create rule: ${result.error || 'Unknown error'}`, 'error');
            }
        } catch (error) {
            console.error('Error creating rule:', error);
            showNotification('Failed to create routing rule', 'error');
        }
    }, [scenario, rules, onRulesChange, showNotification]);

    // Use the model select dialog hook
    const { openModelSelect, ModelSelectDialog, isOpen: modelSelectDialogOpen } = useModelSelectDialog({
        providers,
        rules,
        onRuleChange: handleRuleChange,
        showNotification,
    });

    // Wrapper to maintain compatibility with existing RuleCard interface
    const openModelSelectDialog = useCallback((
        ruleUuid: string,
        configRecord: any,
        mode: 'edit' | 'add',
        providerUuid?: string
    ) => {
        openModelSelect({ ruleUuid, configRecord, providerUuid, mode });
    }, [openModelSelect]);

    // Generate unified rightAction if not provided
    const rightAction = customRightAction ?? (
        (showAddApiKeyButton || showCreateRuleButton) ? (
            <Stack direction="row" spacing={1}>
                {showAddApiKeyButton && (
                    <Tooltip title="Add new API Key">
                        <Button
                            variant="outlined"
                            startIcon={<KeyIcon />}
                            onClick={handleAddApiKeyClick}
                            size="small"
                        >
                            New Key
                        </Button>
                    </Tooltip>
                )}
                {showCreateRuleButton && (
                    <Tooltip title="Create new routing rule">
                        <Button
                            variant="contained"
                            startIcon={<AddIcon />}
                            onClick={handleCreateRule}
                            size="small"
                        >
                            New Rule
                        </Button>
                    </Tooltip>
                )}
            </Stack>
        ) : null
    );

    // Scroll to new rule when it's created (within the scrollable container)
    useEffect(() => {
        if (newRuleUuid && lastRuleRef.current && scrollContainerRef.current) {
            const container = scrollContainerRef.current;
            const target = lastRuleRef.current;
            const containerRect = container.getBoundingClientRect();
            const targetRect = target.getBoundingClientRect();

            // Calculate the scroll position to show the target at the top of the container
            const scrollTop = target.offsetTop - container.offsetTop;

            container.scrollTo({
                top: scrollTop,
                behavior: 'smooth'
            });

            // Clear the new rule UUID after scrolling
            setNewRuleUuid(null);
        }
    }, [newRuleUuid]);

    if (!providers.length || !rules?.length) {
        return null;
    }

    return (
        <>
            <UnifiedCard size="full" title={title} rightAction={rightAction}>
                <Box
                    ref={scrollContainerRef}
                    sx={{
                        maxHeight: 'calc(80vh - 120px)',
                        overflowY: 'auto',
                        '&::-webkit-scrollbar': {
                            width: '8px',
                        },
                        '&::-webkit-scrollbar-track': {
                            backgroundColor: 'transparent',
                        },
                        '&::-webkit-scrollbar-thumb': {
                            backgroundColor: 'rgba(0, 0, 0, 0.2)',
                            borderRadius: '4px',
                            '&:hover': {
                                backgroundColor: 'rgba(0, 0, 0, 0.3)',
                            },
                        },
                    }}
                >
                    {rules.map((rule, index) => {
                        const isNewRule = rule.uuid === newRuleUuid;
                        const isLastRule = index === rules.length - 1;
                        const shouldAttachRef = isNewRule || (isLastRule && !newRuleUuid);

                        return (
                            <div key={rule.uuid} ref={shouldAttachRef ? lastRuleRef : null}>
                                {rule && rule.uuid && (
                                    <RuleCard
                                        rule={rule}
                                        providers={providers}
                                        providerModelsByUuid={providerModelsByUuid}
                                        saving={refreshingProviders.length > 0}
                                        showNotification={showNotification}
                                        onRuleChange={handleRuleChange}
                                        onProviderModelsChange={handleProviderModelsChange}
                                        onRefreshProvider={handleRefreshModels}
                                        collapsible={collapsible}
                                        initiallyExpanded={
                                            collapsible
                                        }
                                        onModelSelectOpen={openModelSelectDialog}
                                        allowDeleteRule={allowDeleteRule}
                                        onRuleDelete={onRuleDelete}
                                        allowToggleRule={allowToggleRule}
                                    />
                                )}
                            </div>
                        );
                    })}
                </Box>
            </UnifiedCard>

            <ModelSelectDialog open={modelSelectDialogOpen} onClose={() => {}} />

            <ApiKeyModal
                open={showTokenModal}
                onClose={() => setShowTokenModal(false)}
                token={token}
                onCopy={async (text, label) => {
                    try {
                        await navigator.clipboard.writeText(text);
                        showNotification(`${label} copied to clipboard!`, 'success');
                    } catch (err) {
                        showNotification('Failed to copy to clipboard', 'error');
                    }
                }}
            />
        </>
    );
};

export default TemplatePage;
