import { useMemo, useState } from 'react';
import { Box, Button, Dialog, DialogActions, DialogContent, Skeleton, Typography } from '@mui/material';
import { DialogHeader, EmptyState } from '@tingly/ui';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { useTranslation } from 'react-i18next';
import type { ChannelBinding } from '../../client/channelBindings';
import { isFolder, type ProjectSummary } from '../../client';
import { folderName } from '../../utils/format';
import type { ImBot, ImChat } from '../../host/ProjectsHost';
import { Info } from '../icons';
import { ChoiceList, type Choice } from './ChoiceList';
import { PlatformMark, usePlatformLabel } from './PlatformMark';
import { chatKey } from './useImBindings';

interface Props {
    open: boolean;
    /** The thread the chat will be bound to. */
    threadId: string;
    /** Its name, for the title: the project's, or the folder thread's. */
    targetName: string;
    /** Every chat the host's bots have seen; null while loading. */
    chats: ImChat[] | null;
    hasBots: boolean;
    bots?: ImBot[];
    byChat: Map<string, ChannelBinding>;
    /** Every project and folder by id, for "Bound to X". */
    directory: Map<string, ProjectSummary>;
    onClose: () => void;
    /** Resolves when bound; rejects (after reporting) to keep the dialog open. */
    onBind: (chat: ImChat, movedFrom: boolean) => Promise<void>;
    onOpenSettings?: () => void;
}

/** Bind a known IM chat to this thread (the project side of "bind from either side"). */
export function BindChatDialog({ open, threadId, targetName, chats, hasBots, bots = [], byChat, directory, onClose, onBind, onOpenSettings }: Props) {
    const { t } = useTranslation('projects');
    const platformLabel = usePlatformLabel();
    const [picked, setPicked] = useState<string | null>(null);
    const [busy, setBusy] = useState(false);

    const close = () => {
        setPicked(null);
        onClose();
    };

    // Chats already bound here are in the header; the rest are candidates, free ones first.
    const candidates = useMemo(() => {
        const rest = (chats ?? []).filter((c) => {
            const binding = byChat.get(chatKey(c.platform, c.chatId));
            return binding?.thread_id !== threadId || binding.bot_uuid !== c.botUuid;
        });
        const elsewhere = (c: ImChat) => (byChat.has(chatKey(c.platform, c.chatId)) ? 1 : 0);
        return rest.sort((a, b) => elsewhere(a) - elsewhere(b));
    }, [chats, byChat, threadId]);

    const items: Choice[] = candidates.map((c) => {
        const bound = byChat.get(chatKey(c.platform, c.chatId));
        const place = bound ? directory.get(bound.project_id) : undefined;
        // A folder is bound by one of its threads: say it is a folder, not a project.
        const where = bound ? t(place && isFolder(place) ? 'im.place.atFolder' : 'im.place.at', { place: (place && isFolder(place) ? folderName(place.path ?? '') || place.name : place?.name) ?? t('im.place.elsewhere') }) : t('im.place.none');
        return {
            key: `${chatKey(c.platform, c.chatId)}\0${c.botUuid}`,
            lead: <PlatformMark platform={c.platform} size={28} />,
            title: c.title,
            subtitle: [platformLabel(c.platform), bots.find((bot) => bot.uuid === c.botUuid)?.name, t(`im.chatKind.${c.kind}`), c.state === 'blocked' ? t('im.bind.blocked') : where].filter(Boolean).join(' · '),
            chip: bound && c.state !== 'blocked' ? t('im.bind.rebind') : undefined,
            disabled: c.state === 'blocked',
        };
    });

    const submit = async () => {
        const chat = candidates.find((c) => `${chatKey(c.platform, c.chatId)}\0${c.botUuid}` === picked);
        if (!chat) return;
        setBusy(true);
        try {
            await onBind(chat, byChat.has(chatKey(chat.platform, chat.chatId)));
            close();
        } catch {
            // reported by the caller; stay open so the choice is not lost
        } finally {
            setBusy(false);
        }
    };

    const loading = chats === null;
    return (
        <Dialog open={open} onClose={busy ? undefined : close} maxWidth="sm" fullWidth aria-labelledby="bind-chat-title">
            <DialogHeader titleId="bind-chat-title" title={t('im.bind.title', { project: targetName })} closeLabel={t('common.close')} onClose={close} closeDisabled={busy} />
            <DialogContent sx={{ pt: 0.5 }}>
                {loading ? (
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.75 }}>
                        {[0, 1, 2].map((i) => <Skeleton key={i} variant="rounded" height={56} />)}
                    </Box>
                ) : items.length === 0 ? (
                    <EmptyState
                        compact
                        title={hasBots ? t('im.bind.noChatsTitle') : t('im.bind.noBotsTitle')}
                        description={hasBots ? ((chats ?? []).length > 0 ? t('im.bind.allBound') : t('im.bind.noChatsBody')) : t('im.bind.noBotsBody')}
                        primaryAction={onOpenSettings ? { label: t('im.bind.openSettings'), onClick: onOpenSettings, variant: 'outlined' } : undefined}
                    />
                ) : (
                    <ChoiceList items={items} value={picked} onChange={setPicked} label={t('im.bind.title', { project: targetName })} />
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
