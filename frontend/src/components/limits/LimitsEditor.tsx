import {Box, MenuItem, Stack, TextField, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import type {components} from '@/client';
import AmountField from './AmountField';

export type Limits = components['schemas']['Limits'];
export type LimitStatusEntry = components['schemas']['LimitStatusEntry'];

const WINDOW_OPTIONS = [
    {seconds: 3600, key: 'window1h'},
    {seconds: 5 * 3600, key: 'window5h'},
    {seconds: 24 * 3600, key: 'window24h'},
    {seconds: 7 * 24 * 3600, key: 'window7d'},
] as const;

const DEFAULT_WINDOW_SECONDS = 3600;

interface LimitsEditorProps {
    value?: Limits;
    onChange: (next: Limits) => void;
    /** Provider-level only: stop at an upstream quota percentage. */
    showQuota?: boolean;
    /** Steering threshold; meaningless for a person/team (nothing to steer to). Default true. */
    showSoft?: boolean;
    disabled?: boolean;
}

/** Empty string <-> unset; anything else is clamped to a non-negative integer. */
function parseCount(raw: string): number | undefined {
    const n = Math.floor(Number(raw));
    return raw.trim() === '' || !Number.isFinite(n) || n <= 0 ? undefined : n;
}

function numField(v?: number): string {
    return v && v > 0 ? String(v) : '';
}

/** Compact editor for ai.Limits (concurrency cap, usage budget per window, steering thresholds). */
export default function LimitsEditor({value, onChange, showQuota = false, showSoft = true, disabled}: LimitsEditorProps) {
    const {t} = useTranslation();
    const v = value ?? {};
    const hasBudget = !!(v.max_requests || v.max_tokens);
    const set = (patch: Partial<Limits>) => onChange({...v, ...patch});
    const windowSeconds = v.window_seconds || DEFAULT_WINDOW_SECONDS;
    const windowOptions = WINDOW_OPTIONS.some((o) => o.seconds === windowSeconds)
        ? WINDOW_OPTIONS.map((o) => ({seconds: o.seconds, label: t(`limits.${o.key}`)}))
        : [...WINDOW_OPTIONS.map((o) => ({seconds: o.seconds, label: t(`limits.${o.key}`)})),
            {seconds: windowSeconds, label: `${windowSeconds}s`}];

    return (
        <Stack spacing={2} data-testid="limits-editor">
            <TextField
                size="small" type="number" disabled={disabled}
                label={t('limits.maxConcurrent')} helperText={t('limits.maxConcurrentHint')}
                value={numField(v.max_concurrent)}
                onChange={(e) => set({max_concurrent: parseCount(e.target.value)})}
                slotProps={{htmlInput: {min: 1, 'aria-label': t('limits.maxConcurrent')}}}
            />
            <Box sx={{display: 'grid', gridTemplateColumns: {xs: '1fr', sm: '1fr 1fr 1fr'}, gap: 1.5}}>
                <TextField
                    select size="small" disabled={disabled || !hasBudget}
                    label={t('limits.window')} value={windowSeconds}
                    onChange={(e) => set({window_seconds: Number(e.target.value)})}
                >
                    {windowOptions.map((o) => <MenuItem key={o.seconds} value={o.seconds}>{o.label}</MenuItem>)}
                </TextField>
                <TextField
                    size="small" type="number" disabled={disabled}
                    label={t('limits.maxRequests')} value={numField(v.max_requests)}
                    onChange={(e) => set({max_requests: parseCount(e.target.value)})}
                    slotProps={{htmlInput: {min: 1, 'aria-label': t('limits.maxRequests')}}}
                />
                <AmountField
                    disabled={disabled} label={t('limits.maxTokens')}
                    value={v.max_tokens} onChange={(n) => set({max_tokens: n})}
                />
            </Box>
            <Typography variant="caption" color="text.secondary" sx={{mt: -1}}>{t('limits.maxTokensHint')}</Typography>
            {showSoft && (
                <TextField
                    size="small" type="number" disabled={disabled}
                    label={t('limits.softPercent')} helperText={t('limits.softPercentHint')}
                    value={numField(v.soft_percent)}
                    onChange={(e) => {
                        const n = parseCount(e.target.value);
                        set({soft_percent: n && n < 100 ? n : undefined});
                    }}
                    slotProps={{htmlInput: {min: 1, max: 99, 'aria-label': t('limits.softPercent')}}}
                />
            )}
            {showQuota && (
                <TextField
                    size="small" type="number" disabled={disabled}
                    label={t('limits.quotaPercent')} helperText={t('limits.quotaPercentHint')}
                    value={numField(v.quota_percent)}
                    onChange={(e) => {
                        const n = parseCount(e.target.value);
                        set({quota_percent: n ? Math.min(n, 100) : undefined});
                    }}
                    slotProps={{htmlInput: {min: 1, max: 100, 'aria-label': t('limits.quotaPercent')}}}
                />
            )}
        </Stack>
    );
}

/** True when no limit is configured. */
export function isEmptyLimits(l?: Limits): boolean {
    return !l || !(l.max_concurrent || l.max_requests || l.max_tokens || l.soft_percent || l.quota_percent);
}

/** Drops zero/undefined members so an emptied editor serialises as `{}` (= clear on the wire). */
export function compactLimits(l: Limits): Limits {
    const out: Limits = {};
    if (l.max_concurrent) out.max_concurrent = l.max_concurrent;
    if (l.max_requests) out.max_requests = l.max_requests;
    if (l.max_tokens) out.max_tokens = l.max_tokens;
    if ((l.max_requests || l.max_tokens) && l.window_seconds) out.window_seconds = l.window_seconds;
    if (l.soft_percent) out.soft_percent = l.soft_percent;
    if (l.quota_percent) out.quota_percent = l.quota_percent;
    return out;
}
