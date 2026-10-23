import { useCallback, useState, type ReactNode } from 'react';
import { Box, Button, IconButton, ListItemIcon, ListItemText, Menu, MenuItem, Tooltip, Typography } from '@mui/material';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { ChannelBinding } from '../../client/channelBindings';
import { useNotify } from '../../hooks/useNotify';
import { useProjectsHost, type ImChat } from '../../host/ProjectsHost';
import { Add, Close } from '../icons';
import { BindChatDialog } from './BindChatDialog';
import { PlatformMark } from './PlatformMark';
import { chatKey, useChannelBindings, useImChats, useProjectDirectory } from './useImBindings';
import { radius } from '@tingly/ui/theme/radius';

/**
 * What a header binds chats to: a thread, named for the dialog. A project's
 * conversation is its main thread and is bound as the project (the server
 * resolves it); a folder cannot be bound as a project, so its open thread is
 * bound by id.
 */
export interface ImTarget {
    threadId: string;
    name: string;
    /** Set for a project's main thread. */
    projectId?: string;
}

/** The IM chats bound to one thread, and what the header needs to show and change them. */
export interface ProjectIm {
    /** The chats bound to this thread; null until the first answer. */
    mine: ChannelBinding[] | null;
    open: () => void;
    unbind: (b: ChannelBinding) => Promise<void>;
    /** A bound chat's name by platform and chat id, for a message's "From …" line. */
    chatTitle: (platform: string, chatId: string) => string | undefined;
    /** The bind dialog; render it once, anywhere. */
    dialog: ReactNode;
}

/**
 * Everything a page header needs for IM (TEP #2143): the bindings for its
 * thread, unbinding, and the bind dialog. `null` when the host has no IM (the
 * standalone app) or there is no thread to bind yet, so the header shows
 * nothing at all.
 */
export function useProjectIm(target: ImTarget | null): ProjectIm | null {
    const { im, paths } = useProjectsHost();
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const navigate = useNavigate();
    const [dialogOpen, setDialogOpen] = useState(false);
    const { bindings, byChat, bind, unbind } = useChannelBindings(!!im);
    // The dialog's data is only needed while it is open.
    const { bots, chats } = useImChats(!!im && dialogOpen);
    const directory = useProjectDirectory(!!im && dialogOpen);

    const threadId = target?.threadId ?? '';
    const projectId = target?.projectId;
    const projectName = target?.name ?? '';

    const onBind = useCallback(async (chat: ImChat, movedFrom: boolean) => {
        try {
            await bind(chat, projectId ? { project_id: projectId } : { thread_id: threadId });
            notify(t(movedFrom ? 'im.bind.moved' : 'im.bind.done', { chat: chat.title, project: projectName }), 'success');
        } catch (e) {
            notify(`${t('im.bind.failed')}: ${(e as Error).message}`, 'error');
            throw e;
        }
    }, [bind, notify, projectId, threadId, projectName, t]);

    const onUnbind = useCallback(async (b: ChannelBinding) => {
        try {
            await unbind(b.id);
            notify(t('im.bind.unbound', { chat: b.chat_title || b.chat_id }), 'success');
        } catch (e) {
            notify(`${t('im.bind.unbindFailed')}: ${(e as Error).message}`, 'error');
        }
    }, [notify, t, unbind]);

    if (!im || !target) return null;
    return {
        mine: bindings && bindings.filter((b) => b.thread_id === threadId),
        open: () => setDialogOpen(true),
        unbind: onUnbind,
        chatTitle: (platform, chatId) => byChat.get(chatKey(platform, chatId))?.chat_title || undefined,
        dialog: (
            <BindChatDialog
                open={dialogOpen}
                threadId={threadId}
                targetName={projectName}
                chats={chats}
                hasBots={(bots ?? []).length > 0}
                byChat={byChat}
                directory={directory}
                onClose={() => setDialogOpen(false)}
                onBind={onBind}
                onOpenSettings={paths.im ? () => { setDialogOpen(false); navigate(paths.im!); } : undefined}
            />
        ),
    };
}

const tagTitle = (b: ChannelBinding) => b.chat_title || b.chat_id;

