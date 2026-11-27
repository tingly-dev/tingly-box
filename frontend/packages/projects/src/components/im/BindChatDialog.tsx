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
import { ImChatLabel } from './ImChatLabel';
import { PlatformMark } from './PlatformMark';
import { classifyImId, isBindable, isOfferedForBinding } from './imIdKind';
import { chatKey } from './useImBindings';

interface Props {
    open: boolean;
    /** The thread the chat will be bound to. */
    threadId?: string;
    folderId?: string;
    projectId?: string;
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
export function BindChatDialog({ open, threadId, folderId, projectId, targetName, chats, hasBots, bots = [], byChat, directory, onClose, onBind, onOpenSettings }: Props) {
    const { t } = useTranslation('projects');
    const [picked, setPicked] = useState<string | null>(null);
    const [busy, setBusy] = useState(false);

    const close = () => {
        setPicked(null);
        onClose();
    };

    // Chats already bound here are in the header; the rest are candidates, free ones first.
    // A known non-receiver (a Feishu department, a WeChat group JID, …) is not
    // offered at all; an id type we cannot read stays visible but disabled so
    // the user sees it is there. Both stay in IM settings (the raw overview).
    const { offered, hiddenCount } = useMemo(() => {
        const rest = (chats ?? []).filter((c) => {
            const binding = byChat.get(chatKey(c.platform, c.chatId));
            const here = folderId ? binding?.project_id === folderId && !binding.target_thread_id : projectId ? binding?.project_id === projectId : !!threadId && (binding?.target_thread_id === threadId || (!binding?.target_project_id && binding?.thread_id === threadId && !binding.folder));
            return !here || binding?.bot_uuid !== c.botUuid;
        });
        const elsewhere = (c: ImChat) => (byChat.has(chatKey(c.platform, c.chatId)) ? 1 : 0);
        rest.sort((a, b) => elsewhere(a) - elsewhere(b));
        const shown = rest.filter((c) => isOfferedForBinding(classifyImId(c.platform, c.kind, c.chatId)));
        return { offered: shown, hiddenCount: rest.length - shown.length };
    }, [chats, byChat, threadId, folderId, projectId]);

    const items: Choice[] = offered.map((c) => {
        const bound = byChat.get(chatKey(c.platform, c.chatId));
        const place = bound ? directory.get(bound.project_id) : undefined;
        const id = classifyImId(c.platform, c.kind, c.chatId);
        // A folder is bound by one of its threads: say it is a folder, not a project.
        const where = bound ? t(place && isFolder(place) ? 'im.place.atFolder' : 'im.place.at', { place: (place && isFolder(place) ? folderName(place.path ?? '') || place.name : place?.name) ?? t('im.place.elsewhere') }) : t('im.place.none');
        // An id we cannot read says why it is not selectable instead of looking broken.
        const state = c.state === 'blocked' ? t('im.bind.blocked') : !isBindable(id) ? t(id.reasonKey ?? 'im.idReason.unknown') : where;
        return {
            key: `${chatKey(c.platform, c.chatId)}\0${c.botUuid}`,
            lead: <PlatformMark platform={c.platform} size={28} colored={!!bound} />,
            title: <ImChatLabel platform={c.platform} chatId={c.chatId} kind={c.kind} alias={c.alias} platformAs="text" />,
            subtitle: [bots.find((bot) => bot.uuid === c.botUuid)?.name, state].filter(Boolean).join(' · '),
            chip: bound && c.state !== 'blocked' ? t('im.bind.rebind') : undefined,
            disabled: c.state === 'blocked' || !isBindable(id),
        };
    });

    const submit = async () => {
        const chat = offered.find((c) => `${chatKey(c.platform, c.chatId)}\0${c.botUuid}` === picked);
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
    const known = (chats ?? []).length;
    // Nothing to show has several causes; say which one instead of a generic "no chats".
    const empty = !hasBots
        ? { title: t('im.bind.noBotsTitle'), body: t('im.bind.noBotsBody') }
        : known === 0
            ? { title: t('im.bind.noChatsTitle'), body: t('im.bind.noChatsBody') }
            : hiddenCount > 0 && offered.length === 0
                ? { title: t('im.bind.noSendableTitle'), body: t('im.bind.noSendableBody') }
                : { title: t('im.bind.noChatsTitle'), body: t('im.bind.allBound') };
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
                        title={empty.title}
                        description={empty.body}
                        primaryAction={onOpenSettings ? { label: t('im.bind.openSettings'), onClick: onOpenSettings, variant: 'outlined' } : undefined}
                    />
                ) : (
                    <ChoiceList items={items} value={picked} onChange={setPicked} label={t('im.bind.title', { project: targetName })} />
                )}
                {hiddenCount > 0 && items.length > 0 && (
                    <Typography color="text.secondary" sx={{ mt: 1.25, fontSize: fontSizes.sm }}>
                        {t('im.bind.hidden', { count: hiddenCount })}
                    </Typography>
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
