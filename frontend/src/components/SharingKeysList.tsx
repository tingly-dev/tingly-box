import { Box, Chip, IconButton, ListItemIcon, ListItemText, Menu, MenuItem, Switch, Typography } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    CompareArrows as IconMove,
    ContentCopy as IconCopy,
    DeleteOutline as IconDelete,
    MoreHoriz,
    Speed as IconLimits,
    Visibility as IconEye,
    VisibilityOff as IconEyeOff,
} from '@/components/icons';
import type { SharingKey } from '@/components/SharingKeysTable';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { maskToken } from '@/utils/sharingKey';
import { mobile } from '@tingly/ui/theme/mobile';
import { radius } from '@tingly/ui/theme/radius';

// What a key row can do — the same handlers the desktop table takes
// (`useSharingKeyActions` builds them).
export interface SharingKeyActions {
    visibleTokens: Record<string, boolean>;
    onToggleVisibility: (tokenId: string) => void;
    onCopy: (tokenId: string) => void;
    onToggleEnabled: (token: SharingKey) => void;
    onDelete: (token: SharingKey) => void;
    onMove: (token: SharingKey) => void;
    onLimits?: (token: SharingKey) => void;
    limitedUserIds?: ReadonlySet<string>;
}

/**
 * A Team's keys on a phone (`.design/mobile-ui.md`, "Team"): a grouped list
 * instead of the table. A row is a person — name and user, the enable switch,
 * the masked key with reveal and copy — and its "⋯" holds limits, move and
 * delete (which asks first, as on the desktop).
 */
export function SharingKeysList({ tokens, ...actions }: { tokens: SharingKey[] } & SharingKeyActions) {
    const { t } = useTranslation();
    return (
        <Box
            component="ul"
            aria-label={t('sharingKeys.title')}
            sx={{
                listStyle: 'none', m: 0, p: 0,
                border: 1, borderColor: 'divider', borderRadius: radius.row, overflow: 'hidden',
                '& > li:not(:last-child)': { borderBottom: 1, borderColor: 'divider' },
            }}
        >
            {tokens.map((key) => <KeyRow key={key.token_id} sharingKey={key} actions={actions} />)}
        </Box>
    );
}

function KeyRow({ sharingKey: key, actions }: { sharingKey: SharingKey; actions: SharingKeyActions }) {
    const { t } = useTranslation();
    const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
    const visible = !!actions.visibleTokens[key.token_id];
    const close = () => setMenuAnchor(null);
    const then = (fn: () => void) => () => { close(); fn(); };

    return (
        <Box component="li" sx={{ px: 1.5, py: 0.5, minHeight: mobile.rowMin, bgcolor: 'background.paper' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                <Box sx={{ flex: 1, minWidth: 0, opacity: key.enabled ? 1 : 0.55 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
                        <Typography noWrap sx={{ fontWeight: 500, minWidth: 0 }}>{key.display_name}</Typography>
                        {!key.enabled && (
                            <Chip label={t('sharingKeys.table.inactive')} size="small" variant="outlined" sx={{ height: 18, fontSize: fontSizes.micro }} />
                        )}
                    </Box>
                    <Typography variant="caption" noWrap component="p" sx={{ color: 'text.secondary', fontFamily: fontMono }}>
                        {key.user_id}
                    </Typography>
                </Box>
                <Switch
                    color="success"
                    checked={key.enabled}
                    onChange={() => actions.onToggleEnabled(key)}
                    slotProps={{ input: { 'aria-label': `${key.display_name} ${t('sharingKeys.table.status')}` } }}
                />
                <IconButton aria-label={t('sharingKeys.phone.keyActions', { name: key.display_name })} onClick={(e) => setMenuAnchor(e.currentTarget)}>
                    <MoreHoriz />
                </IconButton>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, opacity: key.enabled ? 1 : 0.55 }}>
                <Typography
                    variant="body2"
                    sx={{ flex: 1, minWidth: 0, fontFamily: fontMono, color: 'text.secondary', overflowWrap: 'anywhere' }}
                >
                    {visible ? key.token_id : maskToken(key.token_id)}
                </Typography>
                <IconButton
                    aria-label={visible ? t('sharingKeys.table.hide') : t('sharingKeys.table.show')}
                    onClick={() => actions.onToggleVisibility(key.token_id)}
                >
                    {visible ? <IconEyeOff fontSize="small" /> : <IconEye fontSize="small" />}
                </IconButton>
                <IconButton aria-label={t('sharingKeys.table.copy')} onClick={() => actions.onCopy(key.token_id)}>
                    <IconCopy fontSize="small" />
                </IconButton>
            </Box>
            <Menu anchorEl={menuAnchor} open={!!menuAnchor} onClose={close}>
                {actions.onLimits && (
                    <MenuItem onClick={then(() => actions.onLimits?.(key))}>
                        <ListItemIcon><IconLimits fontSize="small" color={actions.limitedUserIds?.has(key.user_id) ? 'primary' : undefined} /></ListItemIcon>
                        <ListItemText>{t('sharingKeys.table.limitsTooltip')}</ListItemText>
                    </MenuItem>
                )}
                <MenuItem onClick={then(() => actions.onMove(key))}>
                    <ListItemIcon><IconMove fontSize="small" /></ListItemIcon>
                    <ListItemText>{t('sharingKeys.table.moveTooltip')}</ListItemText>
                </MenuItem>
                <MenuItem onClick={then(() => actions.onDelete(key))} sx={{ color: 'error.main' }}>
                    <ListItemIcon><IconDelete fontSize="small" color="error" /></ListItemIcon>
                    <ListItemText>{t('sharingKeys.table.deleteTooltip')}</ListItemText>
                </MenuItem>
            </Menu>
        </Box>
    );
}

export default SharingKeysList;
