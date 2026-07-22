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
} from '@mui/material';
import { useEffect, useRef, useState } from 'react';
import UnifiedCard from '@/components/UnifiedCard';
import { providersDataCache } from '@/services/scenarioDataCache';
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
        const applyProviders = (providers: any[]) => {
            const enabled = providers.filter((p: any) => p.enabled);
            setHasProvider(enabled.length > 0);
            setProviderCount(enabled.length);
            setProviderLoading(false);
        };

        // Scenario pages on this route already fetch providers into the
        // shared cache (see scenarioDataCache.ts); read from it instead of
        // firing a second, independent request that would otherwise show
        // its own spinner right after the page's own loading resolves.
        const cached = providersDataCache.getCached();
        if (cached) applyProviders(cached);

        const unsubscribe = providersDataCache.subscribe(applyProviders);
        if (!cached) void providersDataCache.refresh();
        return unsubscribe;
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

    const stepRowSx = () => ({
        py: 0.75,
        px: 1.5,
        borderRadius: 1.5,
    });

    return (
        <UnifiedCard
            size="header"
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
                    }}>Quick Start</Typography>
                    <Chip
                        label={progressLabel}
                        size="small"
                        color={progressColor as any}
                        sx={{ height: 20, fontSize: '0.75rem' }}
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
                <Tooltip title={collapsed ? 'Expand' : 'Collapse'}>
                    <IconButton size="small" onClick={toggleCollapsed}>
                        {collapsed ? <ExpandMoreIcon fontSize="small" /> : <ExpandLessIcon fontSize="small" />}
                    </IconButton>
                </Tooltip>
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
                                Connect AI Provider
                            </Typography>
                            {providerDone && (
                                <Typography variant="body2" sx={{
                                    color: "text.secondary"
                                }}>
                                    {providerCount} provider{providerCount !== 1 ? 's' : ''}
                                </Typography>
                            )}
                            {onConnectProvider && (
                                <Tooltip title={providerDone ? '' : `Connect an AI provider (e.g. OpenAI, Anthropic, DeepSeek) to start using ${agentName}.`}>
                                    <Button
                                        size="small"
                                        variant={providerDone ? 'text' : 'contained'}
                                        onClick={onConnectProvider}
                                        sx={providerDone ? { py: 0, textTransform: 'none', minWidth: 0 } : { py: 0.25 }}
                                    >
                                        {providerDone ? '+ Connect' : 'Connect AI'}
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
                                Select a Model
                            </Typography>
                            {modelDone && (
                                <Typography variant="body2" sx={{
                                    color: "text.secondary"
                                }}>Configured</Typography>
                            )}
                            {onSelectModel && (
                                <Tooltip title={modelDone ? '' : `Choose which model ${agentName} will use in the Model Rules section below.`}>
                                    <span>
                                        <Button
                                            size="small"
                                            variant={modelDone ? 'text' : 'contained'}
                                            disabled={!modelDone && !providerDone}
                                            onClick={onSelectModel}
                                            sx={modelDone ? { py: 0, textTransform: 'none', minWidth: 0 } : { py: 0.25 }}
                                        >
                                            {modelDone ? 'Change' : 'Choose Model'}
                                        </Button>
                                    </span>
                                </Tooltip>
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
                                alignItems: "center"
                            }, installDone ? { cursor: 'pointer', '&:hover': { opacity: 0.8 } } : false]}>
                            <StepIndicator step={3} done={installDone} active={firstIncomplete === 2} />
                            <Typography
                                variant="body2"
                                color={installDone ? 'text.primary' : firstIncomplete === 2 ? 'primary.main' : 'text.disabled'}
                                sx={{
                                    fontWeight: 500,
                                    flex: 1
                                }}>
                                Install {agentName}
                            </Typography>
                            {installDone && (
                                <Typography variant="body2" sx={{
                                    color: "text.secondary"
                                }}>Installed</Typography>
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
                                            {installStepDescription || `Install ${agentName} on your local machine — copy and run the command below.`}
                                        </Typography>
                                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, maxWidth: 800 }}>
                                            <Typography
                                                variant="body2"
                                                sx={{
                                                    color: "text.secondary",
                                                    minWidth: '80px'
                                                }}>npm official</Typography>
                                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flex: 1, minWidth: 0 }}>
                                                <Tooltip title={copied ? 'Copied!' : 'Copy'}>
                                                    <IconButton size="small" onClick={handleCopy} sx={{ flexShrink: 0, p: 0.25 }}><ContentCopyIcon sx={{ fontSize: 16 }} /></IconButton>
                                                </Tooltip>
                                                <Typography variant="body2" onClick={handleCopy} sx={{ fontFamily: 'monospace', flex: 1, color: 'text.primary', cursor: 'pointer', '&:hover': { color: 'primary.main' }, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={installCommand}>{installCommand}</Typography>
                                            </Box>
                                        </Box>
                                        {installMirrorCommand && (
                                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, maxWidth: 800 }}>
                                                <Typography
                                                    variant="body2"
                                                    sx={{
                                                        color: "text.secondary",
                                                        minWidth: '80px'
                                                    }}>npm mirror</Typography>
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
                                }}>Applied</Typography>
                            )}
                            {!applyDone && firstIncomplete === 3 && (
                                <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', flexWrap: 'wrap', rowGap: 0.5 }}>
                                    {onApply && (
                                        <Tooltip title={applyStepDescription ?? `One click to write the proxy configuration to ${agentName}'s settings file.`}>
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
                                        localStorage.setItem(APPLY_DONE_KEY(agentKey), 'true');
                                        setApplyDone(true);
                                    }} sx={{ py: 0, textTransform: 'none', color: 'text.disabled', minWidth: 0 }}>
                                        Skip
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
                                            <Typography key={f} variant="body2" sx={{ display: 'block', fontFamily: 'monospace', color: 'text.secondary' }}>{f}</Typography>
                                        ))}
                                    </Box>
                                ) : (
                                    <Typography variant="body2">{applyResult.error ?? 'Apply failed'}</Typography>
                                )}
                            </Alert>
                        )}
                        <Collapse in={applyDone && expandedDoneSteps.has(3)}>
                            <Stack spacing={0.75} sx={{ mt: 0.75, pl: 4.25 }}>
                                <Typography variant="body2" sx={{
                                    color: "text.secondary"
                                }}>
                                    {applyStepDescription ?? `One click to write the proxy configuration to ${agentName}'s settings file.`}
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
                                            {viewConfigButtonLabel} (Advanced)
                                        </Button>
                                    )}
                                </Stack>
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
