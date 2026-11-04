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
    /** `column`: finished work is a column too. `count`: only its heading stays, the list is where to read it. */
    resolved: 'column' | 'count';
    /** Cards a column shows before it scrolls; unset, a column fills the height it is given. */
    listMaxHeight?: number;
    rowRef?: Ref<HTMLDivElement>;
    /** Narrow cards: the time and badges sit under the title, so the title keeps its width. */
    stacked?: boolean;
    /** The row's own size: a fixed height, or room to fill. */
    rowSx?: object;
}

/**
 * Threads as a board: one column per status, what needs the user first, each
 * thread an outlined card (the Overview's band and its kanban view draw the
 * same columns). Columns with no threads are left out.
 */
export function StatusColumns({ threads, selectedId, onOpen, resolved, listMaxHeight, stacked, rowRef, rowSx }: Props) {
    const { t, i18n } = useTranslation('projects');
    const groups = STATUS_ORDER.map((s) => ({ key: s, list: threads.filter((th) => th.status === s) })).filter((g) => g.list.length > 0);
    const columns = resolved === 'column' ? groups : groups.filter((g) => g.key !== 'resolved');
    const finished = resolved === 'count' ? groups.find((g) => g.key === 'resolved')?.list.length ?? 0 : 0;
    if (columns.length === 0) return null;
    return (
        <Box ref={rowRef} sx={{ display: 'flex', gap: 1.5, px: 2, pb: 1.5, overflowX: 'auto', ...rowSx }}>
            {columns.map((g) => (
                <Box key={g.key} role="group" aria-label={t(`overview.group.${g.key}`)} sx={{ flex: '1 1 0', minWidth: 220, maxWidth: 360, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                    <Box sx={{ mb: 0.75 }}><StatusHeading label={t(`overview.group.${g.key}`)} status={g.key} count={g.list.length} /></Box>
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.75, overflowY: 'auto', minHeight: 0, ...(listMaxHeight == null ? { flex: 1 } : { maxHeight: listMaxHeight }) }}>
                        {g.list.map((th) => (
                            <ListRow key={th.id} outlined stacked={stacked} selected={th.id === selectedId} onClick={() => onOpen(th.id)}
                                title={th.title} subtitle={<ThreadSubtitle thread={th} />} badges={<ThreadBadges thread={th} />} meta={timeAgo(th.updated_at, i18n.language)} />
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
