import { useState } from 'react';
import { Button, IconButton, ListSubheader, Menu, MenuItem, Tooltip } from '@mui/material';
import { mobile } from '@tingly/ui/theme/mobile';
import { useTranslation } from 'react-i18next';
import type { ChannelBinding } from '../../client/channelBindings';
import { serverNow } from '../../utils/serverClock';
import { Bell, BellOff } from '../icons';

type Mute = Pick<ChannelBinding, 'muted' | 'muted_until'>;

/** Whether a binding is muted now; a mute that ran out is not. */
export function isMuted(b: Mute | undefined, now = serverNow()): boolean {
    return !!b?.muted && (!b.muted_until || Date.parse(b.muted_until) > now);
}

export const MUTE_CHOICES = ['hour', 'hours8', 'tomorrow', 'forever'] as const;
export type MuteChoice = (typeof MUTE_CHOICES)[number];

const HOUR_MS = 3_600_000;
/** "Until tomorrow" ends at this local hour. */
const TOMORROW_HOUR = 9;

/** When a mute chosen now ends; undefined is until unmuted. */
export function muteUntil(choice: MuteChoice, now = new Date()): Date | undefined {
    switch (choice) {
        case 'hour':
            return new Date(serverNow() + HOUR_MS);
        case 'hours8':
            return new Date(serverNow() + 8 * HOUR_MS);
        case 'tomorrow':
            return new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1, TOMORROW_HOUR);
        case 'forever':
            return undefined;
    }
}

/** "Muted until 18:00" (today), "Muted until Nov 21, 09:00", or "Muted"; empty when not muted. */
export function useMuteText(): (b: Mute | undefined) => string {
    const { t, i18n } = useTranslation('projects');
    return (b) => {
        if (!isMuted(b)) return '';
        if (!b?.muted_until) return t('im.mute.indefinite');
        const at = new Date(b.muted_until);
        const today = at.toDateString() === new Date().toDateString();
        const time = at.toLocaleString(i18n.language, today
            ? { hour: '2-digit', minute: '2-digit' }
            : { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
        return t('im.mute.until', { time });
    };
}

const tapSx = { '@media (pointer: coarse)': { minWidth: mobile.tapMin, minHeight: mobile.tapMin } } as const;

/**
 * Mute or unmute one bound chat: a bell that offers how long to mute, or,
 * while muted, one click to unmute.
 */
export function MuteControl({ binding, chat, onMute }: {
    binding: ChannelBinding;
    /** The chat's name, for the labels. */
    chat: string;
    /** Mutes until `until` (undefined: until unmuted), or unmutes with `muted` false. */
    onMute: (muted: boolean, until?: Date) => Promise<unknown>;
}) {
    const { t } = useTranslation('projects');
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const [busy, setBusy] = useState(false);
    const run = async (muted: boolean, until?: Date) => {
        setAnchor(null);
        setBusy(true);
        try {
            await onMute(muted, until);
        } catch {
            // The caller says what went wrong.
        } finally {
            setBusy(false);
        }
    };

    if (isMuted(binding)) {
        return (
            <Button size="small" variant="outlined" color="inherit" disabled={busy} startIcon={<Bell sx={{ fontSize: 16 }} />}
                aria-label={t('im.mute.unmuteLabel', { chat })} onClick={() => void run(false)} sx={{ whiteSpace: 'nowrap', ...tapSx }}>
                {t('im.mute.unmute')}
            </Button>
        );
    }
    return (
        <>
            <Tooltip title={t('im.mute.action')}>
                <IconButton size="small" disabled={busy} aria-label={t('im.mute.muteLabel', { chat })} aria-haspopup="menu"
                    onClick={(e) => setAnchor(e.currentTarget)} sx={{ color: 'text.secondary', ...tapSx }}>
                    <BellOff sx={{ fontSize: 18 }} />
                </IconButton>
            </Tooltip>
            <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)}
                slotProps={{ list: { 'aria-label': t('im.mute.muteLabel', { chat }) } }}>
                <ListSubheader sx={{ lineHeight: 1.5, py: 1, maxWidth: 280, whiteSpace: 'normal', bgcolor: 'transparent' }}>{t('im.mute.hint')}</ListSubheader>
                {MUTE_CHOICES.map((c) => (
                    <MenuItem key={c} onClick={() => void run(true, muteUntil(c))} sx={tapSx}>{t(`im.mute.${c}`)}</MenuItem>
                ))}
            </Menu>
        </>
    );
}
