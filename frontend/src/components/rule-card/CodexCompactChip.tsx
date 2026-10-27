import { Box, Button, Link, Popover, Slider, Stack, Tooltip, Typography } from '@mui/material';
import { alpha, type SxProps, type Theme } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { ExpandMore as ExpandMoreIcon } from '@/components/icons';
import { api } from '@/services/api';
import { fontSizes } from '@/theme/fonts';
import {
    CODEX_COMPACT,
    compactZones,
    formatTokens,
    planCodexCompact,
    resolveCodexWindow,
    trackPosition,
    type CodexCompactStatus,
} from './codexCompact';
import { useCodexCatalogWindow } from './useCodexCatalogWindow';

interface CodexCompactChipProps {
    /** The rule's own percent; 0 = follow the Auto Config default. */
    percent: number;
    /** The rule's own context window (0 = automatic) and 1M flag; with what
     * the model list says Auto writes they give the window this model has
     * (resolveCodexWindow). */
    contextWindow: number;
    context1M: boolean;
    requestModel?: string;
    disabled?: boolean;
    onChange: (percent: number) => void;
    buttonSx: SxProps<Theme>;
    highlightedButtonSx: SxProps<Theme>;
}

const fmt = (n: number) => n.toLocaleString('en-US');

// What Auto Config currently holds (the shared default every rule without its
// own value follows). Read lazily, when the popover opens.
interface SharedDefaults {
    percent: number;
    effective: number;
}

const parsePercent = (raw: unknown, fallback: number): number => {
    const n = Number.parseInt(String(raw ?? ''), 10);
    return Number.isFinite(n) && n >= CODEX_COMPACT.min && n <= 100 ? n : fallback;
};

// The capped stretch is hatched: values there still work, they are only cut
// down, so it reads as "not this far" rather than as a warning.
const stripes = (theme: Theme) => {
    const line = alpha(theme.palette.text.primary, theme.palette.mode === 'dark' ? 0.28 : 0.2);
    return `repeating-linear-gradient(135deg, ${line} 0 2px, transparent 2px 5px)`;
};

const swatchSx = { width: 18, height: 8, borderRadius: 4, flexShrink: 0 } as const;

const NOTE_COLOR: Record<Exclude<CodexCompactStatus, 'ok'>, 'warning' | 'info'> = {
    low: 'warning',
    limited: 'info',
};

/**
 * The "Compact: 75%" chip on a Codex rule's header: opens a popover with a
 * slider for the share of the context window at which Codex auto-compacts this
 * model. Config-only — it lands in the generated Codex model catalog the next
 * time Auto Config is applied and never touches request routing.
 */
