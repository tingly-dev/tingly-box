import React, { useCallback, useState, useEffect } from 'react';
import { Box, Fab } from '@mui/material';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import { useNavigate } from 'react-router-dom';
import ApiKeyModal from '@/components/ApiKeyModal';
import RuleCard from './RuleCard.tsx';
import UnifiedCard from '@/components/UnifiedCard';
import { TemplatePageActions } from './TemplatePageActions';
import type { TabTemplatePageProps } from './TemplatePage.types';
import { SCROLLBOX_SX } from './TemplatePage.constants';
import { useTemplatePageRules } from './hooks/useTemplatePageRules';
import { useScrollToNewRule } from './hooks/useScrollToNewRule';
import { useModelSelectDialog } from '../hooks/useModelSelectDialog';

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
    newlyCreatedRuleUuids: _newlyCreatedRuleUuids,
    scenario,
    showAddApiKeyButton = true,
    showCreateRuleButton = true,
    showExpandCollapseButton = true,
    rightAction: customRightAction,
    headerHeight = 0,
}) => {
    const navigate = useNavigate();
    const [allExpanded, setAllExpanded] = useState<boolean>(true);
    const [expandedStates, setExpandedStates] = useState<Record<string, boolean>>({});
    const [showScrollTop, setShowScrollTop] = useState<boolean>(false);

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
    });

    const {
        scrollContainerRef,
        lastRuleRef,
        newRuleUuid,
        setNewRuleUuid,
    } = useScrollToNewRule({ rules });

    // Model select dialog
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
            scrollContainer.scrollTo({ top: 0, behavior: 'smooth' });
        }
    }, []);

    // Generate unified rightAction if not provided
    const rightAction = customRightAction ?? (
        <TemplatePageActions
            collapsible={collapsible}
            allExpanded={allExpanded}
            onToggleExpandAll={handleToggleExpandAll}
            showAddApiKeyButton={showAddApiKeyButton}
            onAddApiKeyClick={handleAddApiKeyClick}
            showCreateRuleButton={showCreateRuleButton}
            onCreateRule={handleCreateRule}
            showExpandCollapseButton={showExpandCollapseButton}
        />
    );

    if (!providers.length || !rules?.length) {
        return null;
    }

    return (
        <>
            <UnifiedCard size="full" title={title} rightAction={rightAction}>
                {/*<Box ref={scrollContainerRef} sx={SCROLLBOX_SX(headerHeight)}>*/}
                <Box ref={scrollContainerRef}>
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
                    <KeyboardArrowUpIcon />
                </Fab>
            )}
        </>
    );
};

export default TemplatePage;
