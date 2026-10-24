import { Box, Button, ButtonBase, Popover, Stack, TextField, Tooltip, Typography } from '@mui/material';
import { alpha, type SxProps, type Theme } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { ExpandMore as ExpandMoreIcon } from '@/components/icons';
import { fontSizes } from '@/theme/fonts';
import { CODEX_CONTEXT, formatTokens, parseTokens, resolveCodexWindow, resolveCodexWindowSource } from './codexCompact';
import { useCodexCatalogWindow } from './useCodexCatalogWindow';

interface CodexContextChipProps {
    /** The rule's own window in tokens; 0 = automatic. */
    window: number;
    /** Whether the rule's 1M flag is on (the automatic window is then 1M). */
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
    const auto = resolveCodexWindowSource(0, context1M, catalog);
    const autoWindow = auto.window;
    const manual = window >= CODEX_CONTEXT.min && window <= CODEX_CONTEXT.max;
    const shown = resolveCodexWindow(window, context1M, catalog);
    const typed = text.trim() === '' ? undefined : parseTokens(text);

    const choose = (next: number) => {
        onChange(next);
        setText('');
    };
    const submitTyped = () => {
        if (typed) choose(typed);
    };

    const AUTO_NOTE = {
        '1m': () => t('ruleHeader.context.auto1M'),
        catalog: (n: number) => t('ruleHeader.context.autoCatalog', { value: formatTokens(n) }),
        default: () => t('ruleHeader.context.autoDefault'),
        rule: () => '',
    } as const;

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
                        aria-label={t('ruleHeader.context.aria', { value: manual ? formatTokens(window) : t('ruleHeader.context.autoValue', { value: formatTokens(autoWindow) }) })}
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
                            {formatTokens(shown)}
                        </Typography>
                    </Stack>
                    <Typography variant="body2" sx={{ color: 'text.secondary', fontSize: fontSizes.sm }}>
                        {t('ruleHeader.context.description')}
                    </Typography>

                    <Box role="radiogroup" aria-label={t('ruleHeader.context.title')} sx={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 0.75 }}>
                        <ButtonBase role="radio" aria-checked={!manual} disabled={disabled} onClick={() => choose(0)} sx={cellSx(!manual)}>
                            <Box component="span" sx={{ fontSize: fontSizes.sm, fontWeight: 600 }}>{t('ruleHeader.context.auto')}</Box>
                            <Box component="span" sx={{ fontSize: fontSizes.xs, color: 'text.secondary' }}>{formatTokens(autoWindow)}</Box>
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

                    <Typography sx={{ pt: 1, borderTop: 1, borderColor: 'divider', fontSize: fontSizes.xs, color: 'text.secondary' }}>
                        {t('ruleHeader.context.footer', { auto: AUTO_NOTE[auto.source](autoWindow), apply: t('ruleHeader.compact.needsApply') })}
                    </Typography>
                </Stack>
            </Popover>
        </Box>
    );
};

export default CodexContextChip;
