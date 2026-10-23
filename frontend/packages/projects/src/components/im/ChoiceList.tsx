import type { ReactNode } from 'react';
import { Box, ButtonBase, Typography } from '@mui/material';
import { fontSizes } from '@tingly/ui/theme/fonts';

export interface Choice {
    key: string;
    /** The row's leading mark (a platform logo). */
    lead: ReactNode;
    title: string;
    subtitle?: string;
    /** A quiet tag at the end of the row, e.g. "Will move". */
    chip?: string;
    disabled?: boolean;
}

/**
 * A radio list where the whole row picks: a mark, a title, one grey line and
 * an optional tag. The bind dialog and the IM settings' move dialog share it.
 */
export function ChoiceList({ items, value, onChange, label }: { items: Choice[]; value: string | null; onChange: (key: string) => void; label: string }) {
    return (
        <Box role="radiogroup" aria-label={label} sx={{ display: 'flex', flexDirection: 'column', gap: 0.75 }}>
            {items.map((c) => {
                const selected = value === c.key;
                return (
                    <ButtonBase
                        key={c.key}
                        role="radio"
                        aria-checked={selected}
                        disabled={c.disabled}
                        onClick={() => onChange(c.key)}
                        sx={{
                            display: 'flex', alignItems: 'center', justifyContent: 'flex-start', gap: 1.25, width: '100%', textAlign: 'left',
                            px: 1.25, py: 1, borderRadius: 1, border: 1, borderColor: selected ? 'primary.main' : 'divider',
                            bgcolor: selected ? 'action.selected' : 'transparent', opacity: c.disabled ? 0.55 : 1,
                            '&:hover': { borderColor: selected ? 'primary.main' : 'text.disabled' },
                            '&.Mui-focusVisible': { outline: 2, outlineColor: 'primary.main', outlineStyle: 'solid', outlineOffset: 1 },
                        }}
                    >
                        <Box aria-hidden="true" sx={{ width: 16, height: 16, borderRadius: '50%', flexShrink: 0, border: 2, borderColor: selected ? 'primary.main' : 'text.disabled', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                            {selected && <Box sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: 'primary.main' }} />}
                        </Box>
                        {c.lead}
                        <Box sx={{ minWidth: 0, flex: 1 }}>
                            <Typography noWrap sx={{ fontWeight: 600, fontSize: '0.875rem', lineHeight: 1.35 }}>{c.title}</Typography>
                            {c.subtitle && <Typography noWrap color="text.secondary" sx={{ fontSize: fontSizes.sm, lineHeight: 1.4 }}>{c.subtitle}</Typography>}
                        </Box>
                        {c.chip && (
                            <Box component="span" sx={{ flexShrink: 0, px: 0.9, py: 0.15, borderRadius: '6px', fontSize: fontSizes.xs, fontWeight: 600, color: 'text.secondary', border: 1, borderColor: 'divider' }}>
                                {c.chip}
                            </Box>
                        )}
                    </ButtonBase>
                );
            })}
        </Box>
    );
}
