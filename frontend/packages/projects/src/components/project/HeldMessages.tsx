import { IconButton, Tooltip, Typography } from '@mui/material';
import { Box } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { ThreadEvent } from '../../client';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { ListRow } from '../ListRow';
import { Close, Pencil } from '../icons';

/** The messages of a thread the user chose to send after the running turn, still held. */
export function heldMessages(events: ThreadEvent[] | null | undefined): ThreadEvent[] {
    return (events ?? []).flatMap((e) => {
        if (e.kind !== 'queued') return [];
        try {
            return (JSON.parse(e.payload ?? '{}') as { state?: string }).state === 'held' ? [e] : [];
        } catch {
            return [];
        }
    });
}

interface Props {
    held: ThreadEvent[];
    /** Takes a message back into the message box to edit. */
    onEdit: (e: ThreadEvent) => void;
    onRemove: (e: ThreadEvent) => void;
}

/**
 * Sits right above the message box: what will be sent when this turn ends,
 * each with Edit (back into the box) and Remove. Neutral: a held message is
 * nothing the user has to act on.
 */
export function HeldMessages({ held, onEdit, onRemove }: Props) {
    const { t } = useTranslation('projects');
    if (held.length === 0) return null;
    return (
        <Box role="list" aria-label={t('queue.heading')} sx={{ mb: 1, display: 'flex', flexDirection: 'column', gap: 0.5 }}>
            <Typography sx={{ fontSize: fontSizes.sm, color: 'text.secondary', px: 0.5 }}>{t('queue.heading')}</Typography>
            {held.map((h) => (
                <ListRow
                    key={h.id}
                    outlined
                    role="listitem"
                    title={<span title={h.content}>{h.content || t('queue.attachmentsOnly')}</span>}
                    end={(
                        <Box sx={{ display: 'flex', flexShrink: 0 }}>
                            <Tooltip title={t('queue.edit')}>
                                <IconButton size="small" onClick={() => onEdit(h)} aria-label={t('queue.edit')}><Pencil sx={{ fontSize: 16 }} /></IconButton>
                            </Tooltip>
                            <Tooltip title={t('queue.remove')}>
                                <IconButton size="small" onClick={() => onRemove(h)} aria-label={t('queue.remove')}><Close sx={{ fontSize: 16 }} /></IconButton>
                            </Tooltip>
                        </Box>
                    )}
                />
            ))}
        </Box>
    );
}
