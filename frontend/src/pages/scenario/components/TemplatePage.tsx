import ApiKeyModal from '@/components/ApiKeyModal';
import React, {useCallback, useEffect, useState} from 'react';
import {Alert, Box, Fab, Snackbar} from '@mui/material';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import {useNavigate} from 'react-router-dom';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import RuleCard from '@/components/RuleCard.tsx';
import ImportModal from '@/components/ImportModal';
import UnifiedCard from '@/components/UnifiedCard';
import type {TabTemplatePageProps} from './TemplatePage.types';
import {TemplatePageActions} from './TemplatePageActions';
import {useTemplatePageRules} from '@/pages/scenario/hooks/useTemplatePageRules';
import {useScrollToNewRule} from '@/components/hooks/useScrollToNewRule';
import {useModelSelectDialog} from '@/hooks/useModelSelectDialog';
import {useScenarioPageInternal} from '@/pages/scenario/hooks/useScenarioPageInternal';
import api from '@/services/api';

/**
 * TemplatePage component that supports two modes:
 *
 * INTERNAL MODE (recommended):
 * Only provide `scenario` prop - TemplatePage fetches all data internally.
 * <TemplatePage scenario="agent" />
 *
 * EXTERNAL MODE (legacy):
 * Provide all props - for backward compatibility with existing code.
 * <TemplatePage rules={rules} providers={providers} scenario="agent" ... />
 */
