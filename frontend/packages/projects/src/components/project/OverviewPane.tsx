import { useState } from 'react';
import { Box, Tab, Tabs, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Thread } from '../../client';
import { timeAgo } from '../../utils/format';
import { STATUS_ORDER, ThreadBadges, ThreadSubtitle, needsUser } from './threadStatus';
import { StatusGroups } from '../workspace/StatusGroups';
import { StatusBoard } from '../workspace/StatusBoard';
import { useThreadsView, ViewSwitch, type ThreadsView } from '../workspace/ViewSwitch';
import { Library } from './Library';

function ThreadGroups({ threads, selectedId, onOpen, view }: { threads: Thread[]; selectedId?: string; onOpen: (id: string) => void; view: ThreadsView }) {
    const { t, i18n } = useTranslation('projects');
    if (view !== 'list') {
        // The board takes the pane's width and grows downward: the pane scrolls vertically, never sideways.
        return threads.length === 0
            ? <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>{t('overview.noThreads')}</Typography>
            : <StatusBoard threads={threads} selectedId={selectedId} onOpen={onOpen} orientation={view} />;
    }
    const groups = STATUS_ORDER.map((status) => ({
        key: status, label: t(`overview.group.${status}`), collapsed: status === 'resolved',
    }));
    const rows = threads.map((th) => ({
        id: th.id, group: th.status, title: th.title, selected: th.id === selectedId,
        subtitle: <ThreadSubtitle thread={th} />, badges: <ThreadBadges thread={th} />, meta: timeAgo(th.updated_at, i18n.language),
    }));
    return <StatusGroups groups={groups} rows={rows} onOpen={onOpen} empty={t('overview.noThreads')} />;
}

export function OverviewPane({ projectId, threads, onOpen, libraryKey, selectedId, initialTab = 'threads' }: { projectId: string; threads: Thread[]; onOpen: (id: string) => void; libraryKey: number; selectedId?: string; initialTab?: 'threads' | 'library' }) {
    const { t } = useTranslation('projects');
    const [tab, setTab] = useState<'threads' | 'library'>(initialTab);
    const waiting = threads.filter((th) => needsUser(th.status)).length;
    const [view, setView] = useThreadsView();
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', pr: 1.5, borderBottom: 1, borderColor: 'divider' }}>
                <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ flex: 1, minWidth: 0, px: 1, minHeight: 44, '& .MuiTab-root': { minHeight: 44 } }}>
                    <Tab value="threads" label={waiting > 0 ? t('overview.threadsWaiting', { count: waiting }) : t('overview.threads')} />
                    <Tab value="library" label={t('overview.library')} />
                </Tabs>
                {tab === 'threads' && <ViewSwitch view={view} onChange={setView} />}
            </Box>
            <Box sx={{ flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden' }}>
                {tab === 'threads' ? <ThreadGroups threads={threads} selectedId={selectedId} onOpen={onOpen} view={view} /> : <Library key={projectId} projectId={projectId} refreshKey={libraryKey} />}
            </Box>
        </Box>
    );
}
