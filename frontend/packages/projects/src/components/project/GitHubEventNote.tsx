import { Box, Stack, Typography } from '@mui/material';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { useTranslation } from 'react-i18next';
import type { TranscriptEntry } from '../../transcript';

interface GitHubNotePayload {
    github?: string;
    coordinator_event?: string;
}

function payloadOf(message: TranscriptEntry): GitHubNotePayload | null {
    const p = message.payload;
    if (!p) return null;
    if (typeof p === 'object') return p as GitHubNotePayload;
    try {
        return JSON.parse(String(p)) as GitHubNotePayload;
    } catch {
        return null;
    }
}

/**
 * The coordinator input a GitHub event in a thread was told as, or undefined
 * when it was only recorded (a resolved thread, a paused project, a folder).
 */
export function coordinatorEventOf(message: TranscriptEntry): string | undefined {
    if (message.kind !== 'system') return undefined;
    const pl = payloadOf(message);
    return pl?.github ? pl.coordinator_event || undefined : undefined;
}

/**
 * A GitHub event in a thread (its PR merged, closed, in conflict) that the
 * coordinator was told of: the note as the thread's other system notes read,
 * and under it that the coordinator has it, with a way to its reply.
 */
export function GitHubEventNote({ message, onShowReply }: { message: TranscriptEntry; onShowReply?: (eventId: string) => void }) {
    const { t } = useTranslation('projects');
    const eventId = coordinatorEventOf(message);
    if (!eventId) return null;
    return (
        <Stack spacing={0.25} sx={{ alignItems: 'center', textAlign: 'center' }}>
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>{message.content}</Typography>
            <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', fontSize: fontSizes.sm, color: 'text.disabled' }}>
                <span>{t('thread.githubEvent.told')}</span>
                {onShowReply && (
                    <>
                        <span aria-hidden>·</span>
                        <Box component="button" type="button" onClick={() => onShowReply(eventId)}
                            sx={{ p: 0, border: 0, bgcolor: 'transparent', font: 'inherit', color: 'inherit', cursor: 'pointer', '&:hover': { color: 'text.primary', textDecoration: 'underline' } }}>
                            {t('thread.githubEvent.reply')}
                        </Box>
                    </>
                )}
            </Stack>
        </Stack>
    );
}
