import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    Collapse,
    IconButton,
    Stack,
    Step,
    StepLabel,
    Stepper,
    Tooltip,
    Typography,
} from '@mui/material';
import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';

export interface AgentApplyResult {
    success: boolean;
    files?: string[];
    // Optional structured breakdown for callers that want richer feedback
    // (e.g. the Claude Code modal). Existing consumers can keep using
    // `files` alone — these are additive and may be undefined.
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
    /** Whether at least one rule has a service with both a provider and model set. */
    hasModelSelected?: boolean;
    /** Triggered when the user clicks the Step 4 CTA — usually scrolls the page to the rules card. */
    onSelectModel?: () => void;
}

const COLLAPSED_KEY = (agentKey: string) => `setup-card-collapsed-${agentKey}`;
// Storage key names retain their `step2`/`step3` suffixes for backward compat
// even though those steps moved to positions 3 and 4 in the UI.
const INSTALL_DONE_KEY = (agentKey: string) => `setup-card-step2-done-${agentKey}`;
const APPLY_DONE_KEY = (agentKey: string) => `setup-card-step3-done-${agentKey}`;
const TOTAL_STEPS = 4;
const STEP_LABELS = ['Provider', 'Model', 'Install', 'Apply'] as const;

/** True iff at least one rule has a service with both a non-empty provider and model. */
export const hasModelOnAnyRule = (rules: any[] | null | undefined): boolean =>
    Array.isArray(rules) &&
    rules.some(r => Array.isArray(r?.services) && r.services.some((s: any) => s?.provider && s?.model));

