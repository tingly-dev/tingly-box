import React, { useCallback, useState, useRef, useEffect } from 'react';
import { Add as AddIcon, Key as KeyIcon, ExpandMore as ExpandMoreIcon, UnfoldMore as UnfoldMoreIcon } from '@mui/icons-material';
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
    showExpandCollapseButton?: boolean;
    // Allow custom rightAction for backward compatibility
    rightAction?: React.ReactNode;
    // Header height from parent component for calculating available space
    headerHeight?: number;
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
    showExpandCollapseButton = true,
    rightAction: customRightAction,
    headerHeight = 0,
}) => {
    const navigate = useNavigate();
    const [providerModelsByUuid, setProviderModelsByUuid] = useState<ProviderModelsDataByUuid>({});
    const [refreshingProviders, setRefreshingProviders] = useState<string[]>([]);
    const scrollContainerRef = useRef<HTMLDivElement>(null);
    const lastRuleRef = useRef<HTMLDivElement>(null);
    const [newRuleUuid, setNewRuleUuid] = useState<string | null>(null);
    const [allExpanded, setAllExpanded] = useState<boolean>(true);
    const [expandedStates, setExpandedStates] = useState<Record<string, boolean>>({});

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
                // Trigger parent to reload rules first
                onRulesChange?.([...rules, result.data]);
                // Set new rule UUID for scrolling after DOM is fully updated
                // Use double RAF to ensure parent component has re-rendered
                requestAnimationFrame(() => {
                    requestAnimationFrame(() => {
                        setNewRuleUuid(result.data.uuid);
                    });
                });
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

    // Handle expand/collapse all
    const handleToggleExpandAll = useCallback(() => {
        const newState = !allExpanded;
        setAllExpanded(newState);
        const newStates: Record<string, boolean> = {};
        rules.forEach(rule => {
            newStates[rule.uuid] = newState;
        });
        setExpandedStates(newStates);
    }, [allExpanded, rules]);

    // Handle individual rule expand/collapse
    const handleRuleExpandToggle = useCallback((ruleUuid: string) => {
        setExpandedStates(prev => {
            const newStates = { ...prev, [ruleUuid]: !prev[ruleUuid] };
            // Check if all rules have the same expanded state
            const states = Object.values(newStates);
            const allSame = states.every(s => s === states[0]);
            if (allSame) {
                setAllExpanded(states[0]);
            }
            return newStates;
        });
    }, []);

    // Initialize expanded states when rules change
    useEffect(() => {
        if (collapsible) {
            const initialStates: Record<string, boolean> = {};
            rules.forEach(rule => {
                if (!(rule.uuid in expandedStates)) {
                    initialStates[rule.uuid] = allExpanded;
                }
            });
            if (Object.keys(initialStates).length > 0) {
                setExpandedStates(prev => ({ ...prev, ...initialStates }));
            }
        }
    }, [rules, collapsible, allExpanded]);

    // Generate unified rightAction if not provided
    const rightAction = customRightAction ?? (
        <Stack direction="row" spacing={1}>
            {showExpandCollapseButton && collapsible && (
                <Tooltip title={allExpanded ? "Collapse all rules" : "Expand all rules"}>
                    <Button
                        variant="outlined"
                        startIcon={allExpanded ? <UnfoldMoreIcon /> : <ExpandMoreIcon />}
                        onClick={handleToggleExpandAll}
                        size="small"
                    >
                        {allExpanded ? "Collapse" : "Expand"}
                    </Button>
                </Tooltip>
            )}
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
                        maxHeight: headerHeight > 0
                            ? `calc(80vh - ${headerHeight + 0}px)`
                            : 'calc(80vh - 180px)',
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
                                        initiallyExpanded={expandedStates[rule.uuid] ?? collapsible}
                                        onModelSelectOpen={openModelSelectDialog}
                                        allowDeleteRule={allowDeleteRule}
                                        onRuleDelete={onRuleDelete}
                                        allowToggleRule={allowToggleRule}
                                        onToggleExpanded={() => handleRuleExpandToggle(rule.uuid)}
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
