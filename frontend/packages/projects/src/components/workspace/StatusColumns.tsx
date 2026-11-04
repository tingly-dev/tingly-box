import type { Ref } from 'react';
import { Box } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread } from '../../client';
import { STATUS_ORDER, ThreadBadges, ThreadSubtitle } from '../project/threadStatus';
import { ListRow } from '../ListRow';
import { StatusHeading } from './StatusGroups';
import { timeAgo } from '../../utils/format';

interface Props {
    threads: Thread[];
    selectedId?: string;
    onOpen: (id: string) => void;
    /** Cards a column shows before it scrolls; unset, a column fills the height it is given. */
    listMaxHeight?: number;
    rowRef?: Ref<HTMLDivElement>;
    /** The row's own size: a fixed height, or room to fill. */
    rowSx?: object;
}

/** One thread as an outlined card; the band's columns and the Overview's boards all draw it. */
export function ThreadCard({ thread, selectedId, onOpen, stacked }: { thread: Thread; selectedId?: string; onOpen: (id: string) => void; stacked?: boolean }) {
    const { i18n } = useTranslation('projects');
    return (
        <ListRow outlined stacked={stacked} selected={thread.id === selectedId} onClick={() => onOpen(thread.id)}
            title={thread.title} subtitle={<ThreadSubtitle thread={thread} />} badges={<ThreadBadges thread={thread} />} meta={timeAgo(thread.updated_at, i18n.language)} />
    );
}

/**
 * Threads as the top band's board: one column per status, what needs the user first, each
 * thread an outlined card (the Overview's band and its kanban view draw the
 * same card). Columns with no threads are left out. It is the band's fixed-height
 * layout: the columns sit side by side and each scrolls inside; the Overview
 * pane uses `StatusBoard`, which scrolls one way only.
 */
export function StatusColumns({ threads, selectedId, onOpen, listMaxHeight, rowRef, rowSx }: Props) {
    const { t } = useTranslation('projects');
    const groups = STATUS_ORDER.map((s) => ({ key: s, list: threads.filter((th) => th.status === s) })).filter((g) => g.list.length > 0);
    const columns = groups.filter((g) => g.key !== 'resolved');
    // Finished work is only counted here; the Overview pane is where to read it.
    const finished = groups.find((g) => g.key === 'resolved')?.list.length ?? 0;
    if (columns.length === 0) return null;
    return (
        <Box ref={rowRef} sx={{ display: 'flex', gap: 1.5, px: 2, pb: 1.5, overflowX: 'auto', ...rowSx }}>
            {columns.map((g) => (
                <Box key={g.key} role="group" aria-label={t(`overview.group.${g.key}`)} sx={{ flex: '1 1 0', minWidth: 220, maxWidth: 360, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                    <Box sx={{ mb: 0.75 }}><StatusHeading label={t(`overview.group.${g.key}`)} status={g.key} count={g.list.length} /></Box>
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.75, overflowY: 'auto', minHeight: 0, ...(listMaxHeight == null ? { flex: 1 } : { maxHeight: listMaxHeight }) }}>
                        {g.list.map((th) => (
                            <ThreadCard key={th.id} thread={th} selectedId={selectedId} onOpen={onOpen} />
                        ))}
                    </Box>
                </Box>
            ))}
            {finished > 0 && (
                <Box sx={{ flex: '0 0 auto', minWidth: 120 }}>
                    <StatusHeading label={t('overview.group.resolved')} status="resolved" count={finished} />
                </Box>
            )}
        </Box>
    );
}
