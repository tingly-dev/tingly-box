import React, { useState, useEffect, memo, useCallback, useMemo } from 'react';
import {
    Dialog,
    DialogTitle,
    DialogContent,
    Box,
    Typography,
    Chip,
    LinearProgress,
    IconButton,
    Tooltip,
    Button,
    ToggleButton,
    ToggleButtonGroup,
    Collapse,
    Alert,
} from '@mui/material';
import {
    CheckCircle as CheckIcon,
    Error as ErrorIcon,
    Speed as SpeedIcon,
    Token as TokenIcon,
    ContentCopy as CopyIcon,
    Refresh as RefreshIcon,
    PlayArrow as RunIcon,
    ExpandMore as ExpandMoreIcon,
    ExpandLess as ExpandLessIcon,
} from '@/components/icons';
import { useTheme } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';
import { toggleButtonGroupStyle } from '@/styles/toggleStyles';
import type { ProbeResult, ProbeTestMode, ProbeTargetType } from '@/types/probe.ts';
import { runProbe } from './runProbe';

// ── Types ────────────────────────────────────────────────────────────────

interface ProbeDialogProps {
    open: boolean;
    onClose: () => void;
    targetType: ProbeTargetType;
    targetId: string;
    targetName: string;
    scenario?: string;
    model?: string;
    /** Initial request shape; can be changed inside the dialog. Defaults to stream. */
    testMode?: ProbeTestMode;
    /** Pre-computed result to show on open (e.g. from the quick test); re-run replaces it. */
    initialResult?: ProbeResult;
}

// ── Constants / helpers ────────────────────────────────────────────────────

// Request shapes the probe exercises (the "形态" dimension): we only need
// non-streaming vs streaming. ('simple' is the backend's value for nonstream.)
const MODES: { value: ProbeTestMode; tKey: string }[] = [
    { value: 'simple', tKey: 'probe.nonstream' },
    { value: 'streaming', tKey: 'probe.stream' },
];

// Human-friendly labels for routing_source values from the backend.
const ROUTING_SOURCE_LABELS: Record<string, string> = {
    affinity: 'Session Affinity',
    smart_routing: 'Smart Routing',
    load_balancer: 'Load Balancer',
    probe_pin: 'Pinned (probe)',
};

// Human-friendly labels for the resolved upstream API type.
const UPSTREAM_API_LABELS: Record<string, string> = {
    openai_chat: 'Chat Completions',
    openai_responses: 'Responses',
    anthropic_v1: 'Messages',
    anthropic_beta: 'Messages (beta)',
    google: 'GenerateContent',
};

const defaultMessage = (mode: ProbeTestMode): string =>
    mode === 'tool'
        ? "Please use the bash tool to list the current directory contents with 'ls -la'."
        : 'Hello, this is a test message. Please respond with a short greeting.';

// extractText pulls the assistant's text out of the raw (JSON-marshaled) SDK
// response so the user sees plain words instead of a serialized object. Returns
// '' when the shape isn't recognized — the caller falls back to raw JSON.
const extractText = (content?: string): string => {
    if (!content) return '';
    try {
        const data = JSON.parse(content);
        if (Array.isArray(data)) {
            // Streaming: concat OpenAI chat deltas and/or Anthropic text deltas.
            let text = '';
            for (const ch of data) {
                text += ch?.choices?.[0]?.delta?.content ?? '';
                text += ch?.delta?.text ?? '';
            }
            return text;
        }
        // OpenAI chat (non-stream)
        if (data?.choices?.[0]?.message?.content) return data.choices[0].message.content;
        // Anthropic messages
        if (Array.isArray(data?.content)) {
            return data.content
                .filter((b: any) => b?.type === 'text')
                .map((b: any) => b.text)
                .join('');
        }
        // OpenAI Responses
        if (Array.isArray(data?.output)) {
            let t = '';
            for (const o of data.output) {
                if (Array.isArray(o?.content)) {
                    t += o.content
                        .filter((c: any) => c?.text)
                        .map((c: any) => c.text)
                        .join('');
                }
            }
            return t;
        }
    } catch {
        // not JSON — fall through
    }
    return '';
};

// ── Sub-components ──────────────────────────────────────────────────────────

const JourneyRow = memo(({ label, value, muted }: { label: string; value: React.ReactNode; muted?: boolean }) => {
    const theme = useTheme();
    return (
        <Box sx={{ display: 'flex', alignItems: 'baseline', py: 0.75, borderBottom: `1px solid ${theme.palette.divider}` }}>
            <Typography sx={{ width: 92, flexShrink: 0, color: 'text.secondary', fontSize: '0.78rem' }}>
                {label}
            </Typography>
            <Box
                sx={{
                    flex: 1,
                    minWidth: 0,
                    fontFamily: 'monospace',
                    fontSize: '0.78rem',
                    color: muted ? 'text.disabled' : 'text.primary',
                    wordBreak: 'break-all',
                }}
            >
                {value}
            </Box>
        </Box>
    );
});

