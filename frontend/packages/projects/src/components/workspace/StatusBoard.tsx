import { Box } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread } from '../../client';
import { STATUS_ORDER } from '../project/threadStatus';
import { StatusHeading } from './StatusGroups';
import { ThreadCard } from './StatusColumns';

interface Props {
    threads: Thread[];
    selectedId?: string;
    onOpen: (id: string) => void;
    /** Opens the thread's running tasks from its card's count pill. */
    onTasks?: (threads: Thread[], anchor: HTMLElement) => void;
    /** Opens a lane's thread list from its status heading. */
    onStatus?: (status: string, anchor: HTMLElement) => void;
    /** The pinned thread ids, with `onTogglePin`; without both, cards cannot be pinned. They draw above every status. */
    pinnedIds?: string[];
    onTogglePin?: (id: string) => void;
}

// As many equal tracks as fit; a pane narrower than one track gets one full-width track.
const TRACKS = 'repeat(auto-fill, minmax(min(220px, 100%), 1fr))';
// Wide enough for a lane's heading to sit beside its cards instead of above them.
const RAIL_AT = 640;

/**
 * Threads as a board that only ever scrolls vertically: it takes the pane's
 * width, never more, and grows downward. Empty statuses are left out.
 *
 * Pinned threads draw as one section above every status, newest pin first.
 *
 * A lane per status, full width, its cards flowing left to right in as many
 * tracks as the width holds and wrapping onto further lines; in a wide pane the
 * heading moves to the left of its cards. A narrow pane is one track, a wide
 * one many, with nothing to choose.
 */
export function StatusBoard({ threads, selectedId, onOpen, onTasks, onStatus, pinnedIds, onTogglePin }: Props) {
    const { t } = useTranslation('projects');
    const pinned = pinnedIds != null && onTogglePin != null ? new Set(pinnedIds) : undefined;
    const toggle = onTogglePin;
    const cards = (list: Thread[]) => list.map((th) => (
        <ThreadCard key={th.id} thread={th} selectedId={selectedId} onOpen={onOpen} onTasks={onTasks} stacked
            pinned={pinned?.has(th.id)} onTogglePin={pinned != null && toggle != null ? () => toggle(th.id) : undefined} />
    ));
    // Pinned threads leave their statuses for one section above them all, in the stored order (newest pin first).
    const pinnedList = pinnedIds != null ? pinnedIds.map((id) => threads.find((th) => th.id === id)).filter((th): th is Thread => th != null) : [];
    const groups = STATUS_ORDER.map((s) => ({ key: s, status: s, label: t(`overview.group.${s}`), list: threads.filter((th) => th.status === s && !pinned?.has(th.id)) }))
        .filter((g) => g.list.length > 0);
    const sections = [
        ...(pinnedList.length > 0 ? [{ key: 'pinned', status: 'pinned', label: t('overview.pinned'), list: pinnedList }] : []),
        ...groups,
    ];
    if (sections.length === 0) return null;
    return (
        <Box data-board sx={{
            containerType: 'inline-size', boxSizing: 'border-box', width: '100%', minWidth: 0, overflowX: 'hidden', p: 2,
            display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', alignItems: 'start',
        }}>
            {sections.map((s, i) => (
                <Box key={s.key} role="group" aria-label={s.label} sx={{
                    minWidth: 0, py: 1.5, pt: i === 0 ? 0 : 1.5, borderTop: i === 0 ? 0 : 1, borderColor: 'divider',
                    display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', columnGap: 2, rowGap: 0.75, alignItems: 'start',
                    [`@container (min-width: ${RAIL_AT}px)`]: { gridTemplateColumns: '150px minmax(0, 1fr)' },
                }}>
                    <Box sx={{ pt: 0.5 }}><StatusHeading label={s.label} status={s.status} count={s.list.length}
                        onClick={onStatus && ((e) => { e.stopPropagation(); onStatus(s.status, e.currentTarget); })} /></Box>
                    <Box sx={{ minWidth: 0, display: 'grid', gap: 0.75, gridTemplateColumns: TRACKS }}>{cards(s.list)}</Box>
                </Box>
            ))}
        </Box>
    );
}
