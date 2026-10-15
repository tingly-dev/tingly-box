import { useEffect, useState } from 'react';
import { Button, Dialog, DialogActions, DialogContent, DialogTitle, Divider, ListItemIcon, ListItemText, Menu, MenuItem, TextField } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type Conversation } from '../client';
import { Delete, Edit, Star, StarBorder } from './icons';
import { useNotify } from '../hooks/useNotify';
import { useAppData } from '../contexts/AppDataContext';
import { ConfirmDialog } from './ConfirmDialog';

type ChatLike = Pick<Conversation, 'id' | 'title' | 'starred'>;

interface Props {
    chat: ChatLike;
    anchorEl: HTMLElement | null;
    onClose: () => void;
    onChanged?: (c: Conversation) => void;
    onDeleted?: () => void;
}

/** Star / rename / delete a chat. */
export function ChatActionsMenu({ chat, anchorEl, onClose, onChanged, onDeleted }: Props) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const { refresh } = useAppData();
    const [renaming, setRenaming] = useState(false);
    const [deleting, setDeleting] = useState(false);
    const [title, setTitle] = useState(chat.title);

    useEffect(() => setTitle(chat.title), [chat.title]);

    const patch = async (body: { title?: string; starred?: boolean }) => {
        try {
            const c = await unwrap(api.PATCH('/api/v1/conversations/{id}', { params: { path: { id: chat.id } }, body }));
            onChanged?.(c);
            refresh();
            return c;
        } catch (e) {
            notify((e as Error).message, 'error');
            throw e;
        }
    };

    const act = (fn: () => void) => () => {
        onClose();
        fn();
    };

    return (
        <>
            <Menu anchorEl={anchorEl} open={!!anchorEl} onClose={onClose}>
                <MenuItem onClick={act(() => void patch({ starred: !chat.starred }))}>
                    <ListItemIcon>{chat.starred ? <Star fontSize="small" color="primary" /> : <StarBorder fontSize="small" />}</ListItemIcon>
                    <ListItemText>{chat.starred ? t('common.unstar') : t('common.star')}</ListItemText>
                </MenuItem>
                <MenuItem onClick={act(() => setRenaming(true))}>
                    <ListItemIcon><Edit fontSize="small" /></ListItemIcon>
                    <ListItemText>{t('common.rename')}</ListItemText>
                </MenuItem>
                <Divider />
                <MenuItem onClick={act(() => setDeleting(true))} sx={{ color: 'error.main' }}>
                    <ListItemIcon><Delete fontSize="small" color="error" /></ListItemIcon>
                    <ListItemText>{t('common.delete')}</ListItemText>
                </MenuItem>
            </Menu>

            <Dialog open={renaming} onClose={() => setRenaming(false)} maxWidth="xs" fullWidth>
                <DialogTitle>{t('chat.renameTitle')}</DialogTitle>
                <DialogContent>
                    <TextField
                        autoFocus
                        fullWidth
                        value={title}
                        onChange={(e) => setTitle(e.target.value)}
                        sx={{ mt: 1 }}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter') void patch({ title }).then(() => setRenaming(false));
                        }}
                    />
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={() => setRenaming(false)} color="inherit">{t('common.cancel')}</Button>
                    <Button variant="contained" onClick={() => void patch({ title }).then(() => setRenaming(false))}>{t('common.save')}</Button>
                </DialogActions>
            </Dialog>

            <ConfirmDialog
                open={deleting}
                title={t('chat.deleteTitle')}
                message={t('chat.deleteMessage', { title: chat.title || t('chat.untitled') })}
                confirmLabel={t('common.delete')}
                danger
                onClose={() => setDeleting(false)}
                onConfirm={() => {
                    unwrap(api.DELETE('/api/v1/conversations/{id}', { params: { path: { id: chat.id } } }))
                        .then(() => {
                            refresh();
                            onDeleted?.();
                        })
                        .catch((e: Error) => notify(e.message, 'error'));
                }}
            />
        </>
    );
}
