import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import RadioButtonUncheckedIcon from '@mui/icons-material/RadioButtonUnchecked';
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
import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import { copyableTextStyle } from '@/styles/textStyles';

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
}

const COLLAPSED_KEY = (agentKey: string) => `setup-card-collapsed-${agentKey}`;
const STEP2_KEY = (agentKey: string) => `setup-card-step2-done-${agentKey}`;
const STEP3_KEY = (agentKey: string) => `setup-card-step3-done-${agentKey}`;

const StepIcon: React.FC<{ done: boolean; active: boolean }> = ({ done, active }) => {
    if (done) return <CheckCircleIcon sx={{ color: 'success.main', fontSize: 20 }} />;
    if (active) return <RadioButtonUncheckedIcon sx={{ color: 'primary.main', fontSize: 20 }} />;
    return <RadioButtonUncheckedIcon sx={{ color: 'text.disabled', fontSize: 20 }} />;
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
}) => {
    const [collapsed, setCollapsed] = useState(
        () => localStorage.getItem(COLLAPSED_KEY(agentKey)) === 'true'
    );
    const [step2Done, setStep2Done] = useState(
        () => localStorage.getItem(STEP2_KEY(agentKey)) === 'true'
    );
    const [step3Done, setStep3Done] = useState(
        () => localStorage.getItem(STEP3_KEY(agentKey)) === 'true'
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

    const step1Done = hasProvider;
    const allDone = step1Done && step2Done && step3Done;
    const doneCount = [step1Done, step2Done, step3Done].filter(Boolean).length;

    const toggleCollapsed = () => {
        const next = !collapsed;
        localStorage.setItem(COLLAPSED_KEY(agentKey), String(next));
        setCollapsed(next);
    };

    const handleStep2Done = () => {
        localStorage.setItem(STEP2_KEY(agentKey), 'true');
        setStep2Done(true);
    };

    const handleStep3Done = () => {
        localStorage.setItem(STEP3_KEY(agentKey), 'true');
        setStep3Done(true);
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
            handleStep3Done();
        }
    };

    const handleApplyWithStatusLine = async () => {
        if (!onApplyWithStatusLine) return;
        const result = await onApplyWithStatusLine();
        setApplyResult(result);
        if (result.success) {
            handleStep3Done();
        }
    };

    const progressLabel = allDone ? 'Done' : `${doneCount}/3`;
    const progressColor = allDone ? 'success' : 'default';

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
                            {!step1Done ? 'Add a provider to get started' :
                             !step2Done ? `Install ${agentName}` :
                             'Apply config to finish'}
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
                    {/* Step 1: Provider */}
                    <Stack direction="row" spacing={1.5} alignItems="flex-start">
                        {providerLoading
                            ? <CircularProgress size={20} sx={{ mt: 0.2, flexShrink: 0 }} />
                            : <StepIcon done={step1Done} active={!step1Done} />
                        }
                        <Box sx={{ flex: 1 }}>
                            <Typography variant="body2" fontWeight={500} color={step1Done ? 'text.primary' : 'primary.main'}>
                                Step 1 — Add a Provider
                            </Typography>
                            {step1Done ? (
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

                    {/* Step 2: Install */}
                    <Stack direction="row" spacing={1.5} alignItems="flex-start">
                        <StepIcon done={step2Done} active={!step2Done} />
                        <Box sx={{ flex: 1 }}>
                            <Typography variant="body2" fontWeight={500} color={step2Done ? 'text.primary' : 'primary.main'}>
                                Step 2 — Install {agentName}
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

                            {!step2Done && (
                                <Button
                                    size="small"
                                    variant="text"
                                    onClick={handleStep2Done}
                                    sx={{ mt: 0.5, fontSize: '0.75rem', px: 0 }}
                                >
                                    ✓ Already installed / Done
                                </Button>
                            )}
                        </Box>
                    </Stack>

                    {/* Step 3: Apply Config */}
                    <Stack direction="row" spacing={1.5} alignItems="flex-start">
                        <StepIcon done={step3Done} active={step2Done && !step3Done} />
                        <Box sx={{ flex: 1 }}>
                            <Typography
                                variant="body2"
                                fontWeight={500}
                                color={!step2Done ? 'text.disabled' : step3Done ? 'text.primary' : 'primary.main'}
                            >
                                Step 3 — Apply Config
                            </Typography>
                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
                                Write the proxy configuration to {agentName}'s settings file.
                            </Typography>

                            <Collapse in={!step3Done}>
                                <Stack direction="row" spacing={1} flexWrap="wrap" gap={1}>
                                    <Button
                                        variant="contained"
                                        size="small"
                                        disabled={!step2Done || isApplyLoading}
                                        onClick={handleApply}
                                        startIcon={isApplyLoading ? <CircularProgress size={14} color="inherit" /> : undefined}
                                    >
                                        Apply
                                    </Button>
                                    {onApplyWithStatusLine && (
                                        <Button
                                            variant="outlined"
                                            size="small"
                                            disabled={!step2Done || isApplyLoading}
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
                                            Manual
                                        </Button>
                                    )}
                                    <Button
                                        variant="text"
                                        size="small"
                                        disabled={!step2Done}
                                        onClick={handleStep3Done}
                                        sx={{ fontSize: '0.75rem' }}
                                    >
                                        ✓ Already configured / Done
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
