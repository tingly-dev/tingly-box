import { useState } from 'react';
import { Box, Button, Dialog, DialogActions, DialogContent, Typography } from '@mui/material';
import { DialogHeader, EmptyState } from '@tingly/ui';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { useTranslation } from 'react-i18next';
import type { ChannelBinding } from '../../client/channelBindings';
import { isFolder, type ProjectSummary } from '../../client';
import type { ImChat } from '../../host/ProjectsHost';
import { Folder, Info } from '../icons';
import { ChoiceList, type Choice } from './ChoiceList';

interface Props {
    /** The chat to bind or move; null closes the dialog. */
    chat: ImChat | null;
    /** Where it is bound now. */
    bound?: ChannelBinding;
    /** The projects a chat can be bound to (folders are bound by thread, from their page). */
    projects: ProjectSummary[];
    onClose: () => void;
    /** Resolves when done; rejects (after reporting) to keep the dialog open. */
    onMove: (chat: ImChat, projectId: string) => Promise<void>;
}

/** Bind or move one chat to a project (the chat side of "bind from either side"). */
export function MoveChatDialog({ chat, bound, projects, onClose, onMove }: Props) {
    const { t } = useTranslation('projects');
    const [picked, setPicked] = useState<string | null>(null);
    const [busy, setBusy] = useState(false);

    const close = () => {
        setPicked(null);
        onClose();
    };

    // Moving to where it already is changes nothing, so that project is not offered.
    const items: Choice[] = projects.filter((p) => p.id !== bound?.project_id || (isFolder(p) && !!bound.target_thread_id)).map((p) => ({
        key: p.id,
        lead: <Folder sx={{ fontSize: 22, color: 'text.secondary' }} />,
        title: p.name,
        subtitle: isFolder(p) ? t('im.focus.folder') : t('im.focus.project'),
    }));

    const submit = async () => {
        if (!chat || !picked) return;
        setBusy(true);
        try {
            await onMove(chat, picked);
            close();
        } catch {
            // reported by the caller; stay open
        } finally {
            setBusy(false);
        }
    };

    const title = t('im.settings.pickProject', { chat: chat?.title ?? '' });
    return (
        <Dialog open={!!chat} onClose={busy ? undefined : close} maxWidth="sm" fullWidth aria-labelledby="move-chat-title">
            <DialogHeader titleId="move-chat-title" title={title} closeLabel={t('common.close')} onClose={close} closeDisabled={busy} />
            <DialogContent sx={{ pt: 0.5 }}>
                {items.length === 0 ? (
                    <EmptyState compact title={t('im.settings.noProjects')} />
                ) : (
                    <ChoiceList items={items} value={picked} onChange={setPicked} label={title} />
                )}
                <Box sx={{ display: 'flex', gap: 1, alignItems: 'flex-start', mt: 2, color: 'text.secondary' }}>
                    <Info sx={{ fontSize: 16, mt: '2px', flexShrink: 0 }} />
                    <Typography sx={{ fontSize: fontSizes.sm, lineHeight: 1.5 }}>{t('im.bind.note')}</Typography>
                </Box>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={close} color="inherit" disabled={busy}>{t('common.cancel')}</Button>
                <Button variant="contained" onClick={() => void submit()} disabled={!picked || busy}>{t('im.bind.submit')}</Button>
            </DialogActions>
        </Dialog>
    );
}
