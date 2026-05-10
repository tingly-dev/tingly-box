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
    error?: string;
}

export interface AgentSetupCardProps {
    agentKey: string;
    agentName: string;
    installCommand: string;
    installMirrorCommand?: string;
    onApply: () => Promise<AgentApplyResult>;
    onApplyWithStatusLine?: () => Promise<AgentApplyResult>;
    isApplyLoading?: boolean;
    onViewConfig?: () => void;
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
    onApply,
    onApplyWithStatusLine,
    isApplyLoading = false,
    onViewConfig,
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

    const handleInstallDone = () => {
        localStorage.setItem(INSTALL_DONE_KEY(agentKey), 'true');
        setInstallDone(true);
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
                : 'Apply config to finish';

    return (
        <UnifiedCard
            size="full"
            title={
                <Stack direction="row" alignItems="center" spacing={1} sx={{ flex: 1 }}>
                    <Typography variant="subtitle1" fontWeight={600}>
                        Quick Setup
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
                    <Stepper activeStep={doneCount} alternativeLabel sx={{ px: 1 }}>
                        <Step completed={providerDone}>
                            <StepLabel>Provider</StepLabel>
                        </Step>
                        <Step completed={modelDone}>
                            <StepLabel>Model</StepLabel>
                        </Step>
                        <Step completed={installDone}>
                            <StepLabel>Install</StepLabel>
                        </Step>
                        <Step completed={applyDone}>
                            <StepLabel>Apply</StepLabel>
                        </Step>
                    </Stepper>
                    {/* Step 1: Provider */}
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
                                    <Button
                                        component={Link}
                                        to="/onboarding"
                                        size="small"
                                        variant="outlined"
                                        sx={{ flexShrink: 0, py: 0, fontSize: '0.7rem' }}
                                    >
                                        Add Provider
                                    </Button>
                                </Stack>
                            )}
                        </Box>
                    </Stack>

                    {/* Step 2: Select a Model */}
                    <Stack direction="row" spacing={1.5} alignItems="flex-start">
                        <Box sx={{ flex: 1 }}>
                            <Typography
                                variant="body2"
                                fontWeight={500}
                                color={!providerDone ? 'text.disabled' : modelDone ? 'text.primary' : 'primary.main'}
                            >
                                Step 2 — Select a Model
                            </Typography>
                            {modelDone ? (
                                <Typography variant="caption" color="text.secondary">
                                    Model selected — you're ready to go.
                                </Typography>
                            ) : (
                                <Stack direction="row" spacing={1} alignItems="center" sx={{ mt: 0.5 }} flexWrap="wrap" gap={1}>
                                    <Typography variant="caption" color="text.secondary">
                                        Pick a model in <em>Models and Forwarding Rules</em> to start routing requests.
                                    </Typography>
                                    {onSelectModel && (
                                        <Button
                                            size="small"
                                            variant="outlined"
                                            disabled={!providerDone}
                                            onClick={onSelectModel}
                                            sx={{ flexShrink: 0, py: 0, fontSize: '0.7rem' }}
                                        >
                                            Choose Model
                                        </Button>
                                    )}
                                </Stack>
                            )}
                        </Box>
                    </Stack>

                    {/* Step 3: Install */}
                    <Stack direction="row" spacing={1.5} alignItems="flex-start">
                        <Box sx={{ flex: 1 }}>
                            <Typography
                                variant="body2"
                                fontWeight={500}
                                color={!modelDone ? 'text.disabled' : installDone ? 'text.primary' : 'primary.main'}
                            >
                                Step 3 — Install {agentName}
                            </Typography>

                            {/* npm official */}
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, maxWidth: 800 }}>
                                <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem', minWidth: '80px' }}>
                                    npm official
                                </Typography>
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flex: 1, minWidth: 0 }}>
                                    <Tooltip title={copied ? 'Copied!' : 'Copy'}>
                                        <IconButton size="small" onClick={handleCopy} sx={{ flexShrink: 0, p: 0.25 }}>
                                            <ContentCopyIcon sx={{ fontSize: 14 }} />
                                        </IconButton>
                                    </Tooltip>
                                    <Typography
                                        variant="body2"
                                        onClick={handleCopy}
                                        sx={{
                                            fontFamily: 'monospace',
                                            flex: 1,
                                            fontSize: '0.8rem',
                                            color: 'text.primary',
                                            cursor: 'pointer',
                                            '&:hover': {
                                                color: 'primary.main',
                                            },
                                            overflow: 'hidden',
                                            textOverflow: 'ellipsis',
                                            whiteSpace: 'nowrap',
                                        }}
                                        title={installCommand}
                                    >
                                        {installCommand}
                                    </Typography>
                                </Box>
                            </Box>

                            {/* npm mirror */}
                            {installMirrorCommand && (
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, maxWidth: 800, mt: 0.75 }}>
                                    <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem', minWidth: '80px' }}>
                                        npm mirror
                                    </Typography>
                                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flex: 1, minWidth: 0 }}>
                                        <Tooltip title={copiedMirror ? 'Copied!' : 'Copy'}>
                                            <IconButton size="small" onClick={handleCopyMirror} sx={{ flexShrink: 0, p: 0.25 }}>
                                                <ContentCopyIcon sx={{ fontSize: 14 }} />
                                            </IconButton>
                                        </Tooltip>
                                        <Typography
                                            variant="body2"
                                            onClick={handleCopyMirror}
                                            sx={{
                                                fontFamily: 'monospace',
                                                flex: 1,
                                                fontSize: '0.8rem',
                                                color: 'text.primary',
                                                cursor: 'pointer',
                                                '&:hover': {
                                                    color: 'primary.main',
                                                },
                                                overflow: 'hidden',
                                                textOverflow: 'ellipsis',
                                                whiteSpace: 'nowrap',
                                            }}
                                            title={installMirrorCommand}
                                        >
                                            {installMirrorCommand}
                                        </Typography>
                                    </Box>
                                </Box>
                            )}

                            {!installDone && (
                                <Button
                                    size="small"
                                    variant="outlined"
                                    onClick={handleInstallDone}
                                    sx={{ mt: 0.5, fontSize: '0.75rem', px: 0 }}
                                >
                                    Mark as Installed
                                </Button>
                            )}
                        </Box>
                    </Stack>

                    {/* Step 4: Apply Config */}
                    <Stack direction="row" spacing={1.5} alignItems="flex-start">
                        <Box sx={{ flex: 1 }}>
                            <Typography
                                variant="body2"
                                fontWeight={500}
                                color={!installDone ? 'text.disabled' : applyDone ? 'text.primary' : 'primary.main'}
                            >
                                Step 4 — Apply Config
                            </Typography>
                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
                                Write the proxy configuration to {agentName}'s settings file.
                            </Typography>

                            <Collapse in={!applyDone}>
                                <Stack direction="row" spacing={1} flexWrap="wrap" gap={1}>
                                    <Button
                                        variant="contained"
                                        size="small"
                                        disabled={!installDone || isApplyLoading}
                                        onClick={handleApply}
                                        startIcon={isApplyLoading ? <CircularProgress size={14} color="inherit" /> : undefined}
                                    >
                                        Apply
                                    </Button>
                                    {onApplyWithStatusLine && (
                                        <Button
                                            variant="outlined"
                                            size="small"
                                            disabled={!installDone || isApplyLoading}
                                            onClick={handleApplyWithStatusLine}
                                        >
                                            Apply + Status Line
                                        </Button>
                                    )}
                                    {onViewConfig && (
                                        <Button
                                            variant="outlined"
                                            size="small"
                                            onClick={onViewConfig}
                                        >
                                            Manual Setup
                                        </Button>
                                    )}
                                    <Button
                                        variant="text"
                                        size="small"
                                        disabled={!installDone}
                                        onClick={handleApplyDone}
                                        sx={{ fontSize: '0.75rem' }}
                                    >
                                        Mark as Configured
                                    </Button>
                                </Stack>
                            </Collapse>

                            {applyResult && (
                                <Alert
                                    severity={applyResult.success ? 'success' : 'error'}
                                    sx={{ mt: 1, py: 0.5 }}
                                >
                                    {applyResult.success ? (
                                        <Box>
                                            <Typography variant="caption" fontWeight={600}>Config applied!</Typography>
                                            {applyResult.files?.map(f => (
                                                <Typography key={f} variant="caption" sx={{ display: 'block', fontFamily: 'monospace', color: 'text.secondary' }}>
                                                    {f}
                                                </Typography>
                                            ))}
                                        </Box>
                                    ) : (
                                        <Typography variant="caption">{applyResult.error ?? 'Apply failed'}</Typography>
                                    )}
                                </Alert>
                            )}
                        </Box>
                    </Stack>
                </Stack>
            </Collapse>
        </UnifiedCard>
    );
};

export default AgentSetupCard;
