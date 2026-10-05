import { ContentCopy as ContentCopyIcon } from '@/components/icons';
import { CheckCircle as CheckCircleIcon } from '@/components/icons';
import { ExpandLess as ExpandLessIcon } from '@/components/icons';
import { ExpandMore as ExpandMoreIcon } from '@/components/icons';
import { HelpOutline as HelpOutlineIcon } from '@/components/icons';
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    Collapse,
    IconButton,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import { isCredentialProvider } from '@/utils/providers';
import { SPOTLIGHT_ADD_MODEL_EVENT } from '@/components/nodes/ActionAddNode';
import { EntryGuideDialog } from '@/components/tier/EntryGuideDialog';
import { useCopyFeedback } from '@/hooks/useCopyFeedback';
import { removeSyncedItem, setSyncedItem } from '@/services/uiPrefs';
import { timeAgo } from '@/utils/timeAgo';
import { fontMono, fontSizes } from '@/theme/fonts';

export interface AgentApplyResult {
    success: boolean;
    files?: string[];
    createdFiles?: string[];
    updatedFiles?: string[];
    backupPaths?: string[];
    error?: string;
}

export interface AgentInstallAction {
    label: string;
    href: string;
    variant?: 'contained' | 'outlined' | 'text';
    external?: boolean;
}

export interface AgentSetupCardProps {
    agentKey: string;
    agentName: string;
    installCommand: string;
    installMirrorCommand?: string;
    installStepDescription?: string;
    installActions?: AgentInstallAction[];
    onApply?: () => Promise<AgentApplyResult>;
    onApplyWithStatusLine?: () => Promise<AgentApplyResult>;
    isApplyLoading?: boolean;
    onViewConfig?: () => void;
    applyStepLabel?: string;
    applyStepDescription?: string;
    applyButtonLabel?: string;
    applySuccessLabel?: string;
    viewConfigButtonLabel?: string;
    hasModelSelected?: boolean;
    onSelectModel?: () => void;
    onConnectProvider?: () => void;
    /** Opened by the header "How routing works" help button. */
    onShowGuide?: () => void;
}

const COLLAPSED_KEY = (agentKey: string) => `setup-card-collapsed-${agentKey}`;
const INSTALL_DONE_KEY = (agentKey: string) => `setup-card-step2-done-${agentKey}`;
const APPLY_DONE_KEY = (agentKey: string) => `setup-card-step3-done-${agentKey}`;
// Step 2 (model) can be skipped, mirroring Step 4's skip. Skipping marks the
// step done so the wizard advances, without claiming a model was configured.
const MODEL_SKIPPED_KEY = (agentKey: string) => `setup-card-step2-skipped-${agentKey}`;
const TOTAL_STEPS = 4;

/** True iff at least one rule has a service with both a non-empty provider and model. */
export const hasModelOnAnyRule = (rules: any[] | null | undefined): boolean =>
    Array.isArray(rules) &&
    rules.some(r => Array.isArray(r?.services) && r.services.some((s: any) => s?.provider && s?.model));

/**
 * Smoothly scroll the "Model Rules" card into view, then
 * spotlight the "+ Add model" target so "Select a Model" actually points the
 * user at where to click — not just near it. The pulse is fired after the
 * scroll settles so it lands in view.
 */
export const scrollToModelsCard = () => {
    document.getElementById('models-and-forwarding-rules')?.scrollIntoView({
        behavior: 'smooth',
        block: 'start',
    });
    window.setTimeout(() => {
        window.dispatchEvent(new CustomEvent(SPOTLIGHT_ADD_MODEL_EVENT));
    }, 450);
};

const StepIndicator: React.FC<{ step: number; done: boolean; active: boolean }> = ({ step, done, active }) => (
    done ? (
        <CheckCircleIcon sx={{ fontSize: 22, color: 'success.main', flexShrink: 0 }} />
    ) : (
        <Box sx={{
            width: 22, height: 22, borderRadius: '50%', flexShrink: 0,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            bgcolor: active ? 'primary.main' : 'action.disabledBackground',
            color: active ? 'primary.contrastText' : 'text.disabled',
            fontSize: fontSizes.xs, fontWeight: 700,
        }}>
            {step}
        </Box>
    )
);

