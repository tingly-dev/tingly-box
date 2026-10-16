import { useState } from 'react';
import { Box, IconButton, ListSubheader, Menu, MenuItem, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { Check, LayoutIcon } from '../icons';
import type { OverviewPlacement, ThreadPlacement } from '../../hooks/usePlacement';

interface Props {
    overview: OverviewPlacement;
    thread: ThreadPlacement;
    onOverview: (v: OverviewPlacement) => void;
    onThread: (v: ThreadPlacement) => void;
}

/** Two choices, kept apart: where the Overview lives and where an open thread lives. */
export function PlacementMenu({ overview, thread, onOverview, onThread }: Props) {
    const { t } = useTranslation('projects');
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const item = (selected: boolean, label: string, onClick: () => void, disabled = false) => (
        <MenuItem key={label} selected={selected} disabled={disabled} onClick={() => { onClick(); setAnchor(null); }} sx={{ minWidth: 200 }}>
            <Box sx={{ flex: 1 }}>{label}</Box>
            {selected && <Check fontSize="small" color="primary" />}
        </MenuItem>
    );
    const locked = overview === 'top';
    return (
        <>
            <Tooltip title={t('placement.title')}>
                <IconButton size="small" onClick={(e) => setAnchor(e.currentTarget)} aria-label={t('placement.title')} aria-haspopup="menu"><LayoutIcon fontSize="small" /></IconButton>
            </Tooltip>
            <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)} anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }} transformOrigin={{ vertical: 'top', horizontal: 'right' }}>
                <ListSubheader sx={{ lineHeight: '32px', bgcolor: 'transparent' }}>{t('placement.overview')}</ListSubheader>
                {item(overview === 'side', t('placement.side'), () => onOverview('side'))}
                {item(overview === 'window', t('placement.window'), () => onOverview('window'))}
                {item(overview === 'top', t('placement.top'), () => onOverview('top'))}
                <ListSubheader sx={{ lineHeight: '32px', bgcolor: 'transparent' }}>{t('placement.thread')}</ListSubheader>
                {item(!locked && thread === 'side', t('placement.side'), () => onThread('side'), locked)}
                {item(!locked && thread === 'float', t('placement.float'), () => onThread('float'), locked)}
                <Typography sx={{ px: 2, pt: 0.5, pb: 1, fontSize: fontSizes.sm, color: 'text.disabled', maxWidth: 240 }}>
                    {locked ? t('placement.lockedTop') : t('placement.remember')}
                </Typography>
            </Menu>
        </>
    );
}