const TemplatePage: React.FC<TabTemplatePageProps> = (props) => {
    // Determine which mode we're in
    // If `rules` is provided, use external mode (legacy)
    // If only `scenario` is provided, use internal mode (recommended)
    const isInternalMode = !props.rules && props.scenario;

    // Internal mode: fetch all data internally
    const internalData = useScenarioPageInternal(
        isInternalMode ? (props.scenario as string) : ''
    );

    // External mode: use props directly
    const {
        rules = internalData.rules,
        showTokenModal = internalData.showTokenModal,
        setShowTokenModal = internalData.setShowTokenModal,
        token = internalData.token,
        showNotification = internalData.showNotification,
        providers = internalData.providers,
        onRulesChange = internalData.handleRulesChange,
        onProvidersLoad = internalData.loadProviders,
        loadRules = internalData.loadRules,
        title = "",
        collapsible = false,
        allowDeleteRule = false,
        onRuleDelete = internalData.handleRuleDelete,
        allowToggleRule = true,
        allowAddRule = true,
        scenario,
        showAddApiKeyButton = true,
        showCreateRuleButton = true,
        showExpandCollapseButton = true,
        showImportButton = true,
        showEmptyState = true,
        rightAction: customRightAction,
        onAddApiKeyClick,
        newlyCreatedRuleUuids = internalData.newlyCreatedRuleUuids,
    } = props;

    const isLoading = isInternalMode ? internalData.isLoading : false;

    const navigate = useNavigate();
    const [allExpanded, setAllExpanded] = useState<boolean>(true);
    const [expandedStates, setExpandedStates] = useState<Record<string, boolean>>({});
    const [showScrollTop, setShowScrollTop] = useState<boolean>(false);
    const [showImportModal, setShowImportModal] = useState<boolean>(false);
    const [importing, setImporting] = useState<boolean>(false);
    const [importError, setImportError] = useState<{ open: boolean; message: string }>({open: false, message: ''});

    // Custom hooks
    const {
        providerModelsByUuid,
        refreshingProviders,
        handleRuleChange,
        handleProviderModelsChange,
        handleRefreshModels,
        handleCreateRule: createRule,
    } = useTemplatePageRules({
        rules,
        onRulesChange,
        showNotification,
        scenario,
        loadRules,
    });

    const {
        scrollContainerRef,
        lastRuleRef,
        newRuleUuid,
        setNewRuleUuid,
    } = useScrollToNewRule({rules});

    // Model select dialog
    const {openModelSelect, ModelSelectDialog, isOpen: modelSelectDialogOpen} = useModelSelectDialog({
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
        openModelSelect({ruleUuid, configRecord, providerUuid, mode});
    }, [openModelSelect]);

    // Unified action handlers
    const handleAddApiKeyClick = useCallback(() => {
        navigate('/api-keys?dialog=add');
    }, [navigate]);

    const handleCreateRule = useCallback(async () => {
        const newUuid = await createRule();
        if (newUuid) {
            // Set new rule UUID for scrolling after DOM is fully updated
            // Use double RAF to ensure parent component has re-rendered
            requestAnimationFrame(() => {
                requestAnimationFrame(() => {
                    setNewRuleUuid(newUuid);
                });
            });
        }
    }, [createRule, setNewRuleUuid]);

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
            const newStates = {...prev, [ruleUuid]: !prev[ruleUuid]};
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
                setExpandedStates(prev => ({...prev, ...initialStates}));
            }
        }
    }, [rules, collapsible, allExpanded]);

    // Handle scroll to show/hide the back-to-top button
    useEffect(() => {
        // Find the scroll container by looking for elements with overflow-y: auto
        const findScrollContainer = () => {
            const mainElement = document.querySelector('main');
            if (!mainElement) return null;
            const boxes = mainElement.querySelectorAll('div');
            for (const box of boxes) {
                const style = window.getComputedStyle(box);
                if (style.overflowY === 'auto' || style.overflowY === 'scroll') {
                    return box as HTMLElement;
                }
            }
            return null;
        };

        const scrollContainer = findScrollContainer();
        if (!scrollContainer) return;

        const handleScroll = () => {
            setShowScrollTop(scrollContainer.scrollTop > 200);
        };

        scrollContainer.addEventListener('scroll', handleScroll);
        return () => scrollContainer.removeEventListener('scroll', handleScroll);
    }, []);

    // Scroll to top handler
    const handleScrollToTop = useCallback(() => {
        const findScrollContainer = () => {
            const mainElement = document.querySelector('main');
            if (!mainElement) return null;
            const boxes = mainElement.querySelectorAll('div');
            for (const box of boxes) {
                const style = window.getComputedStyle(box);
                if (style.overflowY === 'auto' || style.overflowY === 'scroll') {
                    return box as HTMLElement;
                }
            }
            return null;
        };

        const scrollContainer = findScrollContainer();
        if (scrollContainer) {
            scrollContainer.scrollTo({top: 0, behavior: 'smooth'});
        }
    }, []);

    // Import from clipboard handler
    const handleImportFromClipboard = useCallback(() => {
        setShowImportModal(true);
    }, []);

    // Handle import data (from modal)
    const handleImportData = useCallback(async (data: string) => {
        setImporting(true);
        try {
            const result = await api.importRule(data);
            if (result.success) {
                // Refresh providers first to ensure newly imported providers are available
                if (onProvidersLoad) {
                    await onProvidersLoad();
                }
                // Then refresh rules by calling parent's onRulesChange
                // Only refresh if scenario is available (required by backend API)
                if (onRulesChange && scenario) {
                    const updatedRules = await api.getRules(scenario);
                    if (updatedRules.success) {
                        onRulesChange(updatedRules.data);
                    }
                } else if (onRulesChange) {
                    // If no scenario, trigger parent to refresh by calling without data
                    onRulesChange([] as any);
                }

                const createdMsg = result.data?.rule_created ? 'Rule created.' : '';
                const updatedMsg = result.data?.rule_updated ? 'Rule updated.' : '';
                const providersMsg = result.data?.providers_created > 0
                    ? ` ${result.data.providers_created} provider(s) imported.`
                    : result.data?.providers_used > 0
                        ? ` ${result.data.providers_used} existing provider(s) used.`
                        : '';
                showNotification(
                    `Rule imported successfully! ${createdMsg}${updatedMsg}${providersMsg}`,
                    'success'
                );
                setShowImportModal(false);
            } else {
                setImportError({open: true, message: result.error || 'Import failed'});
            }
        } catch (err) {
            setImportError({open: true, message: (err as Error).message || 'Import failed'});
        } finally {
            setImporting(false);
        }
    }, [showNotification, scenario, onRulesChange, onProvidersLoad]);

    // Generate unified rightAction if not provided
    const rightAction = customRightAction ?? (
        <TemplatePageActions
            collapsible={collapsible}
            allExpanded={allExpanded}
            onToggleExpandAll={handleToggleExpandAll}
            showAddApiKeyButton={showAddApiKeyButton}
            onAddApiKeyClick={handleAddApiKeyClick}
            allowAddRule={allowAddRule}
            onCreateRule={handleCreateRule}
            showExpandCollapseButton={showExpandCollapseButton}
            showImportButton={showImportButton}
            onImportFromClipboard={handleImportFromClipboard}
            scenario={scenario}
        />
    );

    if (!providers.length) {
        if (!showEmptyState) {
            return null;
        }

        return (
            <UnifiedCard size="full" title={title}>
                <EmptyStateGuide
                    title={"No Providers Configured"}
                    description={"Add an API key provider to start routing requests"}
                    onAddApiKeyClick={onAddApiKeyClick || handleAddApiKeyClick}
                />
            </UnifiedCard>
        );
    }

    return (
        <>
            <UnifiedCard size="full" title={title} rightAction={rightAction}>
                {/*<Box ref={scrollContainerRef} sx={SCROLLBOX_SX(headerHeight)}>*/}
                <Box ref={scrollContainerRef}>
                    {rules?.length === 0 ? (
                        <Box sx={{
                            textAlign: 'center',
                            py: 8,
                            color: 'text.secondary'
                        }}>
                            No rules yet. Click "Create Rule" to add one.
                        </Box>
                    ) : (
                        rules.map((rule, index) => {
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
                        })
                    )}
                </Box>
            </UnifiedCard>

            <ModelSelectDialog open={modelSelectDialogOpen} onClose={() => {
            }}/>

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

            <ImportModal
                open={showImportModal}
                onClose={() => setShowImportModal(false)}
                onImport={handleImportData}
                loading={importing}
            />

            {showScrollTop && (
                <Fab
                    color="primary"
                    size="small"
                    onClick={handleScrollToTop}
                    sx={{
                        position: 'fixed',
                        bottom: 50,
                        right: 80,
                        zIndex: 1000,
                    }}
                >
                    <KeyboardArrowUpIcon/>
                </Fab>
            )}
            <Snackbar
                open={importError.open}
                autoHideDuration={6000}
                onClose={() => setImportError({open: false, message: ''})}
                anchorOrigin={{vertical: 'bottom', horizontal: 'center'}}
            >
                <Alert severity="error" onClose={() => setImportError({open: false, message: ''})}>
                    {importError.message}
                </Alert>
            </Snackbar>
        </>
    );
};

export default TemplatePage;
