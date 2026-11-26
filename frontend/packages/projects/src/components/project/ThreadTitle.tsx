import { useRef, useState } from 'react';
import { Box, ButtonBase, InputBase, Tooltip, Typography } from '@mui/material';
import { useCopyFeedback } from '@tingly/ui';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { Pencil } from '../icons';
import { radius } from '@tingly/ui/theme/radius';
import type { Thread } from '../../client';
import { useNotify } from '../../hooks/useNotify';
import { threadRef } from './threadStatus';

/**
 * The thread's title in the header, edited in place: click it to edit, Enter
 * or clicking away saves through `onSave` (the thread's setTitle op), Escape
 * puts the old title back. The saved title shows at once and stays when the
 * thread re-renders; the server's rename keeps the list's order.
 */
export function ThreadTitle({ title, onSave }: { title: string; onSave: (title: string) => Promise<boolean> }) {
    const { t } = useTranslation('projects');
    const [editing, setEditing] = useState(false);
    // The edit buffer; `edit` seeds it from the shown title every time.
    const [value, setValue] = useState('');
    // What the header shows: the saved title, ahead of the thread's next
    // re-read so the rename lands without waiting for its event.
    const [shown, setShown] = useState(title);
    // Escape sets this before the field goes away, so its blur does not save.
    const cancelled = useRef(false);
    const [lastTitle, setLastTitle] = useState(title);
    if (lastTitle !== title) {
        // The thread's own rename arrived: follow it (adjusted during render).
        setLastTitle(title);
        setShown(title);
    }

    const edit = () => {
        setValue(shown);
        setEditing(true);
    };
    const save = () => {
        setEditing(false);
        const next = value.trim();
        if (!next || next === shown) return;
        void onSave(next).then((ok) => ok && setShown(next));
    };

    if (editing) {
        return (
            <InputBase
                autoFocus
                fullWidth
                value={value}
                onFocus={(e) => e.currentTarget.select()}
                onChange={(e) => setValue(e.target.value)}
                onBlur={() => {
                    if (cancelled.current) {
                        cancelled.current = false;
                        return;
                    }
                    save();
                }}
                onKeyDown={(e) => {
                    if (e.key === 'Enter') e.currentTarget.blur();
                    else if (e.key === 'Escape') {
                        cancelled.current = true;
                        setEditing(false);
                    }
                }}
                inputProps={{ 'aria-label': t('thread.editTitle') }}
                sx={{ flex: 1, minWidth: 0, fontSize: fontSizes.base, fontWeight: 600, color: 'text.primary', '& .MuiInputBase-input': { p: 0, borderBottom: 1, borderColor: 'divider' } }}
            />
        );
    }
    return (
        <Box
            role="button"
            tabIndex={0}
            title={shown}
            aria-label={t('thread.editTitle')}
            onClick={edit}
            onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    edit();
                }
            }}
            sx={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'center', gap: 0.5, cursor: 'text', borderRadius: radius.row, px: 0.5, mx: -0.5, '&:hover': { bgcolor: 'action.hover' }, '&:hover .edit-hint': { opacity: 0.7 }, '&:focus-visible': { outline: 1, outlineColor: 'primary.main' }, '@media (pointer: coarse)': { minHeight: 44 } }}
        >
            <Typography noWrap sx={{ fontSize: fontSizes.base, fontWeight: 600, color: 'text.primary' }}>{shown}</Typography>
            <Pencil className="edit-hint" sx={{ fontSize: 13, flexShrink: 0, opacity: 0, transition: 'opacity 0.15s', color: 'text.secondary' }} />
        </Box>
    );
}

/** Copies how the thread is mentioned ("#3 Title", `threadRef`) and says so. */
export function useCopyThreadRef(thread: Pick<Thread, 'seq' | 'title'>) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const { copy } = useCopyFeedback();
    const ref = threadRef(thread);
    return { ref, copy: () => copy(ref, () => notify(t('thread.refCopied', { ref }), 'success')) };
}

/** The thread's number before its title in the header: a click copies "#3 Title", to mention it in the coordinator chat, IM or a PR. Nothing for the coordinator. */
export function ThreadRefButton({ thread }: { thread: Pick<Thread, 'seq' | 'title'> }) {
    const { t } = useTranslation('projects');
    const { ref, copy } = useCopyThreadRef(thread);
    if (!(thread.seq > 0)) return null;
    return (
        <Tooltip title={t('thread.copyRefOf', { ref })}>
            <ButtonBase data-thread-number onClick={() => void copy()} aria-label={t('thread.copyRefOf', { ref })}
                sx={{ flexShrink: 0, px: 0.5, mr: -0.25, borderRadius: radius.row, fontSize: fontSizes.base, fontWeight: 600, color: 'text.disabled', fontVariantNumeric: 'tabular-nums', '&:hover': { bgcolor: 'action.hover', color: 'text.secondary' }, '&:focus-visible': { outline: 1, outlineColor: 'primary.main' } }}>
                #{thread.seq}
            </ButtonBase>
        </Tooltip>
    );
}
