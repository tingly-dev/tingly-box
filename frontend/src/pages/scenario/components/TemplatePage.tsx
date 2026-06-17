import ApiKeyModal from '@/components/ApiKeyModal';
import ScenarioLogDialog from '@/components/RuleLogDialog';
import React, {useCallback, useEffect, useRef, useState} from 'react';
import {Alert, Box, Fab, Snackbar} from '@mui/material';
import { KeyboardArrowUp as KeyboardArrowUpIcon } from '@/components/icons';
import {useNavigate} from 'react-router-dom';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import RuleCard from '@/components/RuleCard.tsx';
import ImportModal from '@/components/ImportModal';
import ProviderFormDialog from '@/components/ProviderFormDialog';
import ConnectProviderDialog from '@/components/ConnectProviderDialog';
import UnifiedCard from '@/components/UnifiedCard';
import { EntryGuideDialog } from '@/components/tier/EntryGuideDialog';
import type {TemplatePageProps} from './TemplatePage.types';
import {TemplatePageActions} from './TemplatePageActions';
import {TitleIconButtons} from './TitleIconButtons';
import {useTemplatePageRules} from '@/pages/scenario/hooks/useTemplatePageRules';
import {useScrollToNewRule} from '@/components/hooks/useScrollToNewRule';
import {useModelSelectDialog} from '@/hooks/useModelSelectDialog';
import {useProviderDialog} from '@/hooks/useProviderDialog';
import {useScenarioPageInternal} from '@/pages/scenario/hooks/useScenarioPageInternal';
import {useScenarioPageModal} from '@/pages/scenario/context/ScenarioPageContext';
import api from '@/services/api';

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
 * <TemplatePage scenario="agent" />
 *
 * HYBRID MODE (for custom logic):
 * Provide `scenario` plus override specific data props for custom behavior.
 * <TemplatePage scenario="custom" rules={customRules} onRulesChange={customHandler} />
 *
 * Modal state (ApiKeyModal) is shared via ScenarioPageModalProvider context.
 */
const TemplatePage: React.FC<TemplatePageProps> = (props) => {
    // Get modal state from context (shared with ProviderConfigCard)
    const { showTokenModal, setShowTokenModal, token, copyToClipboard } = useScenarioPageModal();

    // Internal mode: fetch all data internally (excluding modal - that's from context)
    const internalData = useScenarioPageInternal(props.scenario);

    const {
        title = "Model Rules",
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
    const [showImportModal, setShowImportModal] = useState<boolean>(false);
    const [logDialogOpen, setLogDialogOpen] = useState<boolean>(false);
    const [importing, setImporting] = useState<boolean>(false);
    const [importError, setImportError] = useState<{ open: boolean; message: string }>({open: false, message: ''});
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

    // Add-provider dialog opened in place (rather than navigating away).
    // Refreshes providers locally on success so the new key shows up
    // without leaving the current scenario.
    const {
        providerDialogOpen,
        providerFormData,
        handleProviderSubmit,
        handleProviderForceAdd,
        handleCloseDialog,
        handleFieldChange,
        connectDialogOpen,
        handleConnectAIClick,
        handleConnectSelect,
        handleCloseConnect,
        customMode,
        dualMode,
        fromConnectPicker,
    } = useProviderDialog(showNotification, {
        onProviderAdded: () => {
            void onProvidersLoad?.();
        },
        onImport: () => setShowImportModal(true),
    });

    const handleAddApiKeyClick = useCallback(() => {
        handleConnectAIClick();
    }, [handleConnectAIClick]);

    const handleCreateRule = useCallback(() => {
        openModelSelectForCreate();
    }, [openModelSelectForCreate]);

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
            onViewLogs={scenario ? () => setLogDialogOpen(true) : undefined}
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
                <EmptyStateGuide
                    title={"No Providers Configured"}
                    description={"Add an API key provider to start routing requests"}
                    primaryButtonLabel={"Get started"}
                    showOAuthButton={false}
                    onAddApiKeyClick={onAddApiKeyClick || (() => navigate('/onboarding'))}
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
                                            onContext1MToggle={onContext1MToggle}
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
                    showNotification(`${label} copied to clipboard!`, 'success');
                }}
            />

            <ImportModal
                open={showImportModal}
                onClose={() => setShowImportModal(false)}
                onImport={handleImportData}
                loading={importing}
            />

            <ConnectProviderDialog
                open={connectDialogOpen}
                onClose={handleCloseConnect}
                onSelect={handleConnectSelect}
            />

            <ProviderFormDialog
                open={providerDialogOpen}
                onClose={handleCloseDialog}
                onBack={fromConnectPicker ? () => { handleCloseDialog(); handleConnectAIClick(); } : undefined}
                onSubmit={handleProviderSubmit}
                onForceAdd={handleProviderForceAdd}
                data={providerFormData}
                onChange={handleFieldChange}
                mode="add"
                customMode={customMode}
                dualMode={dualMode}
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