// CollapsibleSection: a section with title and expand/collapse functionality
interface CollapsibleSectionProps {
    title: string;
    defaultExpanded?: boolean;
    children: React.ReactNode;
}

const CollapsibleSection = memo(({ title, defaultExpanded = false, children }: CollapsibleSectionProps) => {
    const [expanded, setExpanded] = useState(defaultExpanded);
    const theme = useTheme();

    return (
        <Box
            sx={{
                mt: 2,
                border: `1px solid ${theme.palette.divider}`,
                borderRadius: 1.5,
                overflow: 'hidden',
            }}
        >
            <Box
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    px: 1.5,
                    py: 1,
                    bgcolor: 'action.hover',
                    cursor: 'pointer',
                    '&:hover': {
                        bgcolor: 'action.selected',
                    },
                }}
                onClick={() => setExpanded(!expanded)}
            >
                <Typography variant="subtitle2" fontWeight={600} color="text.primary">
                    {title}
                </Typography>
                {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
            </Box>
            <Collapse in={expanded}>
                <Box sx={{ p: 1.5 }}>{children}</Box>
            </Collapse>
        </Box>
    );
});

// StatusBar: the one-glance verdict — success/failure, latency, tokens.
const StatusBar = memo(({ result }: { result: ProbeResult }) => {
    const theme = useTheme();
    const { t } = useTranslation();
    const ok = result.success;
    const d = result.data;
    return (
        <Alert
            severity={ok ? 'success' : 'error'}
            variant="outlined"
            sx={{
                mt: 2,
                borderRadius: 2,
                borderWidth: 2,
                '& .MuiAlert-icon': {
                    fontSize: 28,
                },
            }}
            icon={ok ? <CheckIcon sx={{ fontSize: 28 }} /> : <ErrorIcon sx={{ fontSize: 28 }} />}
        >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, flexWrap: 'wrap' }}>
                <Typography variant="subtitle1" fontWeight={700} sx={{ fontSize: '1rem' }}>
                    {ok ? t('probe.success') : t('probe.failed')}
                </Typography>
                {d?.latency_ms ? (
                    <Chip
                        icon={<SpeedIcon sx={{ fontSize: 16 }} />}
                        label={`${d.latency_ms}ms`}
                        size="medium"
                        sx={{
                            height: 28,
                            bgcolor: ok ? 'success.main' : 'error.main',
                            color: 'common.white',
                            '& .MuiChip-icon': {
                                color: 'common.white',
                            },
                        }}
                    />
                ) : null}
                {d?.total_tokens ? (
                    <Chip
                        icon={<TokenIcon sx={{ fontSize: 16 }} />}
                        label={`${d.total_tokens} tokens`}
                        size="medium"
                        sx={{
                            height: 28,
                            bgcolor: ok ? 'success.main' : 'error.main',
                            color: 'common.white',
                            '& .MuiChip-icon': {
                                color: 'common.white',
                            },
                        }}
                    />
                ) : null}
            </Box>
            {!ok && result.error && (
                <Typography
                    variant="body2"
                    sx={{
                        fontFamily: 'monospace',
                        fontSize: '0.85rem',
                        mt: 1,
                        color: 'text.primary',
                        wordBreak: 'break-word',
                    }}
                >
                    {result.error.message}
                </Typography>
            )}
        </Alert>
    );
});

