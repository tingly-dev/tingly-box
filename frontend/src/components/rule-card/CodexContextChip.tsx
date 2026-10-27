import { Box, Button, ButtonBase, Popover, Stack, TextField, Tooltip, Typography } from '@mui/material';
import { alpha, type SxProps, type Theme } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { ExpandMore as ExpandMoreIcon } from '@/components/icons';
import { fontSizes } from '@/theme/fonts';
import { CODEX_CONTEXT, formatTokens, parseTokens } from './codexCompact';
import { useCodexCatalogWindow } from './useCodexCatalogWindow';

interface CodexContextChipProps {
    /** The rule's own window in tokens; 0 = automatic. */
    window: number;
    /** Whether the rule's 1M flag is on: Auto then writes what the model list
     * gives for it (the routed model's larger window, where it has one). */
    context1M: boolean;
    /** The rule's request model, to read its window from Codex's model list. */
    requestModel?: string;
    disabled?: boolean;
    onChange: (window: number) => void;
    buttonSx: SxProps<Theme>;
    highlightedButtonSx: SxProps<Theme>;
}

const PRESETS: readonly number[] = CODEX_CONTEXT.presets;

/**
 * The "Context: Auto" chip on a Codex rule's header: sets the context window
 * Codex is told this model has, for models whose real window Tingly Box cannot
 * know yet. Presets for the common sizes, a box for anything else. Config-only
 * like the Compact chip: it lands in the generated Codex model catalog the next
 * time Auto Config is applied.
 */
