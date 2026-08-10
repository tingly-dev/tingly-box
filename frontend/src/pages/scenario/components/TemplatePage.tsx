import ApiKeyModal from '@/components/ApiKeyModal';
import ScenarioLogDialog from '@/components/RuleLogDialog';
import React, {useCallback, useEffect, useRef, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {Box, Fab} from '@mui/material';
import { KeyboardArrowUp as KeyboardArrowUpIcon } from '@/components/icons';
import {useNavigate} from 'react-router-dom';
import EmptyState from '@/components/EmptyState';
import RuleCard from '@/components/RuleCard.tsx';
import ConnectAIDialogs from '@/components/ConnectAIDialogs';
import UnifiedCard from '@/components/UnifiedCard';
import { EntryGuideDialog } from '@/components/tier/EntryGuideDialog';
import type {TemplatePageProps} from './TemplatePage.types';
import {TemplatePageActions} from './TemplatePageActions';
import {TitleIconButtons} from './TitleIconButtons';
import {useTemplatePageRules} from '@/pages/scenario/hooks/useTemplatePageRules';
import {useRuleSort} from '@/pages/scenario/hooks/useRuleSort';
import {useScrollToNewRule} from '@/components/hooks/useScrollToNewRule';
import {useModelSelectDialog} from '@/hooks/useModelSelectDialog';
import {useProviderDialog} from '@/hooks/useProviderDialog';
import {useScenarioPageInternal} from '@/pages/scenario/hooks/useScenarioPageInternal';
import {useScenarioPageModal} from '@/pages/scenario/context/ScenarioPageContext';

// First-run education: the Direct routing guide auto-opens once per user (new
// and existing), then never again — the toolbar "?" stays as the manual
// re-entry point. localStorage persists the dismissal across sessions; the
// module flag guards against StrictMode double-invoke / quick remounts.
const ROUTING_GUIDE_SEEN_KEY = 'tb.routingGuideAutoShown';
let routingGuideAutoOpenedThisSession = false;

/**
 * TemplatePage component with internally-managed state and optional overrides.
 *
 * INTERNAL MODE (recommended):
 * Just provide `scenario` prop - TemplatePage fetches all data internally.
 * <TemplatePage scenario="custom" />
 *
 * HYBRID MODE (for custom logic):
 * Provide `scenario` plus override specific data props for custom behavior.
 * <TemplatePage scenario="custom" rules={customRules} onRulesChange={customHandler} />
 *
 * Modal state (ApiKeyModal) is shared via ScenarioPageModalProvider context.
 */
const TemplatePage: React.FC<TemplatePageProps> = (props) => {
    // Get modal state from context (shared with ProviderConfigCard)
    const { t } = useTranslation();
    const { showTokenModal, setShowTokenModal, token, copyToClipboard } = useScenarioPageModal();

    // Internal mode: fetch all data internally (excluding modal - that's from context)
    const internalData = useScenarioPageInternal(props.scenario);

    const {
        title = t('scenarioPage.modelRules'),
        collapsible = false,
        allowDeleteRule = false,
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
        onContext1MToggle,
    } = props;

    // Use provided props or fallback to internal data
    const rules = props.rules ?? internalData.rules;
    const showNotification = props.showNotification ?? internalData.showNotification;
    const providers = props.providers ?? internalData.providers;
    const onRulesChange = props.onRulesChange ?? internalData.handleRulesChange;
    const onProvidersLoad = props.onProvidersLoad ?? internalData.loadProviders;
    const loadRules = props.loadRules ?? internalData.loadRules;
    const onRuleDelete = props.onRuleDelete ?? internalData.handleRuleDelete;
    const newlyCreatedRuleUuids = internalData.newlyCreatedRuleUuids;
    const isLoading = internalData.isLoading;

    const navigate = useNavigate();
    const [allExpanded, setAllExpanded] = useState<boolean>(true);
    const [expandedStates, setExpandedStates] = useState<Record<string, boolean>>({});
    const [showScrollTop, setShowScrollTop] = useState<boolean>(false);
    const [logDialogOpen, setLogDialogOpen] = useState<boolean>(false);
    const [showGuide, setShowGuide] = useState<boolean>(false);

    // Auto-open the Direct guide the first time a user lands on a populated
    // routing page. Records the dismissal immediately so it never nags again;
    // the toolbar "?" reopens it on demand.
    const hasContent = providers.length > 0 && rules.length > 0;
    useEffect(() => {
        if (!hasContent || routingGuideAutoOpenedThisSession) return;
        let alreadySeen = false;
        try {
            alreadySeen = !!localStorage.getItem(ROUTING_GUIDE_SEEN_KEY);
        } catch {
            return; // storage unavailable — skip rather than risk re-opening
        }
        if (alreadySeen) return;
        routingGuideAutoOpenedThisSession = true;
        try {
            localStorage.setItem(ROUTING_GUIDE_SEEN_KEY, '1');
        } catch { /* best-effort */ }
        setShowGuide(true);
    }, [hasContent]);

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

    // Display-only ordering for the Model Rules list — doesn't touch `rules`
    // itself, so match priority (if any) and the scroll-to-new-rule logic
    // above stay anchored to the real, backend-ordered array.
    const {sortMode, toggleSortMode, sortedRules} = useRuleSort(rules);

    // Routed through a ref so onCreateFromModel doesn't capture a stale createRule.
    const createRuleRef = useRef(createRule);
    useEffect(() => {
        createRuleRef.current = createRule;
    }, [createRule]);

    const runCreateRuleAndScroll = useCallback(async (
        options?: { providerUuid: string; model: string }
    ) => {
        const newUuid = await createRuleRef.current(options);
        if (newUuid) {
            requestAnimationFrame(() => {
                requestAnimationFrame(() => {
                    setNewRuleUuid(newUuid);
                });
            });
        }
    }, [setNewRuleUuid]);

    // Model select dialog
    const {
        openModelSelect,
        openModelSelectForCreate,
        ModelSelectDialog,
        isOpen: modelSelectDialogOpen,
    } = useModelSelectDialog({
        providers,
        rules,
        onRuleChange: handleRuleChange,
        showNotification,
        onCreateFromModel: (option) => {
            void runCreateRuleAndScroll({
                providerUuid: option.provider.uuid,
                model: option.model,
            });
        },
    });

    // Wrapper to maintain compatibility with existing RuleCard interface
    const openModelSelectDialog = useCallback((
        ruleUuid: string,
        configRecord: any,
        mode: 'edit' | 'add',
        providerUuid?: string,
        addTier?: number
    ) => {
        openModelSelect({ruleUuid, configRecord, providerUuid, mode, addTier});
    }, [openModelSelect]);

    // Add-provider flow opened in place (rather than navigating away).
    // Refreshes providers locally on success so the new key shows up
    // without leaving the current scenario. The hook + ConnectAIDialogs own
    // every picker route: form, OAuth, paste & detect, import.
    const connectAI = useProviderDialog(showNotification, {
        onProviderAdded: () => {
            void onProvidersLoad?.();
        },
    });
    const { handleConnectAIClick } = connectAI;

    // Cloud-credential dialog state (Bedrock/Vertex/Azure)
    const [cloudPresetId, setCloudPresetId] = useState<string | null>(null);

    const handleAddApiKeyClick = useCallback(() => {
        handleConnectAIClick();
    }, [handleConnectAIClick]);

    const handleCreateRule = useCallback(() => {
        openModelSelectForCreate();
    }, [openModelSelectForCreate]);

    const handleProviderUpdated = useCallback(async (_providerUuid: string) => {
        await onProvidersLoad?.();
    }, [onProvidersLoad]);

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

    // "Test all rules": bump a signal consumed by each card's QuickProbeButton;
    // every active rule runs its quick streaming probe and shows its own pill.
    const [probeAllSignal, setProbeAllSignal] = useState(0);
    const handleProbeAll = useCallback(() => {
        setProbeAllSignal((s) => s + 1);
    }, []);

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
            onViewLogs={scenario ? () => setLogDialogOpen(true) : undefined}
            onProbeAll={rules.length > 0 ? handleProbeAll : undefined}
            onShowGuide={() => setShowGuide(true)}
            scenario={scenario}
        />
    );

    if (!providers.length) {
        if (!showEmptyState) {
            return null;
        }

        // First-run path: send users to the onboarding flow rather than the
        // bare provider dialog — they get to browse the catalog or paste a
        // config snippet for auto-detection.
        return (
            <UnifiedCard size="full" title={title}>
                <EmptyState
                    title={t('templatePage.noProviders.title')}
                    description={t('templatePage.noProviders.description')}
                    primaryAction={{
                        label: t('templatePage.noProviders.action'),
                        onClick: onAddApiKeyClick || (() => navigate('/onboarding')),
                    }}
                />
            </UnifiedCard>
        );
    }

    return (
        <>
            <UnifiedCard
                id="models-and-forwarding-rules"
                size="full"
                title={title}
                leftAction={
                    <TitleIconButtons
                        collapsible={collapsible}
                        allExpanded={allExpanded}
                        onToggleExpandAll={handleToggleExpandAll}
                        showExpandCollapseButton={showExpandCollapseButton}
                        onShowGuide={() => setShowGuide(true)}
                        sortMode={rules.length > 1 ? sortMode : undefined}
                        onToggleSort={rules.length > 1 ? toggleSortMode : undefined}
                    />
                }
                rightAction={rightAction}
                sx={{ scrollMarginTop: 16 }}
            >
                {/*<Box ref={scrollContainerRef} sx={SCROLLBOX_SX(headerHeight)}>*/}
                <Box ref={scrollContainerRef}>
                    {rules?.length === 0 ? (
                        <Box sx={{
                            textAlign: 'center',
                            py: 8,
                            color: 'text.secondary'
                        }}>
                            {t('templatePage.noRules')}
                        </Box>
                    ) : (
                        sortedRules.map((rule, index) => {
                            const isNewRule = rule.uuid === newRuleUuid;
                            // "Last rule" for the auto-scroll-to-new-rule anchor is
                            // meaningless once the list is name-sorted — that ref is
                            // only a fallback for the initial mount scroll position,
                            // so only attach it in the original, insertion-ordered view.
                            const isLastRule = sortMode === 'original' && index === sortedRules.length - 1;
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
                                            onProviderUpdated={handleProviderUpdated}
                                            collapsible={collapsible}
                                            initiallyExpanded={expandedStates[rule.uuid] ?? collapsible}
                                            onModelSelectOpen={openModelSelectDialog}
                                            allowDeleteRule={allowDeleteRule}
                                            onRuleDelete={onRuleDelete}
                                            allowToggleRule={allowToggleRule}
                                            onToggleExpanded={() => handleRuleExpandToggle(rule.uuid)}
                                            onContext1MToggle={onContext1MToggle}
                                            quickProbeSignal={probeAllSignal}
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
                    await copyToClipboard(text, label);
                    showNotification(t('templatePage.copiedToClipboard', { label }), 'success');
                }}
            />

            <ConnectAIDialogs flow={connectAI}/>

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
            {scenario && (
                <ScenarioLogDialog
                    open={logDialogOpen}
                    onClose={() => setLogDialogOpen(false)}
                    scenario={scenario}
                />
            )}

            <EntryGuideDialog
                open={showGuide}
                onClose={() => setShowGuide(false)}
                mode="direct"
            />
        </>
    );
};

export default TemplatePage;