/** One bound chat: platform mark, name, and × to unbind. */
function Tag({ binding, onUnbind }: { binding: ChannelBinding; onUnbind: () => void }) {
    const { t } = useTranslation('projects');
    const title = tagTitle(binding);
    return (
        <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, height: 26, pl: 1, pr: 0.25, borderRadius: radius.pill, border: 1, borderColor: 'divider', bgcolor: 'action.hover', minWidth: 0, maxWidth: 180 }}>
            <PlatformMark platform={binding.platform} size={14} />
            <Typography noWrap title={title} sx={{ fontSize: fontSizes.sm, fontWeight: 500, minWidth: 0 }}>{title}</Typography>
            <Tooltip title={t('im.tags.unbind', { chat: title })}>
                <IconButton size="small" onClick={onUnbind} aria-label={t('im.tags.unbind', { chat: title })} sx={{ p: 0.25, color: 'text.secondary', '&:hover': { color: 'error.main' } }}>
                    <Close sx={{ fontSize: 14 }} />
                </IconButton>
            </Tooltip>
        </Box>
    );
}

/** "+N" for the tags that did not fit: opens them as a list with the same ×. */
function MoreTags({ bindings, onUnbind }: { bindings: ChannelBinding[]; onUnbind: (b: ChannelBinding) => void }) {
    const { t } = useTranslation('projects');
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    return (
        <>
            <Button size="small" color="inherit" onClick={(e) => setAnchor(e.currentTarget)} aria-haspopup="menu" aria-label={t('im.tags.label')}
                sx={{ minWidth: 0, height: 26, px: 1, borderRadius: radius.pill, border: 1, borderColor: 'divider', color: 'text.secondary', fontSize: fontSizes.sm }}>
                {t('im.tags.more', { count: bindings.length })}
            </Button>
            <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)}>
                {bindings.map((b) => (
                    <MenuItem key={b.id} onClick={() => { setAnchor(null); onUnbind(b); }} aria-label={t('im.tags.unbind', { chat: tagTitle(b) })}>
                        <ListItemIcon><PlatformMark platform={b.platform} size={16} /></ListItemIcon>
                        <ListItemText primary={tagTitle(b)} slotProps={{ primary: { noWrap: true, sx: { maxWidth: 200 } } }} />
                        <Close sx={{ fontSize: 14, ml: 1.5, color: 'text.secondary' }} />
                    </MenuItem>
                ))}
            </Menu>
        </>
    );
}

/**
 * "Bound IM chats" pill tags plus a dashed "Bind chat" button. `maxVisible`
 * keeps a tight header on one line; the rest fold into "+N".
 */
export function ProjectImTags({ im, maxVisible = 2, showLabel = false }: { im: ProjectIm; maxVisible?: number; showLabel?: boolean }) {
    const { t } = useTranslation('projects');
    const mine = im.mine ?? [];
    const shown = mine.slice(0, maxVisible);
    const rest = mine.slice(maxVisible);
    return (
        <Box role="group" aria-label={t('im.tags.label')} sx={{ display: 'flex', alignItems: 'center', gap: 0.75, flexWrap: 'wrap', minWidth: 0 }}>
            {showLabel && <Typography sx={{ fontSize: fontSizes.sm, color: 'text.secondary', mr: 0.25 }}>{t('im.tags.label')}</Typography>}
            {shown.map((b) => <Tag key={b.id} binding={b} onUnbind={() => void im.unbind(b)} />)}
            {rest.length > 0 && <MoreTags bindings={rest} onUnbind={(b) => void im.unbind(b)} />}
            <Button size="small" color="inherit" startIcon={<Add sx={{ fontSize: 14 }} />} onClick={im.open}
                sx={{ height: 26, px: 1.25, borderRadius: radius.pill, border: 1, borderStyle: 'dashed', borderColor: 'text.disabled', color: 'text.secondary', fontSize: fontSizes.sm, whiteSpace: 'nowrap', '&:hover': { borderColor: 'text.secondary', color: 'text.primary', bgcolor: 'action.hover' }, '& .MuiButton-startIcon': { mr: 0.5, ml: 0 } }}>
                {t('im.tags.bind')}
            </Button>
        </Box>
    );
}
