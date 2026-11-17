import { useState, type ReactNode } from 'react';
import { Box, Button, ButtonBase, Checkbox, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import type { Thread } from '../../client';
import { usePinnedThreads } from '../../hooks/usePinnedThreads';
import { timeAgo } from '../../utils/format';
import { Checklist } from '../icons';
import { STATUS_ORDER, ThreadBadges } from '../project/threadStatus';
import { ThreadRequestSubtitle, ThreadRequestsStrip } from '../project/ThreadRequests';
import { StatusBoard } from './StatusBoard';
import { StatusGroups, type StatusGroup } from './StatusGroups';
import { useThreadsView, ViewSwitch } from './ViewSwitch';

interface Props {
    /** Keeps the pins apart: a project's id, or the folder's. */
    scope: string;
    threads: Thread[];
    selectedId?: string;
    onOpen: (id: string) => void;
    /** Opens a thread's running tasks from its card's count pill, or all of them from the toolbar's chip. */
    onTasks?: (threads: Thread[], anchor: HTMLElement) => void;
    /** Opens a group's list from its heading: the group's key, its label, the heading. */
    onGroup?: (key: string, label: string, anchor: HTMLElement) => void;
    /** The groups, in order, with the labels they read; by default a project's statuses. */
    groups?: StatusGroup[];
    /** Which group a thread stands in; by default its status. */
    groupOf?: (thread: Thread) => string;
    /** The line under a row's title; by default what its request or report says. */
    subtitle?: (thread: Thread) => ReactNode;
    /** The toolbar's count: "6 threads", "3 sessions". */
    countLabel: (count: number) => string;
    /** What the list says when there is nothing in it. */
    empty: string;
    /** Given, the toolbar offers a select mode over the non-archived threads and the bar resolves or archives the picked ones. */
    bulk?: {
        resolve: (ids: string[]) => void;
        archive: (ids: string[]) => void;
    };
}

/** What the toolbar says about the list: how many, and the tasks they run — or,
 * in the select mode, what is picked and what the picked ones can do. */
function Toolbar({ threads, countLabel, onTasks, view, onView, canSelect, selecting, picked, onToggleSelecting, onPick, onResolve, onArchive }: Pick<Props, 'threads' | 'countLabel' | 'onTasks'> & { view: ReturnType<typeof useThreadsView>[0]; onView: ReturnType<typeof useThreadsView>[1]; canSelect: boolean; selecting: boolean; picked: number; onToggleSelecting: () => void; onPick: (all: boolean) => void; onResolve: () => void; onArchive: () => void }) {
    const { t } = useTranslation('projects');
    const busy = threads.filter((th) => th.running_tasks > 0);
    const running = busy.reduce((n, th) => n + th.running_tasks, 0);
    const pickable = threads.filter((th) => !th.archived_at);
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, px: 1.5, py: 0.75, minHeight: 40, borderBottom: 1, borderColor: 'divider' }}>
            {selecting ? (
                <>
                    <Typography noWrap sx={{ flex: 1, minWidth: 0, fontSize: fontSizes.sm, color: 'text.secondary' }}>{t('overview.selectedCount', { count: picked })}</Typography>
                    <Button size="small" disabled={picked === 0} onClick={() => onResolve()}>{t('overview.bulkResolve')}</Button>
                    <Button size="small" disabled={picked === 0} onClick={() => onArchive()}>{t('overview.bulkArchive')}</Button>
                    <Button size="small" onClick={() => onPick(picked < pickable.length)}>{picked < pickable.length ? t('overview.selectAll') : t('overview.selectNone')}</Button>
                    <ButtonBase aria-label={t('common.cancel')} title={t('common.cancel')} onClick={onToggleSelecting}
                        sx={{ width: 24, height: 24, borderRadius: radius.chip, fontSize: fontSizes.sm, color: 'text.secondary', '&:hover': { bgcolor: 'action.hover' } }}>✕</ButtonBase>
                </>
            ) : (
                <>
                    <Box sx={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'center', gap: 1.5, overflow: 'hidden' }}>
                        <Typography noWrap sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{countLabel(threads.length)}</Typography>
                        {running > 0 && (
                            <ButtonBase disabled={!onTasks} onClick={(e) => onTasks?.(busy, e.currentTarget)} title={t('rail.tasksInThreads', { tasks: running, count: busy.length })}
                                sx={{ minWidth: 0, px: 0.75, py: 0.25, borderRadius: radius.chip, fontSize: fontSizes.sm, color: 'text.secondary', border: 1, borderColor: 'divider', '&:hover': { bgcolor: 'action.hover' } }}>
                                <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{t('thread.tasksBadge', { count: running })}</Box>
                            </ButtonBase>
                        )}
                    </Box>
                    {canSelect && pickable.length > 0 && (
                        <ButtonBase aria-label={t('overview.select')} title={t('overview.select')} onClick={onToggleSelecting}
                            sx={{ width: 28, height: 28, borderRadius: radius.chip, color: 'text.secondary', '&:hover': { bgcolor: 'action.hover' } }}><Checklist fontSize="small" /></ButtonBase>
                    )}
                    <Box sx={{ flexShrink: 0 }}><ViewSwitch view={view} onChange={onView} /></Box>
                </>
            )}
        </Box>
    );
}

