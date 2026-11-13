import { useId, useState } from 'react';
import { Box, ListItemButton, ListItemIcon, Popover, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { ProviderQuotaPanel } from '@/components/ProviderQuotaPanel';
import { QuotaRing } from '@/components/credential/QuotaRing';
import { activityItemSx } from './styles';
import { radius } from '@tingly/ui/theme/radius';

export function QuotaMenu({ onNavigate }: { onNavigate?: () => void }) {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
    const id = useId();
    const open = Boolean(anchorEl);
    const label = 'Quota';

    return (
        // Stretch the native button to the rail width, just like the block
        // links above it; otherwise it shrinks to the short label's width.
        <Box sx={{ display: 'flex', flexDirection: 'column' }}>
            <Tooltip title={label} placement="right" arrow>
                <ListItemButton
                    component="button"
                    type="button"
                    aria-label={label}
                    aria-haspopup="dialog"
                    aria-expanded={open}
                    aria-controls={open ? id : undefined}
                    onClick={event => setAnchorEl(open ? null : event.currentTarget)}
                    sx={activityItemSx({
                        '&:hover': { bgcolor: 'action.hover', color: 'primary.main' },
                        ...(open && { color: 'primary.main', bgcolor: 'action.selected' }),
                    })}
                >
                    <ListItemIcon sx={{ minWidth: 0, width: 22, height: 22, color: 'inherit', alignItems: 'center', justifyContent: 'center' }}>
                        <QuotaRing remaining={100} color="currentColor" size={18} />
                    </ListItemIcon>
                    <Typography variant="caption" sx={{ color: 'inherit', textAlign: 'center', lineHeight: 1.2 }}>
                        {label}
                    </Typography>
                </ListItemButton>
            </Tooltip>
            <Popover
                open={open}
                anchorEl={anchorEl}
                onClose={() => setAnchorEl(null)}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                slotProps={{ paper: {
                    id,
                    role: 'dialog',
                    'aria-label': t('hub.quota.title'),
                    sx: { width: 380, maxWidth: 'calc(100vw - 32px)', borderRadius: radius.card, ml: 0.5 },
                } }}
            >
                {open && (
                    <Box sx={{ display: 'flex', height: 'min(520px, calc(100dvh - 32px))' }}>
                        <ProviderQuotaPanel onNavigate={path => {
                            setAnchorEl(null);
                            navigate(path);
                            onNavigate?.();
                        }} />
                    </Box>
                )}
            </Popover>
        </Box>
    );
}