const AgentSetupCard: React.FC<AgentSetupCardProps> = ({
    agentKey,
    agentName,
    installCommand,
    installMirrorCommand,
    installStepDescription,
    installActions,
    onApply,
    onApplyWithStatusLine,
    isApplyLoading = false,
    onViewConfig,
    applyStepLabel: applyStepLabelProp,
    applyStepDescription,
    applyButtonLabel: applyButtonLabelProp,
    applySuccessLabel: applySuccessLabelProp,
    viewConfigButtonLabel: viewConfigButtonLabelProp,
    hasModelSelected = false,
    onSelectModel,
    onConnectProvider,
    onShowGuide,
}) => {
    const { t } = useTranslation();
    // Pages may override these; otherwise fall back to the translated defaults.
    const applyStepLabel = applyStepLabelProp ?? t('agentSetup.apply.label');
    const applyButtonLabel = applyButtonLabelProp ?? t('agentSetup.apply.button');
    const applySuccessLabel = applySuccessLabelProp ?? t('agentSetup.apply.success');
    const viewConfigButtonLabel = viewConfigButtonLabelProp ?? t('agentSetup.apply.viewConfig');
    const initialCollapsedPref = useRef<string | null>(localStorage.getItem(COLLAPSED_KEY(agentKey)));
    const [collapsed, setCollapsed] = useState(initialCollapsedPref.current === 'true');
    const [installConfirmed, setInstallConfirmed] = useState(
        () => localStorage.getItem(INSTALL_DONE_KEY(agentKey)) === 'true'
    );
    const [applyDone, setApplyDone] = useState(
        () => localStorage.getItem(APPLY_DONE_KEY(agentKey)) === 'true'
    );
    const [modelSkipped, setModelSkipped] = useState(
        () => localStorage.getItem(MODEL_SKIPPED_KEY(agentKey)) === 'true'
    );
    // The real signal for "installed": this agent has already sent a request
    // through the gateway. The manual "I've installed it" stays as the way to
    // move on before that first request.
    const [lastRequestAt, setLastRequestAt] = useState<string | null>(null);
    useEffect(() => {
        let cancelled = false;
        api.getUsageRecords({ scenario: agentKey, limit: 1 }).then((res) => {
            const first = Array.isArray(res?.data) ? res.data[0] : undefined;
            if (!cancelled && first?.timestamp) setLastRequestAt(first.timestamp);
        }).catch(() => { /* no signal: fall back to the manual confirm */ });
        return () => { cancelled = true; };
    }, [agentKey]);
    const installDone = installConfirmed || lastRequestAt !== null;
    const [hasProvider, setHasProvider] = useState(false);
    const [providerCount, setProviderCount] = useState(0);
    const [providerLoading, setProviderLoading] = useState(true);
    const [applyResult, setApplyResult] = useState<AgentApplyResult | null>(null);
    const { copied, copy: copyInstallCommand, reset: resetCopied } = useCopyFeedback(1500);
    const { copied: copiedMirror, copy: copyInstallMirrorCommand, reset: resetCopiedMirror } = useCopyFeedback(1500);
    const [showGuide, setShowGuide] = useState(false);
    // Tracks which completed steps the user has manually expanded
    const [expandedDoneSteps, setExpandedDoneSteps] = useState<Set<number>>(new Set());

    // Progress is shared with the other UI surface (services/uiPrefs); pick
    // up values the post-sign-in sync pulls in while this card is mounted.
    useEffect(() => {
        const onStorage = (e: StorageEvent) => {
            if (e.key === INSTALL_DONE_KEY(agentKey)) setInstallConfirmed(localStorage.getItem(e.key) === 'true');
            else if (e.key === APPLY_DONE_KEY(agentKey)) setApplyDone(localStorage.getItem(e.key) === 'true');
            else if (e.key === MODEL_SKIPPED_KEY(agentKey)) setModelSkipped(localStorage.getItem(e.key) === 'true');
        };
        window.addEventListener('storage', onStorage);
        return () => window.removeEventListener('storage', onStorage);
    }, [agentKey]);

    const toggleDoneStep = (step: number) => {
        setExpandedDoneSteps(prev => {
            const next = new Set(prev);
            if (next.has(step)) next.delete(step);
            else next.add(step);
            return next;
        });
    };

    useEffect(() => {
        let cancelled = false;
        api.getProviders().then((result) => {
            if (cancelled) return;
            const providers = Array.isArray(result?.data) ? result.data : [];
            const enabled = providers.filter((p: any) => p.enabled && isCredentialProvider(p));
            setHasProvider(enabled.length > 0);
            setProviderCount(enabled.length);
            setProviderLoading(false);
        }).catch(() => {
            if (!cancelled) setProviderLoading(false);
        });
        return () => { cancelled = true; };
    }, []);

    const providerDone = hasProvider;
    const modelSelected = hasModelSelected;
    // The step counts as done either when a model is really configured (the
    // provider actually routes a model on some rule) or when the user chose to
    // skip it. "Configured" vs "skipped" stays distinct on the row itself.
    const modelDone = modelSelected || modelSkipped;
    const allDone = providerDone && modelDone && installDone && applyDone;
    const doneCount = [providerDone, modelDone, installDone, applyDone].filter(Boolean).length;

    const autoCollapsedRef = useRef(false);
    useEffect(() => {
        if (autoCollapsedRef.current) return;
        if (providerLoading) return;
        if (initialCollapsedPref.current !== null) return;
        if (allDone) {
            autoCollapsedRef.current = true;
            setCollapsed(true);
            setSyncedItem(COLLAPSED_KEY(agentKey), 'true');
        }
    }, [providerLoading, allDone, agentKey]);

    const toggleCollapsed = () => {
        const next = !collapsed;
        setSyncedItem(COLLAPSED_KEY(agentKey), String(next));
        setCollapsed(next);
    };

    const handleCopy = () => {
        copyInstallCommand(installCommand);
    };

    const handleCopyMirror = () => {
        if (!installMirrorCommand) return;
        copyInstallMirrorCommand(installMirrorCommand);
    };

    const markInstallDone = () => {
        setSyncedItem(INSTALL_DONE_KEY(agentKey), 'true');
        setInstallConfirmed(true);
    };

    const markModelSkipped = () => {
        setSyncedItem(MODEL_SKIPPED_KEY(agentKey), 'true');
        setModelSkipped(true);
    };

    // Re-entry after a skip: opening the model picker drops the skipped mark
    // (progress still stands on the already-selected model / provider), so the
    // row flips back to the real "configured" state as soon as a model is set.
    const handleChooseModel = () => {
        if (modelSkipped) {
            removeSyncedItem(MODEL_SKIPPED_KEY(agentKey));
            setModelSkipped(false);
        }
        onSelectModel?.();
    };

    const handleApplyWithStatusLine = async () => {
        // onApplyWithStatusLine is only provided by callers with an extra
        // install-time toggle (e.g. Claude Code's status line); everyone else
        // wires the plain onApply and expects the button to invoke it.
        const apply = onApplyWithStatusLine ?? onApply;
        if (!apply) return;
        const result = await apply();
        setApplyResult(result);
        if (result.success) {
            setSyncedItem(APPLY_DONE_KEY(agentKey), 'true');
            setApplyDone(true);
        }
    };

    const handleReset = () => {
        removeSyncedItem(COLLAPSED_KEY(agentKey));
        removeSyncedItem(INSTALL_DONE_KEY(agentKey));
        removeSyncedItem(APPLY_DONE_KEY(agentKey));
        removeSyncedItem(MODEL_SKIPPED_KEY(agentKey));
        setCollapsed(false);
        setInstallConfirmed(false);
        setApplyDone(false);
        setModelSkipped(false);
        setApplyResult(null);
        resetCopied();
        resetCopiedMirror();
    };

    const progressLabel = allDone ? t('agentSetup.done') : `${doneCount}/${TOTAL_STEPS}`;
    const progressColor = allDone ? 'success' : 'default';

    const collapsedHint = !providerDone
        ? t('agentSetup.hint.connectProvider')
        : !modelDone
            ? t('agentSetup.hint.selectModel')
            : !installDone
                ? t('agentSetup.hint.install', { agent: agentName })
                : t('agentSetup.hint.apply', { action: applyStepLabel });

    // Which step is the first incomplete one (determines which expands)
    const firstIncomplete = !providerDone ? 0 : !modelDone ? 1 : !installDone ? 2 : !applyDone ? 3 : -1;

    const stepRowSx = () => ({
        py: 0.75,
        px: 1.5,
        borderRadius: 1.5,
    });

    return (
        <>
        <UnifiedCard
            size="header"
            titleMarginBottom={collapsed ? 0 : 2}
            title={
                <Stack
                    direction="row"
                    spacing={1}
                    sx={{
                        alignItems: "center",
                        flex: 1
                    }}>
                    <Typography variant="subtitle1" sx={{
                        fontWeight: 600
                    }}>{t('agentSetup.quickStart')}</Typography>
                    <Chip
                        label={progressLabel}
                        size="small"
                        color={progressColor as any}
                        sx={{ height: 20, fontSize: fontSizes.sm }}
                    />
                    {collapsed && !allDone && (
                        <Typography
                            variant="body2"
                            sx={{
                                color: "text.secondary",
                                ml: 0.5
                            }}>
                            {collapsedHint}
                        </Typography>
                    )}
                </Stack>
            }
            rightAction={
                <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
                    {/* Reset is always reachable from the header (not buried at the
                        bottom of an expanded card), so it works whether the card is
                        open, closed, or mid-flow. */}
                    <Button
                        size="small"
                        variant="text"
                        onClick={handleReset}
                        sx={{ py: 0, textTransform: 'none', color: 'text.secondary', minWidth: 0, fontSize: fontSizes.sm }}
                    >
                        {t('agentSetup.resetProgress')}
                    </Button>
                    <Tooltip title={t('templateActions.howRoutingWorks', { defaultValue: 'How routing works' })}>
                        <IconButton
                            size="small"
                            aria-label={t('templateActions.howRoutingWorks', { defaultValue: 'How routing works' })}
                            onClick={() => { if (onShowGuide) onShowGuide(); else setShowGuide(true); }}
                            sx={{ color: 'text.secondary', '&:hover': { color: 'primary.main' } }}
                        >
                            <HelpOutlineIcon fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <Tooltip title={collapsed ? t('agentSetup.expand') : t('agentSetup.collapse')}>
                        <IconButton size="small" onClick={toggleCollapsed}>
                            {collapsed ? <ExpandMoreIcon fontSize="small" /> : <ExpandLessIcon fontSize="small" />}
                        </IconButton>
                    </Tooltip>
                </Stack>
            }
        >
            <Collapse in={!collapsed} unmountOnExit={false}>
                <Stack spacing={0.5}>

                    {/* Step 1 — Provider (always a single flat row — no expand needed) */}
                    <Box sx={stepRowSx()}>
                        <Stack
                            direction="row"
                            spacing={1.25}
                            sx={{ alignItems: "center", flexWrap: 'wrap', rowGap: 0.5 }}>
                            {providerLoading ? <CircularProgress size={20} sx={{ flexShrink: 0 }} /> : <StepIndicator step={1} done={providerDone} active={firstIncomplete === 0} />}
                            <Typography
                                variant="body2"
                                color={providerDone ? 'text.primary' : firstIncomplete === 0 ? 'primary.main' : 'text.disabled'}
                                sx={{
                                    fontWeight: 500,
                                    flex: 1
                                }}>
                                {t('agentSetup.provider.label')}
                            </Typography>
                            {providerDone && (
                                <Typography variant="body2" sx={{
                                    color: "text.secondary"
                                }}>
                                    {providerCount === 1
                                        ? t('agentSetup.provider.countOne')
                                        : t('agentSetup.provider.count', { count: providerCount })}
                                </Typography>
                            )}
                            {onConnectProvider && (
                                <Tooltip title={providerDone ? '' : t('agentSetup.provider.tooltip', { agent: agentName })}>
                                    <Button
                                        size="small"
                                        variant={providerDone ? 'text' : 'contained'}
                                        onClick={onConnectProvider}
                                        sx={providerDone ? { py: 0, textTransform: 'none', minWidth: 0 } : { py: 0.25 }}
                                    >
                                        {providerDone ? t('agentSetup.provider.addMore') : t('agentSetup.provider.connect')}
                                    </Button>
                                </Tooltip>
                            )}
                        </Stack>
                    </Box>

                    {/* Step 2 — Model (always a single flat row — no expand needed) */}
                    <Box sx={stepRowSx()}>
                        <Stack
                            direction="row"
                            spacing={1.25}
                            sx={{ alignItems: "center", flexWrap: 'wrap', rowGap: 0.5 }}>
                            <StepIndicator step={2} done={modelDone} active={firstIncomplete === 1} />
                            <Typography
                                variant="body2"
                                color={modelDone ? 'text.primary' : firstIncomplete === 1 ? 'primary.main' : 'text.disabled'}
                                sx={{
                                    fontWeight: 500,
                                    flex: 1
                                }}>
                                {t('agentSetup.model.label')}
                            </Typography>
                            {modelSelected && (
                                <Typography variant="body2" sx={{
                                    color: "text.secondary"
                                }}>{t('agentSetup.model.configured')}</Typography>
                            )}
                            {modelSkipped && !modelSelected && (
                                <Typography variant="body2" sx={{
                                    color: "text.secondary"
                                }}>{t('agentSetup.model.skipped')}</Typography>
                            )}
                            {onSelectModel && (
                                <Tooltip title={modelDone ? '' : t('agentSetup.model.tooltip', { agent: agentName })}>
                                    <span>
                                        <Button
                                            size="small"
                                            variant={modelDone ? 'text' : 'contained'}
                                            disabled={!modelDone && !providerDone}
                                            onClick={handleChooseModel}
                                            sx={modelDone ? { py: 0, textTransform: 'none', minWidth: 0 } : { py: 0.25 }}
                                        >
                                            {modelSelected ? t('agentSetup.model.change') : t('agentSetup.model.choose')}
                                        </Button>
                                    </span>
                                </Tooltip>
                            )}
                            {firstIncomplete === 1 && (
                                <Button variant="text" size="small"
                                    onClick={(e) => { e.stopPropagation(); markModelSkipped(); }}
                                    sx={{ py: 0, textTransform: 'none', color: 'text.disabled', minWidth: 0 }}>
                                    {t('agentSetup.model.skip')}
                                </Button>
                            )}
                        </Stack>
                    </Box>

                    {/* Step 3 — Install */}
                    <Box sx={stepRowSx()}>
                        <Stack
                            direction="row"
                            spacing={1.25}
                            onClick={installDone ? () => toggleDoneStep(2) : undefined}
                            sx={[{
                                alignItems: "center",
                                flexWrap: 'wrap',
                                rowGap: 0.5
                            }, installDone ? { cursor: 'pointer', '&:hover': { opacity: 0.8 } } : false]}>
                            <StepIndicator step={3} done={installDone} active={firstIncomplete === 2} />
                            <Typography
                                variant="body2"
                                color={installDone ? 'text.primary' : firstIncomplete === 2 ? 'primary.main' : 'text.disabled'}
                                sx={{
                                    fontWeight: 500,
                                    flex: 1
                                }}>
                                {t('agentSetup.install.label', { agent: agentName })}
                            </Typography>
                            {installDone && (
                                <Typography variant="body2" sx={{
                                    color: "text.secondary"
                                }}>
                                    {lastRequestAt
                                        ? t('agentSetup.install.detected', { time: timeAgo(lastRequestAt) })
                                        : t('agentSetup.install.installed')}
                                </Typography>
                            )}
                            {/* Step-completing action lives in the row's right action
                                column, same as steps 1 / 2 / 4. */}
                            {!installDone && firstIncomplete === 2 && (
                                <Tooltip title={t('agentSetup.install.confirmTooltip', { agent: agentName })}>
                                    <Button
                                        variant="contained"
                                        size="small"
                                        onClick={(e) => { e.stopPropagation(); markInstallDone(); }}
                                        sx={{ py: 0.25 }}
                                    >
                                        {t('agentSetup.install.confirm')}
                                    </Button>
                                </Tooltip>
                            )}
                            {installDone && (
                                expandedDoneSteps.has(2) ? <ExpandLessIcon fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} /> : <ExpandMoreIcon fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} />
                            )}
                        </Stack>
                        <Collapse in={(!installDone && firstIncomplete === 2) || expandedDoneSteps.has(2)}>
                            <Stack spacing={0.75} sx={{ mt: 0.75, pl: 4.25 }}>
                                {installActions?.length ? (
                                    <>
                                        {installStepDescription && (
                                            <Typography variant="body2" sx={{
                                                color: "text.secondary"
                                            }}>{installStepDescription}</Typography>
                                        )}
                                        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ maxWidth: 520 }}>
                                            {installActions.map((action) => (
                                                <Button
                                                    key={`${action.label}-${action.href}`}
                                                    href={action.href}
                                                    target={action.external ? '_blank' : undefined}
                                                    rel={action.external ? 'noopener noreferrer' : undefined}
                                                    variant={action.variant ?? 'outlined'}
                                                    size="small"
                                                    sx={{ flex: 1 }}
                                                >
                                                    {action.label}
                                                </Button>
                                            ))}
                                        </Stack>
                                    </>
                                ) : (
                                    <>
                                        <Typography variant="body2" sx={{
                                            color: "text.secondary"
                                        }}>
                                            {installStepDescription || t('agentSetup.install.description', { agent: agentName })}
                                        </Typography>
                                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, maxWidth: 800 }}>
                                            <Typography
                                                variant="body2"
                                                sx={{
                                                    color: "text.secondary",
                                                    minWidth: '80px'
                                                }}>{t('agentSetup.install.npmOfficial')}</Typography>
                                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flex: 1, minWidth: 0 }}>
                                                <Tooltip title={copied ? t('agentSetup.install.copied') : t('agentSetup.install.copy')}>
                                                    <IconButton size="small" onClick={handleCopy} sx={{ flexShrink: 0, p: 0.25 }}><ContentCopyIcon sx={{ fontSize: 16 }} /></IconButton>
                                                </Tooltip>
                                                <Typography variant="body2" onClick={handleCopy} sx={{ fontFamily: fontMono, flex: 1, color: 'text.primary', cursor: 'pointer', '&:hover': { color: 'primary.main' }, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={installCommand}>{installCommand}</Typography>
                                            </Box>
                                        </Box>
                                        {installMirrorCommand && (
                                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, maxWidth: 800 }}>
                                                <Typography
                                                    variant="body2"
                                                    sx={{
                                                        color: "text.secondary",
                                                        minWidth: '80px'
                                                    }}>{t('agentSetup.install.npmMirror')}</Typography>
                                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flex: 1, minWidth: 0 }}>
                                                    <Tooltip title={copiedMirror ? t('agentSetup.install.copied') : t('agentSetup.install.copy')}>
                                                        <IconButton size="small" onClick={handleCopyMirror} sx={{ flexShrink: 0, p: 0.25 }}><ContentCopyIcon sx={{ fontSize: 16 }} /></IconButton>
                                                    </Tooltip>
                                                    <Typography variant="body2" onClick={handleCopyMirror} sx={{ fontFamily: fontMono, flex: 1, color: 'text.primary', cursor: 'pointer', '&:hover': { color: 'primary.main' }, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={installMirrorCommand}>{installMirrorCommand}</Typography>
                                                </Box>
                                            </Box>
                                        )}
                                    </>
                                )}
                            </Stack>
                        </Collapse>
                    </Box>

                    {/* Step 4 — Apply (flat row while active; re-expandable once done for re-view) */}
                    <Box sx={stepRowSx()}>
                        <Stack
                            direction="row"
                            spacing={1.25}
                            onClick={applyDone ? () => toggleDoneStep(3) : undefined}
                            sx={[{
                                alignItems: "center",
                                flexWrap: 'wrap',
                                rowGap: 0.5
                            }, applyDone ? { cursor: 'pointer', '&:hover': { opacity: 0.8 } } : false]}>
                            <StepIndicator step={4} done={applyDone} active={firstIncomplete === 3} />
                            <Typography
                                variant="body2"
                                color={applyDone ? 'text.primary' : firstIncomplete === 3 ? 'primary.main' : 'text.disabled'}
                                sx={{
                                    fontWeight: 500,
                                    flex: 1
                                }}>
                                {applyStepLabel}
                            </Typography>
                            {applyDone && (
                                <Typography variant="body2" sx={{
                                    color: "text.secondary"
                                }}>{t('agentSetup.apply.applied')}</Typography>
                            )}
                            {!applyDone && firstIncomplete === 3 && (
                                <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', flexWrap: 'wrap', rowGap: 0.5 }}>
                                    {onApply && (
                                        <Tooltip title={applyStepDescription ?? t('agentSetup.apply.tooltip', { agent: agentName })}>
                                            <span>
                                                <Button variant="contained" size="small" disabled={isApplyLoading} onClick={(e) => { e.stopPropagation(); handleApplyWithStatusLine(); }} startIcon={isApplyLoading ? <CircularProgress size={14} color="inherit" /> : undefined} sx={{ py: 0.25 }}>
                                                    {applyButtonLabel}
                                                </Button>
                                            </span>
                                        </Tooltip>
                                    )}
                                    {onViewConfig && (
                                        <Button variant="text" size="small" onClick={(e) => { e.stopPropagation(); onViewConfig(); }} sx={{ py: 0, textTransform: 'none', color: 'text.secondary', minWidth: 0 }}>
                                            {viewConfigButtonLabel}
                                        </Button>
                                    )}
                                    <Button variant="text" size="small" onClick={(e) => {
                                        e.stopPropagation();
                                        setSyncedItem(APPLY_DONE_KEY(agentKey), 'true');
                                        setApplyDone(true);
                                    }} sx={{ py: 0, textTransform: 'none', color: 'text.disabled', minWidth: 0 }}>
                                        {t('agentSetup.apply.skip')}
                                    </Button>
                                </Stack>
                            )}
                            {applyDone && (
                                expandedDoneSteps.has(3) ? <ExpandLessIcon fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} /> : <ExpandMoreIcon fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} />
                            )}
                        </Stack>
                        {applyResult && (
                            <Alert severity={applyResult.success ? 'success' : 'error'} sx={{ mt: 0.75, ml: 4.25, py: 0.5 }}>
                                {applyResult.success ? (
                                    <Box>
                                        <Typography variant="body2" sx={{
                                            fontWeight: 600
                                        }}>{applySuccessLabel}</Typography>
                                        {applyResult.files?.map(f => (
                                            <Typography key={f} variant="body2" sx={{ display: 'block', fontFamily: fontMono, color: 'text.secondary' }}>{f}</Typography>
                                        ))}
                                    </Box>
                                ) : (
                                    <Typography variant="body2">{applyResult.error ?? t('agentSetup.apply.failed')}</Typography>
                                )}
                            </Alert>
                        )}
                        <Collapse in={applyDone && expandedDoneSteps.has(3)}>
                            <Stack spacing={0.75} sx={{ mt: 0.75, pl: 4.25 }}>
                                <Typography variant="body2" sx={{
                                    color: "text.secondary"
                                }}>
                                    {applyStepDescription ?? t('agentSetup.apply.tooltip', { agent: agentName })}
                                </Typography>
                                <Stack
                                    direction="row"
                                    spacing={1}
                                    sx={{
                                        flexWrap: "wrap",
                                        gap: 0.5
                                    }}>
                                    {onApply && (
                                        <Button variant="contained" size="small" disabled={isApplyLoading} onClick={handleApplyWithStatusLine} startIcon={isApplyLoading ? <CircularProgress size={14} color="inherit" /> : undefined}>
                                            {applyButtonLabel}
                                        </Button>
                                    )}
                                    {onViewConfig && (
                                        <Button variant="text" size="small" onClick={onViewConfig} sx={{ textTransform: 'none', color: 'text.secondary' }}>
                                            {t('agentSetup.apply.viewConfigAdvanced', { label: viewConfigButtonLabel })}
                                        </Button>
                                    )}
                                </Stack>
                            </Stack>
                        </Collapse>
                    </Box>
                </Stack>
            </Collapse>
        </UnifiedCard>

        {/* Self-hosted "How routing works" guide — only when the page didn't
            hand in its own `onShowGuide` (in which case the page owns the
            dialog). */}
        {!onShowGuide && (
            <EntryGuideDialog
                open={showGuide}
                onClose={() => setShowGuide(false)}
                mode="direct"
            />
        )}
        </>
    );
};

export default AgentSetupCard;