/**
 * A page's threads, one way wherever they are listed: a toolbar (how many, the
 * tasks they run, `List | Board`), then the list or the board, with pins and
 * the request each thread waits on answered on its row. A project's Overview and
 * a working directory's sessions both draw this; they differ in the groups,
 * the subtitle and the words, never in how the list behaves.
 */
export function ThreadsView({ scope, threads, selectedId, onOpen, onTasks, onGroup, groups, groupOf = (th) => (th.archived_at ? 'archived' : th.status), subtitle, countLabel, empty, bulk }: Props) {
    const { t, i18n } = useTranslation('projects');
    const [view, setView] = useThreadsView();
    const [pinnedIds, togglePin] = usePinnedThreads(scope);
    // The select mode and the picked threads. Archived ones are not pickable:
    // resolving or archiving them again says nothing.
    const [selecting, setSelecting] = useState(false);
    const [picked, setPicked] = useState<Set<string>>(new Set());
    const pickable = new Set(threads.filter((th) => !th.archived_at).map((th) => th.id));
    const togglePick = (id: string) => setPicked((p) => {
        const next = new Set(p);
        if (next.has(id)) next.delete(id); else next.add(id);
        return next;
    });
    const pick = (all: boolean) => setPicked(all ? new Set(pickable) : new Set());
    const exit = () => { setSelecting(false); setPicked(new Set()); };
    const act = (run: (ids: string[]) => void) => {
        const ids = [...picked].filter((id) => pickable.has(id));
        if (ids.length === 0) return;
        run(ids);
        exit();
    };
    const lanes: StatusGroup[] = groups ?? [...STATUS_ORDER.map((status) => ({ key: status, label: t(`overview.group.${status}`), collapsed: status === 'resolved' })), { key: 'archived', label: t('overview.group.archived'), collapsed: true }];
    const rows = threads.map((th) => ({
        id: th.id, group: groupOf(th), title: th.title, selected: th.id === selectedId,
        subtitle: subtitle ? subtitle(th) : <ThreadRequestSubtitle thread={th} />,
        footer: <ThreadRequestsStrip thread={th} onOpen={onOpen} />, badges: <ThreadBadges thread={th} onTasks={onTasks} />, meta: timeAgo(th.updated_at, i18n.language),
        onClick: selecting && pickable.has(th.id) ? () => togglePick(th.id) : undefined,
        end: selecting && pickable.has(th.id) ? <Checkbox size="small" checked={picked.has(th.id)} onChange={() => togglePick(th.id)} onClick={(e) => e.stopPropagation()} sx={{ p: 0.5 }} /> : undefined,
    }));
    const labelOf = (key: string) => lanes.find((g) => g.key === key)?.label ?? key;
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
            <Toolbar threads={threads} countLabel={countLabel} onTasks={onTasks} view={view} onView={setView} canSelect={!!bulk}
                selecting={selecting} picked={picked.size} onToggleSelecting={() => (selecting ? exit() : setSelecting(true))}
                onPick={pick} onResolve={() => act(bulk!.resolve)} onArchive={() => act(bulk!.archive)} />
            <Box sx={{ flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden' }}>
                {view === 'list'
                    ? <StatusGroups groups={lanes} rows={rows} onOpen={onOpen} onGroupClick={onGroup && ((key, label, anchor) => onGroup(key, label, anchor))} empty={empty} />
                    // The board takes the pane's width and grows downward: the pane scrolls vertically, never sideways.
                    : threads.length === 0
                        ? <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>{empty}</Typography>
                        : <StatusBoard threads={threads} selectedId={selectedId} onOpen={onOpen} onTasks={onTasks} groups={lanes} groupOf={groupOf} subtitle={subtitle}
                            onStatus={onGroup && ((key, anchor) => onGroup(key, labelOf(key), anchor))} pinnedIds={pinnedIds} onTogglePin={togglePin}
                            selecting={selecting} picked={picked} onTogglePick={togglePick} />}
            </Box>
        </Box>
    );
}
