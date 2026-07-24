import { Box, ButtonBase, Grid, Typography, alpha } from '@mui/material';
import type { ReactNode } from 'react';

export interface PlatformPickerItem {
    id: string;
    label: string;
    /**
     * Rendered with the tile's selected state. The brand logos default to
     * grayscale (createBrandIcon(..., true)) and carry that filter on the
     * <img> itself — a parent filter can't undo it — so the icon has to be
     * (re)rendered with `grayscale={!active}` to come back to full color
     * when its tile is selected. Hence a render fn, not a static node.
     */
    icon: (active: boolean) => ReactNode;
    subtitle?: string;
}

interface PlatformPickerProps {
    items: PlatformPickerItem[];
    value: string;
    onChange: (id: string) => void;
}

// PlatformPicker is the platform selector for Overview and Remote — a
// responsive grid of equal-size tiles, one per platform, mirroring the
// dashboard's StatCard grid (`Grid` with `size`, uniform `height: 100%`
// cards, tinted-border selected state). Earlier cuts (MUI Tabs, then a row
// of Chips) produced ragged widths — "Lark" tiny next to "WeCom (企业微信)"
// — which read as inconsistent with the rest of the app. Fixed tiles keep
// every option the same shape regardless of label length.
const PlatformPicker: React.FC<PlatformPickerProps> = ({ items, value, onChange }) => (
    <Grid container spacing={1.5} sx={{ mb: 2 }}>
        {items.map((item) => {
            const active = item.id === value;
            return (
                <Grid key={item.id} size={{ xs: 6, sm: 4, md: 3, lg: 2 }}>
                    <ButtonBase
                        onClick={() => onChange(item.id)}
                        aria-current={active ? 'true' : undefined}
                        sx={{
                            width: '100%',
                            height: '100%',
                            justifyContent: 'flex-start',
                            gap: 1.25,
                            p: 1.5,
                            borderRadius: 2,
                            border: '1px solid',
                            borderColor: active ? 'primary.main' : 'divider',
                            bgcolor: active
                                ? (theme) => alpha(theme.palette.primary.main, 0.08)
                                : 'background.paper',
                            transition: 'border-color 0.18s ease-out, background-color 0.18s ease-out',
                            '&:hover': {
                                borderColor: active ? 'primary.main' : 'text.disabled',
                                bgcolor: active
                                    ? (theme) => alpha(theme.palette.primary.main, 0.12)
                                    : 'action.hover',
                            },
                        }}
                    >
                        <Box
                            sx={{
                                width: 28,
                                height: 28,
                                flexShrink: 0,
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                            }}
                        >
                            {item.icon(active)}
                        </Box>
                        <Box sx={{ minWidth: 0, textAlign: 'left' }}>
                            <Typography
                                variant="body2"
                                noWrap
                                sx={{ fontWeight: 600, color: active ? 'primary.main' : 'text.primary' }}
                            >
                                {item.label}
                            </Typography>
                            <Typography variant="caption" noWrap sx={{ display: 'block', color: 'text.secondary' }}>
                                {item.subtitle || '—'}
                            </Typography>
                        </Box>
                    </ButtonBase>
                </Grid>
            );
        })}
    </Grid>
);

export default PlatformPicker;