// Journey: the request's path through TB — rule, routing, provider, endpoint.
// Fields the backend doesn't yet bubble up render as greyed placeholders.
const Journey = memo(
    ({
        result,
        targetType,
        targetName,
        scenario,
        model,
        bypassed,
    }: {
        result: ProbeResult;
        targetType: ProbeTargetType;
        targetName: string;
        scenario?: string;
        model?: string;
        bypassed: boolean;
    }) => {
        const { t } = useTranslation();
        const d = result.data;
        const isRule = targetType === 'rule';
        const provider = d?.selected_provider || (isRule ? '' : targetName);
        const routedModel = d?.selected_model || model || '';
        const ruleLabel = d?.matched_rule_desc || targetName;
        const endpoint = d?.upstream_api ? UPSTREAM_API_LABELS[d.upstream_api] || d.upstream_api : '';
        const pending = t('probe.pending');

        return (
            <Box>
                {isRule && (
                    <JourneyRow label={t('probe.row.rule')} value={`${ruleLabel}${scenario ? `  ·  ${scenario}` : ''}`} />
                )}
                {isRule && (
                    <JourneyRow
                        label={t('probe.row.flags')}
                        value={d?.applied_flags || t('probe.flagsNone')}
                        muted={!d?.applied_flags}
                    />
                )}
                {bypassed ? (
                    <JourneyRow label={t('probe.scope')} value={t('probe.directValue')} />
                ) : (
                    <JourneyRow
                        label={t('probe.row.routing')}
                        value={
                            d?.routing_source
                                ? `${ROUTING_SOURCE_LABELS[d.routing_source] || d.routing_source}${
                                      d.matched_smart_rule !== undefined && d.matched_smart_rule >= 0
                                          ? `  ·  smart rule #${d.matched_smart_rule}`
                                          : ''
                                  }`
                                : pending
                        }
                        muted={!d?.routing_source}
                    />
                )}
                <JourneyRow
                    label={t('probe.row.provider')}
                    value={provider ? `${provider}${routedModel ? `  →  ${routedModel}` : ''}` : pending}
                    muted={!provider}
                />
                <JourneyRow label={t('probe.row.endpoint')} value={endpoint || pending} muted={!endpoint} />
                <JourneyRow label={t('probe.row.upstreamUrl')} value={d?.upstream_url || pending} muted={!d?.upstream_url} />
                {d?.request_url && <JourneyRow label={t('probe.row.requestUrl')} value={d.request_url} />}
            </Box>
        );
    }
);

// ── Main dialog ──────────────────────────────────────────────────────────

export const ProbeDialog: React.FC<ProbeDialogProps> = ({
    open,
    onClose,
    targetType,
    targetId,
    targetName,
    scenario,
    model,
    testMode = 'streaming',
    initialResult,
}) => {
    const { t } = useTranslation();
    const [mode, setMode] = useState<ProbeTestMode>(testMode);
    // 范围: false = 经过 TB (default), true = 直连上游. Provider targets only.
    const [direct, setDirect] = useState(false);
    const [isLoading, setIsLoading] = useState(false);
    const [result, setResult] = useState<ProbeResult | null>(null);
    const [copyTooltipOpen, setCopyTooltipOpen] = useState(false);

    // Reset on open — do NOT auto-run; the user clicks 运行测试. When a quick
    // test already produced a result, show it instead of the empty hint.
    useEffect(() => {
        if (open) {
            setMode(testMode);
            setDirect(false);
            setResult(initialResult ?? null);
            setIsLoading(false);
        }
    }, [open, testMode, initialResult]);

    const runTest = useCallback(async () => {
        setIsLoading(true);
        setResult(null);

        const body = {
            target_type: targetType,
            ...(targetType === 'rule'
                ? { scenario: scenario || 'openai', rule_uuid: targetId }
                : { provider_uuid: targetId, model: model || '', direct, ...(scenario ? { scenario } : {}) }),
            test_mode: mode,
            message: defaultMessage(mode),
        };

        setResult(await runProbe(body));
        setIsLoading(false);
    }, [targetType, scenario, targetId, model, direct, mode]);

    const handleCopy = () => {
        if (!result) return;
        navigator.clipboard.writeText(JSON.stringify(result, null, 2)).then(() => {
            setCopyTooltipOpen(true);
            setTimeout(() => setCopyTooltipOpen(false), 2000);
        });
    };

    const bypassed = targetType === 'provider' && direct;
    const extracted = useMemo(() => extractText(result?.data?.content), [result?.data?.content]);

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth PaperProps={{ sx: { minHeight: 420 } }}>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap', minWidth: 0, overflow: 'hidden' }}>
                    <Typography
                        variant="subtitle1"
                        fontWeight={600}
                        sx={{
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                        }}
                    >
                        {model ? `${targetName} · ${model}` : targetName}
                    </Typography>
                </Box>
                <Box sx={{ display: 'flex', gap: 0.5, alignItems: 'center' }}>
                    {import.meta.env.DEV && (
                        <>
                            <Tooltip title="Simulate Success">
                                <IconButton
                                    size="small"
                                    onClick={() => {
                                        setResult({
                                            success: true,
                                            data: {
                                                content: 'Simulated success response for demo purposes',
                                                latency_ms: 450,
                                                request_url: 'https://api.example.com/v1/chat',
                                                stream: mode === 'streaming',
                                                prompt_tokens: 25,
                                                completion_tokens: 18,
                                                total_tokens: 43,
                                                selected_provider: targetName,
                                                selected_model: model || 'claude-sonnet-4-20250514',
                                                routing_source: 'smart_routing',
                                                matched_smart_rule: 1,
                                                upstream_api: 'anthropic_v1',
                                                upstream_url: 'https://api.anthropic.com/v1/messages',
                                                matched_rule: 'test-rule',
                                                matched_rule_desc: 'Test Rule Description',
                                                applied_flags: 'stream,bypass_cache',
                                            },
                                        });
                                        setIsLoading(false);
                                    }}
                                    sx={{ color: 'success.main' }}
                                >
                                    <CheckIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                            <Tooltip title="Simulate Failure">
                                <IconButton
                                    size="small"
                                    onClick={() => {
                                        setResult({
                                            success: false,
                                            error: {
                                                message: 'Simulated error for demo purposes: Connection timeout',
                                                type: 'upstream_error',
                                            },
                                        });
                                        setIsLoading(false);
                                    }}
                                    sx={{ color: 'error.main' }}
                                >
                                    <ErrorIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                        </>
                    )}
                    {result && (
                        <>
                            <Tooltip
                                title={copyTooltipOpen ? t('probe.copied') : t('probe.copyResponse')}
                                open={copyTooltipOpen || undefined}
                                disableHoverListener={copyTooltipOpen}
                            >
                                <IconButton onClick={handleCopy} size="small" sx={{ color: 'text.secondary' }}>
                                    <CopyIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                            <Tooltip title={t('probe.rerun')}>
                                <IconButton onClick={runTest} size="small" sx={{ color: 'text.secondary' }} disabled={isLoading}>
                                    <RefreshIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                        </>
                    )}
                    <Button
                        variant="contained"
                        size="small"
                        startIcon={isLoading ? null : <RunIcon fontSize="small" />}
                        onClick={runTest}
                        disabled={isLoading}
                        sx={{ minWidth: 100 }}
                    >
                        {isLoading ? t('probe.running') : t('probe.run')}
                    </Button>
                </Box>
            </DialogTitle>

            <DialogContent>
                {/* Controls: request type + scope */}
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 3, flexWrap: 'wrap', mb: 1 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Typography variant="caption" color="text.secondary">
                            {t('probe.shape')}
                        </Typography>
                        <ToggleButtonGroup
                            size="small"
                            exclusive
                            value={mode}
                            onChange={(_, v) => v && setMode(v)}
                            sx={toggleButtonGroupStyle}
                        >
                            {MODES.map((m) => (
                                <ToggleButton key={m.value} value={m.value}>
                                    {t(m.tKey)}
                                </ToggleButton>
                            ))}
                        </ToggleButtonGroup>
                    </Box>

                    {targetType === 'provider' && (
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <Tooltip title={t('probe.scopeHint')}>
                                <Typography variant="caption" color="text.secondary">
                                    {t('probe.scope')}
                                </Typography>
                            </Tooltip>
                            <ToggleButtonGroup
                                size="small"
                                exclusive
                                value={direct ? 'direct' : 'tb'}
                                onChange={(_, v) => v && setDirect(v === 'direct')}
                                sx={toggleButtonGroupStyle}
                            >
                                <ToggleButton value="tb">{t('probe.throughTB')}</ToggleButton>
                                <ToggleButton value="direct">{t('probe.direct')}</ToggleButton>
                            </ToggleButtonGroup>
                        </Box>
                    )}
                </Box>

                {isLoading && <LinearProgress sx={{ height: 6, borderRadius: 3, mt: 1 }} />}

                {!isLoading && !result && (
                    <Box sx={{ textAlign: 'center', py: 8 }}>
                        <Typography variant="body2" color="text.secondary">
                            {t('probe.runHint')}
                        </Typography>
                    </Box>
                )}

                {!isLoading && result && (
                    <Box>
                        <StatusBar result={result} />

                        <CollapsibleSection title={t('probe.journey')} defaultExpanded={false}>
                            <Journey
                                result={result}
                                targetType={targetType}
                                targetName={targetName}
                                scenario={scenario}
                                model={model}
                                bypassed={bypassed}
                            />
                        </CollapsibleSection>

                        {result.success && (
                            <CollapsibleSection title={t('probe.response')} defaultExpanded={false}>
                                <Box
                                    sx={{
                                        p: 1.5,
                                        bgcolor: 'grey.50',
                                        borderRadius: 1.5,
                                        fontFamily: 'monospace',
                                        fontSize: '0.8rem',
                                        whiteSpace: 'pre-wrap',
                                        wordBreak: 'break-word',
                                        maxHeight: 180,
                                        overflow: 'auto',
                                    }}
                                >
                                    {extracted || t('probe.noText')}
                                </Box>
                            </CollapsibleSection>
                        )}

                        {result.success && result.data?.content && (
                            <CollapsibleSection title={t('probe.rawJson')} defaultExpanded={false}>
                                <Box
                                    sx={{
                                        p: 1.5,
                                        bgcolor: 'grey.50',
                                        borderRadius: 1.5,
                                        fontFamily: 'monospace',
                                        fontSize: '0.72rem',
                                        whiteSpace: 'pre-wrap',
                                        wordBreak: 'break-all',
                                        maxHeight: 240,
                                        overflow: 'auto',
                                    }}
                                >
                                    {result.data.content}
                                </Box>
                            </CollapsibleSection>
                        )}
                    </Box>
                )}
            </DialogContent>
        </Dialog>
    );
};

export default ProbeDialog;