/** Smoothly scroll the "Models and Forwarding Rules" card into view. */
export const scrollToModelsCard = () => {
    document.getElementById('models-and-forwarding-rules')?.scrollIntoView({
        behavior: 'smooth',
        block: 'start',
    });
};

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
    applyStepLabel = 'Apply Config',
    applyStepDescription,
    applyButtonLabel = 'Apply',
    applySuccessLabel = 'Config applied!',
    viewConfigButtonLabel = 'Manual Setup',
    hasModelSelected = false,
    onSelectModel,
}) => {
    // We read the stored collapsed preference once on mount so the auto-collapse
    // effect below can distinguish "user has not chosen" from "user chose expanded".
    const initialCollapsedPref = useRef<string | null>(localStorage.getItem(COLLAPSED_KEY(agentKey)));
    const [collapsed, setCollapsed] = useState(initialCollapsedPref.current === 'true');
    const [installDone, setInstallDone] = useState(
        () => localStorage.getItem(INSTALL_DONE_KEY(agentKey)) === 'true'
    );
    const [applyDone, setApplyDone] = useState(
        () => localStorage.getItem(APPLY_DONE_KEY(agentKey)) === 'true'
    );
    const [hasProvider, setHasProvider] = useState(false);
    const [providerCount, setProviderCount] = useState(0);
    const [providerLoading, setProviderLoading] = useState(true);
    const [applyResult, setApplyResult] = useState<AgentApplyResult | null>(null);
    const [copied, setCopied] = useState(false);
    const [copiedMirror, setCopiedMirror] = useState(false);

    useEffect(() => {
        let cancelled = false;
        api.getProviders().then((result) => {
            if (cancelled) return;
            const providers = Array.isArray(result?.data) ? result.data : [];
            const enabled = providers.filter((p: any) => p.enabled);
            setHasProvider(enabled.length > 0);
            setProviderCount(enabled.length);
            setProviderLoading(false);
        }).catch(() => {
            if (!cancelled) setProviderLoading(false);
        });
        return () => { cancelled = true; };
    }, []);

    // Step order: 1) Provider, 2) Model, 3) Install, 4) Apply
    const providerDone = hasProvider;
    const modelDone = hasModelSelected;
    const allDone = providerDone && modelDone && installDone && applyDone;
    const doneCount = [providerDone, modelDone, installDone, applyDone].filter(Boolean).length;
    const activeStep = !providerDone ? 0 : !modelDone ? 1 : !installDone ? 2 : !applyDone ? 3 : 3;
    const [stepCursor, setStepCursor] = useState(activeStep);
    const [skipped, setSkipped] = useState<Set<number>>(new Set());

    useEffect(() => {
        setStepCursor(prev => (prev < activeStep ? activeStep : prev));
    }, [activeStep]);

    const isStepSkipped = (step: number) => skipped.has(step);
    const handleNext = () => {
        const nextSkipped = new Set(skipped);
        nextSkipped.delete(stepCursor);
        setSkipped(nextSkipped);
        if (stepCursor === 2) {
            localStorage.setItem(INSTALL_DONE_KEY(agentKey), 'true');
            setInstallDone(true);
        }
        if (stepCursor >= TOTAL_STEPS - 1) {
            localStorage.setItem(APPLY_DONE_KEY(agentKey), 'true');
            setApplyDone(true);
            setCollapsed(true);
            localStorage.setItem(COLLAPSED_KEY(agentKey), 'true');
            return;
        }
        setStepCursor(prev => Math.min(prev + 1, TOTAL_STEPS - 1));
    };
    const handleBack = () => setStepCursor(prev => Math.max(prev - 1, 0));
    const handleSkip = () => {
        const nextSkipped = new Set(skipped);
        nextSkipped.add(stepCursor);
        setSkipped(nextSkipped);
        if (stepCursor === 2) {
            localStorage.setItem(INSTALL_DONE_KEY(agentKey), 'true');
            setInstallDone(true);
        }
        if (stepCursor >= TOTAL_STEPS - 1) {
            localStorage.setItem(APPLY_DONE_KEY(agentKey), 'true');
            setApplyDone(true);
            setCollapsed(true);
            localStorage.setItem(COLLAPSED_KEY(agentKey), 'true');
            return;
        }
        setStepCursor(prev => Math.min(prev + 1, TOTAL_STEPS - 1));
    };
    const handleReset = () => {
        localStorage.removeItem(COLLAPSED_KEY(agentKey));
        localStorage.removeItem(INSTALL_DONE_KEY(agentKey));
        localStorage.removeItem(APPLY_DONE_KEY(agentKey));
        setCollapsed(false);
        setInstallDone(false);
        setApplyDone(false);
        setApplyResult(null);
        setCopied(false);
        setCopiedMirror(false);
        setSkipped(new Set());
        setStepCursor(!providerDone ? 0 : !modelDone ? 1 : 2);
    };

    // Auto-collapse on first visit when every step is already complete, but only
    // when the user hasn't expressed a preference. We wait for providerLoading
    // so step1Done has settled before we decide.
    const autoCollapsedRef = useRef(false);
    useEffect(() => {
        if (autoCollapsedRef.current) return;
        if (providerLoading) return;
        if (initialCollapsedPref.current !== null) return;
        if (allDone) {
            autoCollapsedRef.current = true;
            setCollapsed(true);
            localStorage.setItem(COLLAPSED_KEY(agentKey), 'true');
        }
    }, [providerLoading, allDone, agentKey]);

    const toggleCollapsed = () => {
        const next = !collapsed;
        localStorage.setItem(COLLAPSED_KEY(agentKey), String(next));
        setCollapsed(next);
    };

    const handleApplyDone = () => {
        localStorage.setItem(APPLY_DONE_KEY(agentKey), 'true');
        setApplyDone(true);
    };

    const handleCopy = async () => {
        await navigator.clipboard.writeText(installCommand);
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
    };

    const handleCopyMirror = async () => {
        if (!installMirrorCommand) return;
        await navigator.clipboard.writeText(installMirrorCommand);
        setCopiedMirror(true);
        setTimeout(() => setCopiedMirror(false), 1500);
    };

    const handleApply = async () => {
        if (!onApply) return;
        const result = await onApply();
        setApplyResult(result);
        if (result.success) {
            handleApplyDone();
        }
    };

    const handleApplyWithStatusLine = async () => {
        if (!onApplyWithStatusLine) return;
        const result = await onApplyWithStatusLine();
        setApplyResult(result);
        if (result.success) {
            handleApplyDone();
        }
    };

    const progressLabel = allDone ? 'Done' : `${doneCount}/${TOTAL_STEPS}`;
    const progressColor = allDone ? 'success' : 'default';

    const collapsedHint = !providerDone
        ? 'Add a provider to get started'
        : !modelDone
            ? 'Pick a model'
            : !installDone
                ? `Install ${agentName}`
                : `${applyStepLabel} to finish`;

    return (
        <UnifiedCard
            size="full"
            title={
                <Stack direction="row" alignItems="center" spacing={1} sx={{ flex: 1 }}>
                    <Typography variant="subtitle1" fontWeight={600}>
                        Guiding
                    </Typography>
                    <Chip
                        label={progressLabel}
                        size="small"
                        color={progressColor as any}
                        sx={{ height: 20, fontSize: '0.7rem' }}
                    />
                    {collapsed && !allDone && (
                        <Typography variant="caption" color="text.secondary" sx={{ ml: 0.5 }}>
                            {collapsedHint}
                        </Typography>
                    )}
                </Stack>
            }
            rightAction={
                <Tooltip title={collapsed ? 'Expand' : 'Collapse'}>
                    <IconButton size="small" onClick={toggleCollapsed}>
                        {collapsed ? <ExpandMoreIcon fontSize="small" /> : <ExpandLessIcon fontSize="small" />}
                    </IconButton>
                </Tooltip>
            }
        >
            <Collapse in={!collapsed} unmountOnExit={false}>
                <Stack spacing={2.5}>
                    <Box sx={{ width: '100%', px: { xs: 0, sm: 1 }, pt: 0.5 }}>
                    <Stepper activeStep={stepCursor} orientation="horizontal" alternativeLabel>
                            <Step completed={providerDone}>
                                <StepLabel optional={isStepSkipped(0) ? <Typography variant="caption">Skipped</Typography> : undefined}>{STEP_LABELS[0]}</StepLabel>
                            </Step>
                            <Step completed={modelDone}>
                                <StepLabel optional={isStepSkipped(1) ? <Typography variant="caption">Skipped</Typography> : undefined}>{STEP_LABELS[1]}</StepLabel>
                            </Step>
                            <Step completed={installDone}>
                                <StepLabel optional={isStepSkipped(2) ? <Typography variant="caption">Skipped</Typography> : undefined}>{STEP_LABELS[2]}</StepLabel>
                            </Step>
                            <Step completed={applyDone}>
                                <StepLabel optional={isStepSkipped(3) ? <Typography variant="caption">Skipped</Typography> : undefined}>{STEP_LABELS[3]}</StepLabel>
                            </Step>
                    </Stepper>
                    </Box>

                    <Stack spacing={1.5} sx={{ width: '100%', minHeight: 160, justifyContent: 'space-between', border: 1, borderColor: 'divider', borderRadius: 2, p: 1.5 }}>
                        <Box sx={{ flex: 1 }}>
                        {stepCursor === 0 && (
                        <Stack direction="row" spacing={1.5} alignItems="flex-start">
                            {providerLoading ? <CircularProgress size={20} sx={{ mt: 0.2, flexShrink: 0 }} /> : null}
                            <Box sx={{ flex: 1 }}>
                                <Typography variant="body2" fontWeight={500} color={providerDone ? 'text.primary' : 'primary.main'}>
                                    Step 1 — Add a Provider
                                </Typography>
                                {providerDone ? (
                                    <Typography variant="caption" color="text.secondary">
                                        {providerCount === 1
                                            ? `${providerCount} provider ready`
                                            : `${providerCount} providers ready`}
                                    </Typography>
                                ) : (
                                    <Stack direction="row" spacing={1} alignItems="center" sx={{ mt: 0.5 }}>
                                        <Typography variant="caption" color="text.secondary">
                                            An AI provider is required to use {agentName}.
                                        </Typography>
                                        <Button component={Link} to="/onboarding" size="small" variant="outlined" sx={{ flexShrink: 0, py: 0, fontSize: '0.7rem' }}>
                                            Add Provider
                                        </Button>
                                    </Stack>
                                )}
                            </Box>
                        </Stack>
                        )}

                        {stepCursor === 1 && (
                        <Stack direction="row" spacing={1.5} alignItems="flex-start">
                            <Box sx={{ flex: 1 }}>
                                <Typography variant="body2" fontWeight={500} color={!providerDone ? 'text.disabled' : modelDone ? 'text.primary' : 'primary.main'}>
                                    Step 2 — Select a Model
                                </Typography>
                                {modelDone ? (
                                    <Typography variant="caption" color="text.secondary">Model selected — you're ready to go.</Typography>
                                ) : (
                                    <Stack direction="row" spacing={1} alignItems="center" sx={{ mt: 0.5 }} flexWrap="wrap" gap={1}>
                                        <Typography variant="caption" color="text.secondary">Pick a model in <em>Models and Forwarding Rules</em> to start routing requests.</Typography>
                                        {onSelectModel && <Button size="small" variant="outlined" disabled={!providerDone} onClick={onSelectModel} sx={{ flexShrink: 0, py: 0, fontSize: '0.7rem' }}>Choose Model</Button>}
                                    </Stack>
                                )}
                            </Box>
                        </Stack>
                        )}

                        {stepCursor === 2 && (
                        <Stack direction="row" spacing={1.5} alignItems="flex-start"><Box sx={{ flex: 1 }}>
                            <Typography variant="body2" fontWeight={500} color={!modelDone ? 'text.disabled' : installDone ? 'text.primary' : 'primary.main'}>Step 3 — Install {agentName}</Typography>
                            {installStepDescription && (
                                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
                                    {installStepDescription}
                                </Typography>
                            )}
                            {installActions?.length ? (
                                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ maxWidth: 520 }}>
                                    {installActions.map((action) => (
                                        <Button
                                            key={`${action.label}-${action.href}`}
                                            href={action.href}
                                            target={action.external ? '_blank' : undefined}
                                            rel={action.external ? 'noopener noreferrer' : undefined}
                                            variant={action.variant ?? 'outlined'}
                                            size="small"
                                            disabled={!modelDone}
                                            sx={{ flex: 1 }}
                                        >
                                            {action.label}
                                        </Button>
                                    ))}
                                </Stack>
                            ) : (
                                <>
                                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, maxWidth: 800 }}><Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem', minWidth: '80px' }}>npm official</Typography><Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flex: 1, minWidth: 0 }}><Tooltip title={copied ? 'Copied!' : 'Copy'}><IconButton size="small" onClick={handleCopy} sx={{ flexShrink: 0, p: 0.25 }}><ContentCopyIcon sx={{ fontSize: 14 }} /></IconButton></Tooltip><Typography variant="body2" onClick={handleCopy} sx={{ fontFamily: 'monospace', flex: 1, fontSize: '0.8rem', color: 'text.primary', cursor: 'pointer', '&:hover': { color: 'primary.main' }, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={installCommand}>{installCommand}</Typography></Box></Box>
                                    {installMirrorCommand && (<Box sx={{ display: 'flex', alignItems: 'center', gap: 2, maxWidth: 800, mt: 0.75 }}><Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem', minWidth: '80px' }}>npm mirror</Typography><Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flex: 1, minWidth: 0 }}><Tooltip title={copiedMirror ? 'Copied!' : 'Copy'}><IconButton size="small" onClick={handleCopyMirror} sx={{ flexShrink: 0, p: 0.25 }}><ContentCopyIcon sx={{ fontSize: 14 }} /></IconButton></Tooltip><Typography variant="body2" onClick={handleCopyMirror} sx={{ fontFamily: 'monospace', flex: 1, fontSize: '0.8rem', color: 'text.primary', cursor: 'pointer', '&:hover': { color: 'primary.main' }, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={installMirrorCommand}>{installMirrorCommand}</Typography></Box></Box>)}
                                </>
                            )}
                        </Box></Stack>
                        )}

                        {stepCursor === 3 && (
                        <Stack direction="row" spacing={1.5} alignItems="flex-start"><Box sx={{ flex: 1 }}><Typography variant="body2" fontWeight={500} color={!installDone ? 'text.disabled' : applyDone ? 'text.primary' : 'primary.main'}>Step 4 — {applyStepLabel}</Typography><Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>{applyStepDescription ?? `Write the proxy configuration to ${agentName}'s settings file.`}</Typography><Collapse in={!applyDone}><Stack direction="row" spacing={1} flexWrap="wrap" gap={1}>{onApply && <Button variant="contained" size="small" disabled={!installDone || isApplyLoading} onClick={handleApply} startIcon={isApplyLoading ? <CircularProgress size={14} color="inherit" /> : undefined}>{applyButtonLabel}</Button>}{onApplyWithStatusLine && <Button variant="outlined" size="small" disabled={!installDone || isApplyLoading} onClick={handleApplyWithStatusLine}>Apply + Status Line</Button>}{onViewConfig && <Button variant={onApply ? "outlined" : "contained"} size="small" disabled={!installDone} onClick={onViewConfig}>{viewConfigButtonLabel}</Button>}</Stack></Collapse>{applyResult && (<Alert severity={applyResult.success ? 'success' : 'error'} sx={{ mt: 1, py: 0.5 }}>{applyResult.success ? (<Box><Typography variant="caption" fontWeight={600}>{applySuccessLabel}</Typography>{applyResult.files?.map(f => (<Typography key={f} variant="caption" sx={{ display: 'block', fontFamily: 'monospace', color: 'text.secondary' }}>{f}</Typography>))}</Box>) : (<Typography variant="caption">{applyResult.error ?? 'Apply failed'}</Typography>)}</Alert>)}</Box></Stack>
                        )}

                        </Box>

                        <Stack
                            direction="row"
                            justifyContent="space-between"
                            alignItems="center"
                            sx={{ pt: 1, borderTop: 1, borderColor: 'divider' }}
                        >
                            <Button size="small" onClick={handleReset} disabled={stepCursor === 0 && !installDone && !applyDone && skipped.size === 0}>Reset</Button>
                            <Stack direction="row" spacing={1}>
                                <Button size="small" onClick={handleBack} disabled={stepCursor === 0}>Back</Button>
                                <Button size="small" onClick={handleSkip}>Skip</Button>
                                <Button size="small" variant="contained" onClick={handleNext}>{stepCursor >= TOTAL_STEPS - 1 ? 'Finish' : 'Next'}</Button>
                            </Stack>
                        </Stack>
                    </Stack>
                </Stack>
            </Collapse>
        </UnifiedCard>
    );
};

export default AgentSetupCard;
