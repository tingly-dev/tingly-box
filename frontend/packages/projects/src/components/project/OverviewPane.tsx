import { useState, type ReactNode } from 'react';
import { Box, Tab, Tabs, Typography } from '@mui/material';
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

function ThreadGroups({ threads, selectedId, onOpen, onTasks, onStatus, view, pinnedIds, onTogglePin }: { threads: Thread[]; selectedId?: string; onOpen: (id: string) => void; /** Opens the thread's running tasks from its count pill. */ onTasks?: (threads: Thread[], anchor: HTMLElement) => void; /** Opens a status group's thread list from its heading. */ onStatus?: (status: string, anchor: HTMLElement) => void; view: ThreadsView; pinnedIds: string[]; onTogglePin: (id: string) => void }) {
    const { t, i18n } = useTranslation('projects');
    if (view !== 'list') {
        // The board takes the pane's width and grows downward: the pane scrolls vertically, never sideways.
        return threads.length === 0
            ? <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>{t('overview.noThreads')}</Typography>
            : <StatusBoard threads={threads} selectedId={selectedId} onOpen={onOpen} onTasks={onTasks} onStatus={onStatus} orientation={view} pinnedIds={pinnedIds} onTogglePin={onTogglePin} />;
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

export type OverviewTab = 'threads' | 'library' | 'status';

export function OverviewPane({ projectId, threads, onOpen, onTasks, onStatus, libraryKey, selectedId, initialTab = 'threads', status }: { projectId: string; threads: Thread[]; onOpen: (id: string) => void; /** Opens a thread's running tasks from its count pill (band, board and list alike). */ onTasks?: (threads: Thread[], anchor: HTMLElement) => void; /** Opens a status's thread list from its heading or closed count. */ onStatus?: (status: string, anchor: HTMLElement) => void; libraryKey: number; selectedId?: string; initialTab?: OverviewTab; /** The coordinator's status; without it the tab is not offered. */ status?: ReactNode }) {
    const { t } = useTranslation('projects');
    const [tab, setTab] = useState<OverviewTab>(initialTab);
    const waiting = threads.filter((th) => needsUser(th.status)).length;
    const [view, setView] = useThreadsView();
    const [pinnedIds, togglePin] = usePinnedThreads(projectId);
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', pr: 1.5, borderBottom: 1, borderColor: 'divider' }}>
                <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ flex: 1, minWidth: 0, px: 1, minHeight: 44, '& .MuiTab-root': { minHeight: 44 } }}>
                    <Tab value="threads" label={waiting > 0 ? t('overview.threadsWaiting', { count: waiting }) : t('overview.threads')} />
                    <Tab value="library" label={t('overview.library')} />
                    {status != null && <Tab value="status" label={t('overview.status')} />}
                </Tabs>
                {tab === 'threads' && <ViewSwitch view={view} onChange={setView} />}
            </Box>
            <Box sx={{ flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden' }}>
                {tab === 'threads' ? <ThreadGroups threads={threads} selectedId={selectedId} onOpen={onOpen} onTasks={onTasks} onStatus={onStatus} view={view} pinnedIds={pinnedIds} onTogglePin={togglePin} /> : tab === 'status' && status != null ? status : <Library key={projectId} projectId={projectId} refreshKey={libraryKey} />}
            </Box>
        </Box>
    );
}
