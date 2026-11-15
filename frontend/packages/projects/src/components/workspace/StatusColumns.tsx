import type { ReactNode, Ref } from 'react';
import { Box, IconButton, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread } from '../../client';
import { STATUS_ORDER, ThreadBadges } from '../project/threadStatus';
import { ThreadRequestSubtitle, ThreadRequestsStrip } from '../project/ThreadRequests';
import { ListRow } from '../ListRow';
import { Star, StarBorder } from '../icons';
import { StatusHeading } from './StatusGroups';
import { timeAgo } from '../../utils/format';

interface Props {
    threads: Thread[];
    selectedId?: string;
    onOpen: (id: string) => void;
    /** Opens the thread's running tasks from its card's count pill. */
    onTasks?: (threads: Thread[], anchor: HTMLElement) => void;
    /** Opens a column's thread list from its status heading. */
    onStatus?: (status: string, anchor: HTMLElement) => void;
    /** Cards a column shows before it scrolls; unset, a column fills the height it is given. */
    listMaxHeight?: number;
    rowRef?: Ref<HTMLDivElement>;
    /** The row's own size: a fixed height, or room to fill. */
    rowSx?: object;
}

/** One thread as an outlined card; the band's columns and the Overview's boards all draw it. */
export function ThreadCard({ thread, selectedId, onOpen, onTasks, stacked, pinned, onTogglePin, subtitle }: { thread: Thread; /** The line under the title; what the thread's request or report says by default. */ subtitle?: ReactNode; selectedId?: string; onOpen: (id: string) => void; /** Opens the thread's running tasks from the count pill. */ onTasks?: (threads: Thread[], anchor: HTMLElement) => void; stacked?: boolean; pinned?: boolean; /** Given only where a card can be pinned to the top: the Overview board. */ onTogglePin?: () => void }) {
    const { t, i18n } = useTranslation('projects');
    return (
        <ListRow outlined stacked={stacked} selected={thread.id === selectedId} onClick={() => onOpen(thread.id)}
            title={thread.title} subtitle={subtitle ?? <ThreadRequestSubtitle thread={thread} />} badges={<ThreadBadges thread={thread} onTasks={onTasks} />} meta={timeAgo(thread.updated_at, i18n.language)}
            footer={<ThreadRequestsStrip thread={thread} onOpen={onOpen} />}
            // The star stays quiet: an unpinned card shows it on hover or keyboard focus only.
            sx={{ '&:hover .pin-star, &:focus-within .pin-star': { opacity: 1, pointerEvents: 'auto' } }}
            end={onTogglePin != null && (
                <Tooltip title={pinned ? t('overview.unpin') : t('overview.pin')}>
                    {/* The star must not open the thread. */}
                    <IconButton className="pin-star" size="small" sx={{ p: 0.5, opacity: pinned ? 1 : 0, pointerEvents: pinned ? 'auto' : 'none' }} aria-label={pinned ? t('overview.unpin') : t('overview.pin')} onClick={(e) => { e.stopPropagation(); onTogglePin(); }}>
                        {pinned ? <Star fontSize="small" color="primary" /> : <StarBorder fontSize="small" />}
                    </IconButton>
                </Tooltip>
            )} />
    );
}

/**
 * Threads as the top band's board: one column per status, what runs first, each
 * thread an outlined card (the Overview's band and its kanban view draw the
 * same card). Columns with no threads are left out. It is the band's fixed-height
 * layout: the columns sit side by side and each scrolls inside; the Overview
 * pane uses `StatusBoard`, which scrolls one way only.
 */
export function StatusColumns({ threads, selectedId, onOpen, onTasks, onStatus, listMaxHeight, rowRef, rowSx }: Props) {
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
                    <Box sx={{ mb: 0.75 }}><StatusHeading label={t(`overview.group.${g.key}`)} status={g.key} count={g.list.length}
                        onClick={onStatus && ((e) => { e.stopPropagation(); onStatus(g.key, e.currentTarget); })} /></Box>
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.75, overflowY: 'auto', minHeight: 0, ...(listMaxHeight == null ? { flex: 1 } : { maxHeight: listMaxHeight }) }}>
                        {g.list.map((th) => (
                            <ThreadCard key={th.id} thread={th} selectedId={selectedId} onOpen={onOpen} onTasks={onTasks} />
                        ))}
                    </Box>
                </Box>
            ))}
            {finished > 0 && (
                <Box sx={{ flex: '0 0 auto', minWidth: 120 }}>
                    <StatusHeading label={t('overview.group.resolved')} status="resolved" count={finished}
                        onClick={onStatus && ((e) => { e.stopPropagation(); onStatus('resolved', e.currentTarget); })} />
                </Box>
            )}
        </Box>
    );
}
