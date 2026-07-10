import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Box, CircularProgress, Fade, IconButton, Tooltip, Typography } from '@mui/material';
import { alpha, useTheme } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';
import {
    Bolt as BoltIcon,
    CheckCircle as CheckIcon,
    Close as CloseIcon,
    Error as ErrorIcon,
} from '@/components/icons';
import type { ProbeResult } from '@/types/probe';
import { formatLatency, runProbe } from './runProbe';
import { ProbeDialog } from './ProbeDialog';
import { fontMono } from '@/theme/fonts';

interface QuickProbeButtonProps {
    ruleUuid: string;
    ruleName: string;
    scenario?: string;
    model?: string;
    /** Increment to trigger a run externally (e.g. the page-level "test all"). */
    runSignal?: number;
}

// QuickProbeButton: one-click streaming probe for a rule, living in the rule
// card header. No menu, no dialog up front — click the bolt, get a compact
// verdict pill (status + latency) in place. Clicking the pill opens the full
// ProbeDialog pre-loaded with this result for the journey/response details.
export const QuickProbeButton: React.FC<QuickProbeButtonProps> = ({ ruleUuid, ruleName, scenario, model, runSignal }) => {
    const { t } = useTranslation();
    const theme = useTheme();
    const [running, setRunning] = useState(false);
    const [result, setResult] = useState<ProbeResult | null>(null);
    const [dialogOpen, setDialogOpen] = useState(false);
    const mounted = useRef(true);

    useEffect(() => {
        mounted.current = true;
        return () => {
            mounted.current = false;
        };
    }, []);

    const run = useCallback(async () => {
        setRunning(true);
        setResult(null);
        // Stream by default — closest to production traffic. Message defaults
        // server-side.
        const res = await runProbe({
            target_type: 'rule',
            scenario: scenario || 'openai',
            rule_uuid: ruleUuid,
            test_mode: 'streaming',
        });
        if (!mounted.current) return;
        setResult(res);
        setRunning(false);
    }, [ruleUuid, scenario]);

    // External trigger: run when the signal increments (skipped while already
    // running — the signal is consumed either way so it can't re-fire late).
    const lastSignal = useRef(runSignal ?? 0);
    useEffect(() => {
        if (runSignal === undefined || runSignal <= lastSignal.current) return;
        lastSignal.current = runSignal;
        if (!running) void run();
    }, [runSignal, running, run]);

    const ok = result?.success === true;
    const pillColor = ok ? theme.palette.success.main : theme.palette.error.main;

    return (
        <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.25 }}>
            {result && (
                <Fade in>
                    <Tooltip title={ok ? t('probe.viewDetails') : result.error?.message || t('probe.viewDetails')}>
                        <Box
                            onClick={() => setDialogOpen(true)}
                            sx={{
                                display: 'inline-flex',
                                alignItems: 'center',
                                gap: 0.5,
                                height: 22,
                                pl: 0.75,
                                pr: 0.25,
                                borderRadius: 999,
                                cursor: 'pointer',
                                border: `1px solid ${alpha(pillColor, 0.4)}`,
                                backgroundColor: alpha(pillColor, 0.08),
                                '&:hover': { backgroundColor: alpha(pillColor, 0.16) },
                            }}
                        >
                            {ok ? (
                                <CheckIcon sx={{ fontSize: 14, color: pillColor }} />
                            ) : (
                                <ErrorIcon sx={{ fontSize: 14, color: pillColor }} />
                            )}
                            <Typography
                                sx={{
                                    fontSize: '0.72rem',
                                    fontWeight: 600,
                                    lineHeight: 1,
                                    color: pillColor,
                                    fontFamily: fontMono,
                                    whiteSpace: 'nowrap',
                                }}
                            >
                                {ok && result.data ? formatLatency(result.data.latency_ms) : t('probe.failed')}
                            </Typography>
                            <IconButton
                                size="small"
                                aria-label={t('probe.dismiss')}
                                onClick={(e) => {
                                    e.stopPropagation();
                                    setResult(null);
                                }}
                                sx={{ p: 0.25, color: alpha(pillColor, 0.7), '&:hover': { color: pillColor } }}
                            >
                                <CloseIcon sx={{ fontSize: 12 }} />
                            </IconButton>
                        </Box>
                    </Tooltip>
                </Fade>
            )}

            <Tooltip title={t('probe.quickTest')}>
                <span>
                    <IconButton
                        size="small"
                        onClick={run}
                        disabled={running}
                        sx={{ color: 'text.secondary', '&:hover': { backgroundColor: 'action.hover' } }}
                    >
                        {running ? <CircularProgress size={16} thickness={5} /> : <BoltIcon fontSize="small" />}
                    </IconButton>
                </span>
            </Tooltip>

            <ProbeDialog
                open={dialogOpen}
                onClose={() => setDialogOpen(false)}
                targetType="rule"
                targetId={ruleUuid}
                targetName={ruleName}
                scenario={scenario}
                model={model}
                initialResult={result ?? undefined}
            />
        </Box>
    );
};

export default QuickProbeButton;
