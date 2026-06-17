import { ContentCopy as ContentCopyIcon } from '@/components/icons';
import { CheckCircle as CheckCircleIcon } from '@/components/icons';
import { ExpandLess as ExpandLessIcon } from '@/components/icons';
import { ExpandMore as ExpandMoreIcon } from '@/components/icons';
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
    alpha,
} from '@mui/material';
import { useEffect, useRef, useState } from 'react';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import { SPOTLIGHT_ADD_MODEL_EVENT } from '@/components/nodes/ActionAddNode';

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
}

const COLLAPSED_KEY = (agentKey: string) => `setup-card-collapsed-${agentKey}`;
const INSTALL_DONE_KEY = (agentKey: string) => `setup-card-step2-done-${agentKey}`;
const APPLY_DONE_KEY = (agentKey: string) => `setup-card-step3-done-${agentKey}`;
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
            fontSize: '0.7rem', fontWeight: 700,
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
    applyStepLabel = 'Auto Config',
    applyStepDescription,
    applyButtonLabel = 'Auto Config',
    applySuccessLabel = 'Config applied!',
    viewConfigButtonLabel = 'Config',
    hasModelSelected = false,
    onSelectModel,
    onConnectProvider,
}) => {
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
    // Tracks which completed steps the user has manually expanded
    const [expandedDoneSteps, setExpandedDoneSteps] = useState<Set<number>>(new Set());

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
            const enabled = providers.filter((p: any) => p.enabled);
            setHasProvider(enabled.length > 0);
            setProviderCount(enabled.length);
            setProviderLoading(false);
        }).catch(() => {
            if (!cancelled) setProviderLoading(false);
        });
        return () => { cancelled = true; };
    }, []);

    const providerDone = hasProvider;
    const modelDone = hasModelSelected;
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
            localStorage.setItem(COLLAPSED_KEY(agentKey), 'true');
        }
    }, [providerLoading, allDone, agentKey]);

    const toggleCollapsed = () => {
        const next = !collapsed;
        localStorage.setItem(COLLAPSED_KEY(agentKey), String(next));
        setCollapsed(next);
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

    const markInstallDone = () => {
        localStorage.setItem(INSTALL_DONE_KEY(agentKey), 'true');
        setInstallDone(true);
    };

    const handleApplyWithStatusLine = async () => {
        if (!onApplyWithStatusLine) return;
        const result = await onApplyWithStatusLine();
        setApplyResult(result);
        if (result.success) {
            localStorage.setItem(APPLY_DONE_KEY(agentKey), 'true');
            setApplyDone(true);
        }
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
    };

    const progressLabel = allDone ? 'Done' : `${doneCount}/${TOTAL_STEPS}`;
    const progressColor = allDone ? 'success' : 'default';

    const collapsedHint = !providerDone
        ? 'Connect an AI provider to get started'
        : !modelDone
            ? 'Choose a model to continue'
            : !installDone
                ? `Install ${agentName} on your machine`
                : `One-click ${applyStepLabel} to finish`;

    // Which step is the first incomplete one (determines which expands)
    const firstIncomplete = !providerDone ? 0 : !modelDone ? 1 : !installDone ? 2 : !applyDone ? 3 : -1;

    const stepRowSx = (done: boolean, active: boolean) => ({
        py: active ? 1.25 : 0.75,
        px: 1.5,
        borderRadius: 1.5,
        transition: 'background-color 0.15s ease',
        ...(active && !done ? {
            bgcolor: (theme: any) => alpha(theme.palette.primary.main, 0.04),
            border: 1,
            borderColor: 'divider',
        } : {}),
    });

    return (
        <UnifiedCard
            size="header"
            title={
                <Stack direction="row" alignItems="center" spacing={1} sx={{ flex: 1 }}>
                    <Typography variant="subtitle1" fontWeight={600}>Quick Start</Typography>
                    <Chip
                        label={progressLabel}
                        size="small"
                        color={progressColor as any}
                        sx={{ height: 20, fontSize: '0.75rem' }}
                    />
                    {collapsed && !allDone && (
                        <Typography variant="body2" color="text.secondary" sx={{ ml: 0.5 }}>
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
                <Stack spacing={0.5}>

                    {/* Step 1 — Provider */}
                    <Box sx={stepRowSx(providerDone, firstIncomplete === 0)}>
                        <Stack
                            direction="row" spacing={1.25} alignItems="center"
                            onClick={providerDone ? () => toggleDoneStep(0) : undefined}
                            sx={providerDone ? { cursor: 'pointer', '&:hover': { opacity: 0.8 } } : undefined}
                        >
                            {providerLoading ? <CircularProgress size={20} sx={{ flexShrink: 0 }} /> : <StepIndicator step={1} done={providerDone} active={firstIncomplete === 0} />}
                            <Typography variant="body2" fontWeight={500} color={providerDone ? 'text.primary' : firstIncomplete === 0 ? 'primary.main' : 'text.disabled'} sx={{ flex: 1 }}>
                                Connect AI Provider
                            </Typography>
                            {providerDone && (
                                <Typography variant="body2" color="text.secondary">
                                    {providerCount} provider{providerCount !== 1 ? 's' : ''}
                                </Typography>
                            )}
                            {providerDone && onConnectProvider && (
                                <Button size="small" variant="text" onClick={(e) => { e.stopPropagation(); onConnectProvider(); }} sx={{ py: 0, textTransform: 'none', minWidth: 0 }}>+ Connect</Button>
                            )}
                            {providerDone && (
                                expandedDoneSteps.has(0) ? <ExpandLessIcon fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} /> : <ExpandMoreIcon fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} />
                            )}
                        </Stack>
                        <Collapse in={(!providerDone && firstIncomplete === 0) || expandedDoneSteps.has(0)}>
                            <Stack spacing={0.75} sx={{ mt: 0.75, pl: 4.25 }}>
                                <Typography variant="body2" color="text.secondary">
                                    Connect an AI provider (e.g. OpenAI, Anthropic, DeepSeek) to start using {agentName}.
                                </Typography>
                                <Box>
                                    <Button size="small" variant="contained" onClick={onConnectProvider} sx={{ py: 0.25 }}>
                                        Connect AI
                                    </Button>
                                </Box>
                            </Stack>
                        </Collapse>
                    </Box>

                    {/* Step 2 — Model */}
                    <Box sx={stepRowSx(modelDone, firstIncomplete === 1)}>
                        <Stack
                            direction="row" spacing={1.25} alignItems="center"
                            onClick={modelDone ? () => toggleDoneStep(1) : undefined}
                            sx={modelDone ? { cursor: 'pointer', '&:hover': { opacity: 0.8 } } : undefined}
                        >
                            <StepIndicator step={2} done={modelDone} active={firstIncomplete === 1} />
                            <Typography variant="body2" fontWeight={500} color={modelDone ? 'text.primary' : firstIncomplete === 1 ? 'primary.main' : 'text.disabled'} sx={{ flex: 1 }}>
                                Select a Model
                            </Typography>
                            {modelDone && (
                                <Typography variant="body2" color="text.secondary">Configured</Typography>
                            )}
                            {modelDone && onSelectModel && (
                                <Button size="small" variant="text" onClick={(e) => { e.stopPropagation(); onSelectModel(); }} sx={{ py: 0, textTransform: 'none', minWidth: 0 }}>Change</Button>
                            )}
                            {modelDone && (
                                expandedDoneSteps.has(1) ? <ExpandLessIcon fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} /> : <ExpandMoreIcon fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} />
                            )}
                        </Stack>
                        <Collapse in={(!modelDone && firstIncomplete === 1) || expandedDoneSteps.has(1)}>
                            <Stack spacing={0.75} sx={{ mt: 0.75, pl: 4.25 }}>
                                <Typography variant="body2" color="text.secondary">
                                    Choose which model {agentName} will use in the <em>Model Rules</em> section below.
                                </Typography>
                                {onSelectModel && (
                                    <Box>
                                        <Button size="small" variant="contained" disabled={!providerDone} onClick={onSelectModel} sx={{ py: 0.25 }}>
                                            Choose Model
                                        </Button>
                                    </Box>
                                )}
                            </Stack>
                        </Collapse>
                    </Box>

                    {/* Step 3 — Install */}
                    <Box sx={stepRowSx(installDone, firstIncomplete === 2)}>
                        <Stack
                            direction="row" spacing={1.25} alignItems="center"
                            onClick={installDone ? () => toggleDoneStep(2) : undefined}
                            sx={installDone ? { cursor: 'pointer', '&:hover': { opacity: 0.8 } } : undefined}
                        >
                            <StepIndicator step={3} done={installDone} active={firstIncomplete === 2} />
                            <Typography variant="body2" fontWeight={500} color={installDone ? 'text.primary' : firstIncomplete === 2 ? 'primary.main' : 'text.disabled'} sx={{ flex: 1 }}>
                                Install {agentName}
                            </Typography>
                            {installDone && (
                                <Typography variant="body2" color="text.secondary">Installed</Typography>
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
                                            <Typography variant="body2" color="text.secondary">{installStepDescription}</Typography>
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
                                        <Typography variant="body2" color="text.secondary">
                                            {installStepDescription || `Install ${agentName} on your local machine — copy and run the command below.`}
                                        </Typography>
                                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, maxWidth: 800 }}>
                                            <Typography variant="body2" color="text.secondary" sx={{ minWidth: '80px' }}>npm official</Typography>
                                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flex: 1, minWidth: 0 }}>
                                                <Tooltip title={copied ? 'Copied!' : 'Copy'}>
                                                    <IconButton size="small" onClick={handleCopy} sx={{ flexShrink: 0, p: 0.25 }}><ContentCopyIcon sx={{ fontSize: 16 }} /></IconButton>
                                                </Tooltip>
                                                <Typography variant="body2" onClick={handleCopy} sx={{ fontFamily: 'monospace', flex: 1, color: 'text.primary', cursor: 'pointer', '&:hover': { color: 'primary.main' }, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={installCommand}>{installCommand}</Typography>
                                            </Box>
                                        </Box>
                                        {installMirrorCommand && (
                                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, maxWidth: 800 }}>
                                                <Typography variant="body2" color="text.secondary" sx={{ minWidth: '80px' }}>npm mirror</Typography>
                                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flex: 1, minWidth: 0 }}>
                                                    <Tooltip title={copiedMirror ? 'Copied!' : 'Copy'}>
                                                        <IconButton size="small" onClick={handleCopyMirror} sx={{ flexShrink: 0, p: 0.25 }}><ContentCopyIcon sx={{ fontSize: 16 }} /></IconButton>
                                                    </Tooltip>
                                                    <Typography variant="body2" onClick={handleCopyMirror} sx={{ fontFamily: 'monospace', flex: 1, color: 'text.primary', cursor: 'pointer', '&:hover': { color: 'primary.main' }, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={installMirrorCommand}>{installMirrorCommand}</Typography>
                                                </Box>
                                            </Box>
                                        )}
                                    </>
                                )}
                                <Box>
                                    <Button size="small" variant="text" onClick={markInstallDone} sx={{ py: 0, textTransform: 'none', color: 'text.secondary' }}>
                                        I've installed it
                                    </Button>
                                </Box>
                            </Stack>
                        </Collapse>
                    </Box>

                    {/* Step 4 — Apply */}
                    <Box sx={stepRowSx(applyDone, firstIncomplete === 3)}>
                        <Stack
                            direction="row" spacing={1.25} alignItems="center"
                            onClick={applyDone ? () => toggleDoneStep(3) : undefined}
                            sx={applyDone ? { cursor: 'pointer', '&:hover': { opacity: 0.8 } } : undefined}
                        >
                            <StepIndicator step={4} done={applyDone} active={firstIncomplete === 3} />
                            <Typography variant="body2" fontWeight={500} color={applyDone ? 'text.primary' : firstIncomplete === 3 ? 'primary.main' : 'text.disabled'} sx={{ flex: 1 }}>
                                {applyStepLabel}
                            </Typography>
                            {applyDone && (
                                <Typography variant="body2" color="text.secondary">Applied</Typography>
                            )}
                            {applyDone && (
                                expandedDoneSteps.has(3) ? <ExpandLessIcon fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} /> : <ExpandMoreIcon fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} />
                            )}
                        </Stack>
                        <Collapse in={(!applyDone && firstIncomplete === 3) || expandedDoneSteps.has(3)}>
                            <Stack spacing={0.75} sx={{ mt: 0.75, pl: 4.25 }}>
                                <Typography variant="body2" color="text.secondary">
                                    {applyStepDescription ?? `One click to write the proxy configuration to ${agentName}'s settings file.`}
                                </Typography>
                                <Stack direction="row" spacing={1} flexWrap="wrap" gap={0.5}>
                                    {onApply && (
                                        <Button variant="contained" size="small" disabled={isApplyLoading} onClick={handleApplyWithStatusLine} startIcon={isApplyLoading ? <CircularProgress size={14} color="inherit" /> : undefined}>
                                            {applyButtonLabel}
                                        </Button>
                                    )}
                                    {onViewConfig && (
                                        <Button variant="text" size="small" onClick={onViewConfig} sx={{ textTransform: 'none', color: 'text.secondary' }}>
                                            {viewConfigButtonLabel} (Advanced)
                                        </Button>
                                    )}
                                    {!applyDone && (
                                        <Button variant="text" size="small" onClick={() => {
                                            localStorage.setItem(APPLY_DONE_KEY(agentKey), 'true');
                                            setApplyDone(true);
                                        }} sx={{ textTransform: 'none', color: 'text.disabled' }}>
                                            Skip
                                        </Button>
                                    )}
                                </Stack>
                                {applyResult && (
                                    <Alert severity={applyResult.success ? 'success' : 'error'} sx={{ mt: 0.5, py: 0.5 }}>
                                        {applyResult.success ? (
                                            <Box>
                                                <Typography variant="body2" fontWeight={600}>{applySuccessLabel}</Typography>
                                                {applyResult.files?.map(f => (
                                                    <Typography key={f} variant="body2" sx={{ display: 'block', fontFamily: 'monospace', color: 'text.secondary' }}>{f}</Typography>
                                                ))}
                                            </Box>
                                        ) : (
                                            <Typography variant="body2">{applyResult.error ?? 'Apply failed'}</Typography>
                                        )}
                                    </Alert>
                                )}
                            </Stack>
                        </Collapse>
                    </Box>

                    {/* Reset link — only visible when something has been manually completed */}
                    {(installDone || applyDone) && (
                        <Box sx={{ pt: 0.5, pl: 1.5 }}>
                            <Button size="small" variant="text" onClick={handleReset} sx={{ py: 0, textTransform: 'none', color: 'text.disabled', fontSize: '0.75rem' }}>
                                Reset progress
                            </Button>
                        </Box>
                    )}
                </Stack>
            </Collapse>
        </UnifiedCard>
    );
};

export default AgentSetupCard;
