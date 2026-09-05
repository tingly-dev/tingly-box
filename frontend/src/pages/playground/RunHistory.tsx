import React from 'react';
import { Box, Chip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { formatLatency } from '@/components/probe/runProbe';
import type { RunRecord } from './playgroundState';

// RunHistory: this session's runs as chips. Clicking one shows that result
// AND restores the configuration that produced it — comparing two
// experiments is the workbench's daily motion ("done" ≠ locked,
// ux-principles #10). In-memory only; cleared on reload (V1).

export const RunHistory: React.FC<{
    runs: RunRecord[];
    activeId: string | null;
    onSelect: (run: RunRecord) => void;
}> = ({ runs, activeId, onSelect }) => {
    const { t } = useTranslation();
    if (runs.length === 0) {
        return (
            <Typography variant="caption" sx={{ color: 'text.disabled' }}>
                {t('playground.runsEmpty', { defaultValue: 'Runs from this session appear here — click one to restore its result and the config that produced it.' })}
            </Typography>
        );
    }
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, overflowX: 'auto', py: 0.25 }}>
            <Typography variant="overline" sx={{ fontSize: '0.6rem', color: 'text.disabled', mr: 0.5, whiteSpace: 'nowrap' }}>
                {t('playground.runs', { defaultValue: 'Runs' })}
            </Typography>
            {runs.map((run) => {
                const ok = run.result.success;
                const ms = run.result.data?.latency_ms;
                const head = ok ? (ms ? formatLatency(ms) : t('probe.success')) : run.result.error?.message?.slice(0, 32) || t('probe.failed');
                return (
                    <Chip
                        key={run.id}
                        size="small"
                        variant={run.id === activeId ? 'filled' : 'outlined'}
                        color={run.id === activeId ? 'primary' : 'default'}
                        onClick={() => onSelect(run)}
                        icon={<Box sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: ok ? 'success.main' : 'error.main', ml: '6px !important' }} />}
                        label={`${head} · ${run.label}`}
                        sx={{ fontFamily: 'monospace', fontSize: '0.7rem', flexShrink: 0 }}
                    />
                );
            })}
        </Box>
    );
};

export default RunHistory;