export const CodexContextChip: React.FC<CodexContextChipProps> = ({
    window,
    context1M,
    requestModel,
    disabled,
    onChange,
    buttonSx,
    highlightedButtonSx,
}) => {
    const { t } = useTranslation();
    const [anchor, setAnchor] = React.useState<HTMLElement | null>(null);
    const [text, setText] = React.useState('');
    const open = Boolean(anchor);

    const catalog = useCodexCatalogWindow(requestModel, open);
    const auto = context1M ? catalog.on : catalog.off;
    const autoWindow = auto.window;
    const manual = window >= CODEX_CONTEXT.min && window <= CODEX_CONTEXT.max;
    const typed = text.trim() === '' ? undefined : parseTokens(text);
    // Until the model list has been read, Auto has no number to show.
    const autoLabel = catalog.status === 'pending' ? '…' : formatTokens(autoWindow);

    const choose = (next: number) => {
        onChange(next);
        setText('');
    };
    const submitTyped = () => {
        if (typed) choose(typed);
    };

    // Why Auto is what it is, and what can be done about it. The model list
    // never names the upstream, so neither do these.
    const value = formatTokens(autoWindow);
    // A known limit of the route (not the guess for an unknown model).
    const knownMax = auto.source !== 'default' ? auto.max : 0;
    const notes: { id: string; text: string }[] = [];
    if (manual) {
        if (knownMax && window > knownMax) {
            notes.push({ id: 'over', text: t('ruleHeader.context.overCeiling', { value: formatTokens(window), max: formatTokens(knownMax) }) });
        }
    } else if (catalog.status === 'pending') {
        notes.push({ id: 'auto', text: t('ruleHeader.context.autoPending') });
    } else if (catalog.status === 'unavailable') {
        notes.push({ id: 'auto', text: t('ruleHeader.context.autoUnavailable', { value }) });
    } else if (auto.source === 'default') {
        notes.push({ id: 'auto', text: context1M ? t('ruleHeader.context.autoGuess1M') : t('ruleHeader.context.autoDefault', { value }) });
    } else {
        notes.push({ id: 'auto', text: auto.source === 'extended' ? t('ruleHeader.context.auto1M', { value }) : t('ruleHeader.context.autoCatalog', { value }) });
        // A larger window exists, and the 1M flag is what opens it (Auto then
        // keeps following the upstream, unlike a pinned number).
        if (!context1M && catalog.on.source !== 'default' && catalog.on.window > autoWindow) {
            notes.push({ id: 'max', text: t('ruleHeader.context.ceiling', { value: formatTokens(catalog.on.window) }) });
        }
    }
    notes.push({ id: 'apply', text: t('ruleHeader.compact.needsApply') });

    const label = manual ? formatTokens(window) : t('ruleHeader.context.auto');

    const cellSx = (selected: boolean) => (theme: Theme) => ({
        height: 40,
        borderRadius: 1.5,
        border: '1px solid',
        borderColor: selected ? alpha(theme.palette.primary.main, 0.6) : theme.palette.divider,
        bgcolor: selected ? alpha(theme.palette.primary.main, 0.08) : 'transparent',
        color: selected ? 'primary.main' : 'text.primary',
        flexDirection: 'column' as const,
        gap: 0.25,
        fontVariantNumeric: 'tabular-nums',
        '&:hover': { borderColor: selected ? alpha(theme.palette.primary.main, 0.8) : theme.palette.text.secondary },
        '&.Mui-focusVisible': { outline: `2px solid ${theme.palette.primary.main}`, outlineOffset: 1 },
    });

    return (
        <Box sx={{ ml: 0.5 }} onClick={(e) => e.stopPropagation()}>
            <Tooltip title={t('ruleHeader.context.tooltip')}>
                <span>
                    <Button
                        size="small"
                        variant="outlined"
                        color={manual ? 'primary' : 'inherit'}
                        endIcon={<ExpandMoreIcon sx={{ fontSize: 14 }} />}
                        disabled={disabled}
                        aria-label={t('ruleHeader.context.aria', {
                            value: manual
                                ? formatTokens(window)
                                : catalog.status === 'known' ? t('ruleHeader.context.autoValue', { value: autoLabel }) : t('ruleHeader.context.auto'),
                        })}
                        aria-haspopup="dialog"
                        aria-expanded={open}
                        onClick={(e) => setAnchor(e.currentTarget)}
                        sx={manual ? highlightedButtonSx : buttonSx}
                    >
                        {t('ruleHeader.context.chip', { value: label })}
                    </Button>
                </span>
            </Tooltip>
            <Popover
                open={open}
                anchorEl={anchor}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
                onClose={() => { setAnchor(null); setText(''); }}
                onClick={(e) => e.stopPropagation()}
                slotProps={{ paper: { sx: { width: 340, p: 2, borderRadius: 2 } } }}
            >
                <Stack spacing={1.5}>
                    <Stack direction="row" sx={{ alignItems: 'baseline', justifyContent: 'space-between' }}>
                        <Typography sx={{ fontSize: fontSizes.md, fontWeight: 600 }}>{t('ruleHeader.context.title')}</Typography>
                        <Typography sx={{ fontSize: fontSizes.lg, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>
                            {manual ? formatTokens(window) : autoLabel}
                        </Typography>
                    </Stack>
                    <Typography variant="body2" sx={{ color: 'text.secondary', fontSize: fontSizes.sm }}>
                        {t('ruleHeader.context.description')}
                    </Typography>

                    <Box role="radiogroup" aria-label={t('ruleHeader.context.title')} sx={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 0.75 }}>
                        <ButtonBase role="radio" aria-checked={!manual} disabled={disabled} onClick={() => choose(0)} sx={cellSx(!manual)}>
                            <Box component="span" sx={{ fontSize: fontSizes.sm, fontWeight: 600 }}>{t('ruleHeader.context.auto')}</Box>
                            <Box component="span" sx={{ fontSize: fontSizes.xs, color: 'text.secondary' }}>{autoLabel}</Box>
                        </ButtonBase>
                        {PRESETS.map((n) => (
                            <ButtonBase key={n} role="radio" aria-checked={manual && window === n} disabled={disabled} onClick={() => choose(n)} sx={cellSx(manual && window === n)}>
                                <Box component="span" sx={{ fontSize: fontSizes.sm, fontWeight: 600 }}>{formatTokens(n)}</Box>
                            </ButtonBase>
                        ))}
                    </Box>

                    <Box>
                        <Stack direction="row" spacing={1} sx={{ alignItems: 'flex-start' }}>
                            <TextField
                                size="small"
                                fullWidth
                                value={text}
                                disabled={disabled}
                                onChange={(e) => setText(e.target.value)}
                                onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); submitTyped(); } }}
                                placeholder={manual && !PRESETS.includes(window) ? window.toLocaleString('en-US') : t('ruleHeader.context.customPlaceholder')}
                                error={typed === null}
                                slotProps={{ htmlInput: { 'aria-label': t('ruleHeader.context.custom'), sx: { fontVariantNumeric: 'tabular-nums' } } }}
                            />
                            <Button variant="outlined" size="small" disabled={disabled || !typed} onClick={submitTyped} sx={{ height: 40, flexShrink: 0, textTransform: 'none' }}>
                                {t('ruleHeader.context.set')}
                            </Button>
                        </Stack>
                        <Typography sx={{ mt: 0.5, minHeight: 18, fontSize: fontSizes.xs, color: typed === null ? 'error.main' : 'text.secondary', fontVariantNumeric: 'tabular-nums' }}>
                            {typed === null
                                ? t('ruleHeader.context.invalid', { min: formatTokens(CODEX_CONTEXT.min), max: formatTokens(CODEX_CONTEXT.max) })
                                : typed
                                    ? t('ruleHeader.context.equals', { tokens: typed.toLocaleString('en-US') })
                                    : manual && !PRESETS.includes(window)
                                        ? t('ruleHeader.context.customCurrent', { tokens: window.toLocaleString('en-US') })
                                        : t('ruleHeader.context.customHint')}
                        </Typography>
                    </Box>

                    <Stack spacing={0.5} sx={{ pt: 1, borderTop: 1, borderColor: 'divider' }}>
                        {notes.map((note) => (
                            <Typography
                                key={note.id}
                                data-note={note.id}
                                sx={{
                                    fontSize: fontSizes.xs,
                                    color: (theme) => note.id !== 'over' ? theme.palette.text.secondary : theme.palette.mode === 'dark' ? theme.palette.warning.light : theme.palette.warning.dark,
                                    overflowWrap: 'anywhere',
                                }}
                            >
                                {note.text}
                            </Typography>
                        ))}
                    </Stack>
                </Stack>
            </Popover>
        </Box>
    );
};

export default CodexContextChip;