export const CodexCompactChip: React.FC<CodexCompactChipProps> = ({
    percent,
    contextWindow,
    context1M,
    requestModel,
    disabled,
    onChange,
    buttonSx,
    highlightedButtonSx,
}) => {
    const { t } = useTranslation();
    const [anchor, setAnchor] = React.useState<HTMLElement | null>(null);
    const [draft, setDraft] = React.useState<number | null>(null);
    const [shared, setShared] = React.useState<SharedDefaults>({
        percent: CODEX_COMPACT.defaultPercent,
        effective: CODEX_COMPACT.defaultEffectivePercent,
    });

    const open = Boolean(anchor);
    const overridden = percent > 0;
    const shown = draft ?? (overridden ? percent : shared.percent);
    const catalog = useCodexCatalogWindow(requestModel, open);
    const resolvedWindow = resolveCodexWindow(contextWindow, context1M ? catalog.on : catalog.off);
    const window = resolvedWindow.window;
    // Until the model list has said what Auto is (or when it can't be read),
    // token counts would be a guess: show the percent alone and no "too few
    // tokens" warning.
    const windowPending = catalog.status !== 'known' && resolvedWindow.source !== 'rule';
    const plan = planCodexCompact({ window, percent: shown, effectivePercent: shared.effective });
    const allZones = compactZones(window, shared.effective);
    const zones = windowPending ? { ...allZones, low: null } : allZones;
    const noteKind = plan.status === 'ok' || (windowPending && plan.status === 'low') ? null : NOTE_COLOR[plan.status];

    React.useEffect(() => {
        if (!open) return;
        let cancelled = false;
        (async () => {
            try {
                const result = await api.getAppliedCodexConfig();
                const prefs = result?.preferences ?? {};
                if (!cancelled) {
                    setShared({
                        percent: parsePercent(prefs.auto_compact_percent, CODEX_COMPACT.defaultPercent),
                        effective: parsePercent(prefs.effective_context_window_percent, CODEX_COMPACT.defaultEffectivePercent),
                    });
                }
            } catch {
                // Keep the built-in defaults; the preview is only a hint.
            }
        })();
        return () => { cancelled = true; };
    }, [open]);

    // Arrow keys commit on every press; wait until the value settles so one
    // adjustment is one save.
    const pending = React.useRef<{ timer: ReturnType<typeof setTimeout>; value: number } | null>(null);
    const flush = React.useCallback(() => {
        if (!pending.current) return;
        clearTimeout(pending.current.timer);
        const { value } = pending.current;
        pending.current = null;
        onChange(value);
        setDraft(null);
    }, [onChange]);
    const commit = (value: number) => {
        if (pending.current) clearTimeout(pending.current.timer);
        pending.current = { value, timer: setTimeout(flush, 500) };
    };
    // A value still waiting out its debounce is saved, not dropped, if the card
    // goes away first.
    const flushRef = React.useRef(flush);
    flushRef.current = flush;
    React.useEffect(() => () => flushRef.current(), []);

    const close = () => {
        flush();
        setAnchor(null);
        setDraft(null);
    };

    const railBackground = (theme: Theme) => {
        const base = theme.palette.action.disabledBackground;
        const layers: string[] = [];
        const sizes: string[] = [];
        if (zones.cappedFrom !== null) {
            const at = trackPosition(zones.cappedFrom);
            layers.push(stripes(theme));
            sizes.push(`${100 - at}% 100%`);
        }
        const stops = [`${base} 0%`];
        if (zones.low) {
            const from = trackPosition(zones.low.from);
            const to = trackPosition(zones.low.to);
            const warn = alpha(theme.palette.warning.main, 0.55);
            stops.push(`${base} ${from}%`, `${warn} ${from}%`, `${warn} ${to}%`, `${base} ${to}%`);
        }
        stops.push(`${base} 100%`);
        layers.push(`linear-gradient(to right, ${stops.join(', ')})`);
        sizes.push('100% 100%');
        return {
            backgroundImage: layers.join(', '),
            backgroundSize: sizes.join(', '),
            backgroundPosition: 'right center',
            backgroundRepeat: 'no-repeat',
        };
    };

    const noteText = {
        low: t('ruleHeader.compact.low', { remain: fmt(plan.remain), min: fmt(CODEX_COMPACT.minRemaining) }),
        limited: t('ruleHeader.compact.limited', { effective: shared.effective, applied: plan.applied }),
        ok: '',
    }[plan.status];

    return (
        <Box sx={{ ml: 0.5 }} onClick={(e) => e.stopPropagation()}>
            <Tooltip title={t('ruleHeader.compact.tooltip')}>
                <span>
                    <Button
                        size="small"
                        variant="outlined"
                        color={overridden ? 'primary' : 'inherit'}
                        endIcon={<ExpandMoreIcon sx={{ fontSize: 14 }} />}
                        disabled={disabled}
                        aria-label={t('ruleHeader.compact.aria', { value: overridden ? `${percent}%` : t('ruleHeader.compact.auto') })}
                        aria-haspopup="dialog"
                        aria-expanded={open}
                        onClick={(e) => setAnchor(e.currentTarget)}
                        sx={overridden ? highlightedButtonSx : buttonSx}
                    >
                        {t('ruleHeader.compact.chip', { value: overridden ? `${percent}%` : t('ruleHeader.compact.auto') })}
                    </Button>
                </span>
            </Tooltip>
            <Popover
                open={open}
                anchorEl={anchor}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
                onClose={close}
                onClick={(e) => e.stopPropagation()}
                slotProps={{ paper: { sx: { width: 340, p: 2, borderRadius: 2 } } }}
            >
                <Stack spacing={1.5}>
                    <Stack direction="row" sx={{ alignItems: 'baseline', justifyContent: 'space-between' }}>
                        <Typography sx={{ fontSize: fontSizes.md, fontWeight: 600 }}>{t('ruleHeader.compact.title')}</Typography>
                        <Typography sx={{ fontSize: fontSizes.lg, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>
                            {shown}%
                        </Typography>
                    </Stack>
                    <Typography variant="body2" sx={{ color: 'text.secondary', fontSize: fontSizes.sm }}>
                        {overridden || draft !== null
                            ? t('ruleHeader.compact.description')
                            : t('ruleHeader.compact.following', { percent: shared.percent })}
                    </Typography>

                    <Box>
                        <Slider
                            size="small"
                            min={CODEX_COMPACT.min}
                            max={CODEX_COMPACT.max}
                            step={1}
                            value={shown}
                            disabled={disabled}
                            onChange={(_, v) => setDraft(v as number)}
                            onChangeCommitted={(_, v) => {
                                const next = v as number;
                                setDraft(next);
                                commit(next);
                            }}
                            aria-label={t('ruleHeader.compact.title')}
                            valueLabelDisplay="off"
                            sx={(theme) => ({
                                display: 'block',
                                py: 1.25,
                                // The zones are painted on the rail itself, so they line up
                                // with the thumb at every width.
                                '& .MuiSlider-rail': { height: 6, opacity: 1, ...railBackground(theme) },
                                '& .MuiSlider-track': { height: 6, border: 0 },
                            })}
                        />
                        <Stack direction="row" sx={{ justifyContent: 'space-between', fontSize: fontSizes.xs, color: 'text.secondary', fontVariantNumeric: 'tabular-nums' }}>
                            <span>{CODEX_COMPACT.min}%</span>
                            <span>100%</span>
                        </Stack>
                        {(zones.low || zones.cappedFrom !== null) && (
                            <Stack spacing={0.5} sx={{ mt: 1, fontSize: fontSizes.xs, color: 'text.secondary', lineHeight: 1.4 }}>
                                {zones.low && (
                                    <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                                        <Box sx={{ ...swatchSx, bgcolor: (theme) => alpha(theme.palette.warning.main, 0.55) }} />
                                        <span>{t('ruleHeader.compact.zoneLow', { from: Math.round(zones.low.from), to: zones.low.to })}</span>
                                    </Stack>
                                )}
                                {zones.cappedFrom !== null && (
                                    <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                                        <Box sx={{ ...swatchSx, background: (theme) => `${stripes(theme)}, ${theme.palette.action.disabledBackground}` }} />
                                        <span>{t('ruleHeader.compact.zoneCapped', { effective: zones.cappedFrom })}</span>
                                    </Stack>
                                )}
                            </Stack>
                        )}
                    </Box>

                    <Stack spacing={0.25} sx={{ fontSize: fontSizes.sm, fontVariantNumeric: 'tabular-nums' }}>
                        {[
                            [t('ruleHeader.compact.window'), windowPending ? '…' : formatTokens(window), undefined],
                            [t('ruleHeader.compact.line'), windowPending ? `${plan.applied}%` : `${plan.applied}% · ${fmt(plan.line)} tokens`, undefined],
                            [t('ruleHeader.compact.remain'), windowPending ? '…' : `${fmt(plan.remain)} tokens`, plan.status === 'low' && !windowPending ? 'warning.dark' : 'text.primary'],
                        ].map(([label, value, color]) => (
                            <Stack key={label} direction="row" sx={{ justifyContent: 'space-between' }}>
                                <Box component="span" sx={{ color: 'text.secondary' }}>{label}</Box>
                                <Box component="span" sx={{ color: color ?? 'text.primary', fontWeight: color ? 600 : 400 }}>{value}</Box>
                            </Stack>
                        ))}
                    </Stack>

                    {noteKind && (
                        <Box sx={{
                            p: 1, borderRadius: 1, fontSize: fontSizes.sm, lineHeight: 1.45,
                            color: (theme) => theme.palette.mode === 'dark' ? theme.palette[noteKind].light : theme.palette[noteKind].dark,
                            bgcolor: (theme) => alpha(theme.palette[noteKind].main, 0.1),
                            border: '1px solid',
                            borderColor: (theme) => alpha(theme.palette[noteKind].main, 0.4),
                        }}>
                            {noteText}
                        </Box>
                    )}

                    <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'center', pt: 1, borderTop: 1, borderColor: 'divider', fontSize: fontSizes.xs, color: 'text.secondary' }}>
                        <span>{t('ruleHeader.compact.needsApply')}</span>
                        {overridden && (
                            <Link
                                component="button"
                                type="button"
                                underline="hover"
                                onClick={() => { if (pending.current) { clearTimeout(pending.current.timer); pending.current = null; } setDraft(null); onChange(0); }}
                                sx={{ fontSize: fontSizes.xs }}
                            >
                                {t('ruleHeader.compact.reset', { percent: shared.percent })}
                            </Link>
                        )}
                    </Stack>
                </Stack>
            </Popover>
        </Box>
    );
};

export default CodexCompactChip;
