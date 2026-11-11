import type { ReactNode } from 'react';
import { Box, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import type { Thread } from '../../client';
import { timeAgo } from '../../utils/format';
import { ListRow } from '../ListRow';
import { ThreadBadges, ThreadStateMark, ThreadSubtitle } from './threadStatus';

interface Props {
    /** The state the list stands for, as its heading says it. */
    label: string;
    /** The threads (a project's, or a folder's sessions) in that state. */
    threads: Thread[];
    /** Opens a thread (or session) from its row. */
    onOpen: (id: string) => void;
    /** A session's own subtitle; default is the thread's (what it waits on, else its last report). */
    subtitleFor?: (th: Thread) => ReactNode;
    /** Opens a thread's running tasks from its count pill, where it has one. */
    onTasks?: (threads: Thread[], anchor: HTMLElement) => void;
    onClose: () => void;
}

/**
 * What a status count stands for, one click away: the threads (or sessions)
 * in that state, each row opening the thread itself. A working row keeps its
 * tasks pill, so a state drills down to a thread and from there to what it is
 * running. Called from the status area's headings and counts.
 */
export function StatusThreadsPopover({ label, threads, onOpen, subtitleFor, onTasks, onClose }: Props) {
    const { t, i18n } = useTranslation('projects');
    if (threads.length === 0) {
        return (
            <Box sx={{ width: 480, maxWidth: '92vw', p: 2 }}>
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>{t('overview.noThreads')}</Typography>
            </Box>
        );
    }
    return (
        <Box sx={{ width: 480, maxWidth: '92vw', maxHeight: '75vh', overflowY: 'auto', py: 0.5 }}>
            <Typography variant="caption" component="div"
                sx={{ px: 1.5, pt: 1, pb: 0.5, color: 'text.secondary', fontWeight: 600, letterSpacing: 0.3, textTransform: 'uppercase', fontSize: fontSizes.xs }}>
                {label} · {threads.length}
            </Typography>
            {threads.map((th) => (
                <ListRow key={th.id} data-status-thread={th.id}
                    onClick={() => { onClose(); onOpen(th.id); }}
                    mark={<ThreadStateMark status={th.status} queued={th.queued} />}
                    title={th.title}
                    subtitle={subtitleFor ? subtitleFor(th) : <ThreadSubtitle thread={th} />}
                    badges={<ThreadBadges thread={th} onTasks={onTasks && ((ths, anchor) => { onClose(); onTasks(ths, anchor); })} />}
                    meta={timeAgo(th.updated_at, i18n.language)} />
            ))}
        </Box>
    );
}
