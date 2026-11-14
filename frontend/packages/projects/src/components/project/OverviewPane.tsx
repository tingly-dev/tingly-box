import { useState } from 'react';
import { Box, ButtonBase, Tab, Tabs, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread } from '../../client';
import { timeAgo } from '../../utils/format';
import { STATUS_ORDER, ThreadBadges, needsUser } from './threadStatus';
import { ThreadRequestSubtitle, ThreadRequestsStrip } from './ThreadRequests';
import { StatusGroups } from '../workspace/StatusGroups';
import { StatusBoard } from '../workspace/StatusBoard';
import { useThreadsView, ViewSwitch, type ThreadsView } from '../workspace/ViewSwitch';
import { usePinnedThreads } from '../../hooks/usePinnedThreads';
import { Library } from './Library';
import { ProjectLinksCard } from './LinksCard';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

function ThreadGroups({ threads, selectedId, onOpen, onTasks, onStatus, view, pinnedIds, onTogglePin }: { threads: Thread[]; selectedId?: string; onOpen: (id: string) => void; /** Opens the thread's running tasks from its count pill. */ onTasks?: (threads: Thread[], anchor: HTMLElement) => void; /** Opens a status group's thread list from its heading. */ onStatus?: (status: string, anchor: HTMLElement) => void; view: ThreadsView; pinnedIds: string[]; onTogglePin: (id: string) => void }) {
    const { t, i18n } = useTranslation('projects');
    if (view !== 'list') {
        // The board takes the pane's width and grows downward: the pane scrolls vertically, never sideways.
        return threads.length === 0
            ? <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>{t('overview.noThreads')}</Typography>
            : <StatusBoard threads={threads} selectedId={selectedId} onOpen={onOpen} onTasks={onTasks} onStatus={onStatus} pinnedIds={pinnedIds} onTogglePin={onTogglePin} />;
    }
    const groups = STATUS_ORDER.map((status) => ({
        key: status, label: t(`overview.group.${status}`), collapsed: status === 'resolved',
    }));
    const rows = threads.map((th) => ({
        id: th.id, group: th.status, title: th.title, selected: th.id === selectedId,
        subtitle: <ThreadRequestSubtitle thread={th} />, footer: <ThreadRequestsStrip thread={th} onOpen={onOpen} />, badges: <ThreadBadges thread={th} onTasks={onTasks} />, meta: timeAgo(th.updated_at, i18n.language),
    }));
    return <StatusGroups groups={groups} rows={rows} onOpen={onOpen} onGroupClick={onStatus && ((key, _label, anchor) => onStatus(key, anchor))} empty={t('overview.noThreads')} />;
}

export type OverviewTab = 'threads' | 'library';

/** What the Threads view's toolbar says about the list: how many, and the tasks they run. The view switch sits at its right. */
function ThreadsToolbar({ threads, onTasks, view, onView }: { threads: Thread[]; onTasks?: (threads: Thread[], anchor: HTMLElement) => void; view: ThreadsView; onView: (v: ThreadsView) => void }) {
    const { t } = useTranslation('projects');
    const busy = threads.filter((th) => th.running_tasks > 0);
    const running = busy.reduce((n, th) => n + th.running_tasks, 0);
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, px: 1.5, py: 0.75, minHeight: 40, borderBottom: 1, borderColor: 'divider' }}>
            <Box sx={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'center', gap: 1.5, overflow: 'hidden' }}>
                <Typography noWrap sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{t('overview.threadCount', { count: threads.length })}</Typography>
                {running > 0 && (
                    <ButtonBase disabled={!onTasks} onClick={(e) => onTasks?.(busy, e.currentTarget)} title={t('rail.tasksInThreads', { tasks: running, count: busy.length })}
                        sx={{ minWidth: 0, px: 0.75, py: 0.25, borderRadius: radius.chip, fontSize: fontSizes.sm, color: 'text.secondary', border: 1, borderColor: 'divider', '&:hover': { bgcolor: 'action.hover' } }}>
                        <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{t('thread.tasksBadge', { count: running })}</Box>
                    </ButtonBase>
                )}
            </Box>
            <Box sx={{ flexShrink: 0 }}><ViewSwitch view={view} onChange={onView} /></Box>
        </Box>
    );
}

export function OverviewPane({ projectId, threads, onOpen, onTasks, onStatus, libraryKey, selectedId, initialTab = 'threads' }: { projectId: string; threads: Thread[]; onOpen: (id: string) => void; /** Opens a thread's running tasks from its card's count pill, or all of them from the toolbar's chip. */ onTasks?: (threads: Thread[], anchor: HTMLElement) => void; /** Opens a status's thread list from its group heading or column. */ onStatus?: (status: string, anchor: HTMLElement) => void; libraryKey: number; selectedId?: string; initialTab?: OverviewTab }) {
    const { t } = useTranslation('projects');
    const [tab, setTab] = useState<OverviewTab>(initialTab);
    const waiting = threads.filter((th) => needsUser(th.status)).length;
    const [view, setView] = useThreadsView();
    const [pinnedIds, togglePin] = usePinnedThreads(projectId);
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', pr: 1.5, borderBottom: 1, borderColor: 'divider' }}>
                <Tabs value={tab} onChange={(_, v) => setTab(v)} variant="scrollable" scrollButtons="auto" sx={{ flex: 1, minWidth: 0, px: 1, minHeight: 44, '& .MuiTab-root': { minHeight: 44, minWidth: 0 } }}>
                    <Tab value="threads" label={waiting > 0 ? t('overview.threadsWaiting', { count: waiting }) : t('overview.threads')} />
                    <Tab value="library" label={t('overview.library')} />
                </Tabs>
            </Box>
            {tab === 'threads' && <ThreadsToolbar threads={threads} onTasks={onTasks} view={view} onView={setView} />}
            <Box sx={{ flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden' }}>
                {tab === 'threads' ? (
                    <>
                        <ThreadGroups threads={threads} selectedId={selectedId} onOpen={onOpen} onTasks={onTasks} onStatus={onStatus} view={view} pinnedIds={pinnedIds} onTogglePin={togglePin} />
                        {/* What the project's threads touched on GitHub: the whole project's, so under the list rather than in any one thread. */}
                        <Box sx={{ px: 1.5, pb: 1.5 }}><ProjectLinksCard threads={threads} /></Box>
                    </>
                ) : <Library key={projectId} projectId={projectId} refreshKey={libraryKey} />}
            </Box>
        </Box>
    );
}
