import { Box, Chip, type ChipProps } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Branch, CheckCircle, CircleDashed, HandStop, Merge, Spinner } from '../icons';
import type { Thread } from '../../client';

/** Overview groups, in the order the Overview shows them: what needs the
 * user first. */
export const STATUS_ORDER = ['waiting', 'ready', 'working', 'idle', 'resolved'] as const;
export type ThreadStatus = (typeof STATUS_ORDER)[number];

const COLOR: Record<ThreadStatus, ChipProps['color']> = {
    waiting: 'warning',
    ready: 'success',
    working: 'info',
    idle: 'default',
    resolved: 'default',
};

export function statusColor(s: string): ChipProps['color'] {
    return COLOR[s as ThreadStatus] ?? 'default';
}

function StatusIcon({ status }: { status: string }) {
    const sx = { fontSize: 14 };
    switch (status) {
        case 'working':
            return <Spinner sx={{ ...sx, animation: 'spin 1s linear infinite' }} />;
        case 'waiting':
            return <HandStop sx={sx} />;
        case 'ready':
            return <Merge sx={sx} />;
        case 'resolved':
            return <CheckCircle sx={sx} />;
        default:
            return <CircleDashed sx={sx} />;
    }
}

export function ThreadStatusChip({ thread, size = 'small' }: { thread: Pick<Thread, 'status'>; size?: 'small' | 'medium' }) {
    const { t } = useTranslation('projects');
    return (
        <Chip
            size={size}
            color={statusColor(thread.status)}
            variant={thread.status === 'resolved' || thread.status === 'idle' ? 'outlined' : 'filled'}
            icon={<Box component="span" sx={{ display: 'inline-flex', pl: 0.5 }}><StatusIcon status={thread.status} /></Box>}
            label={t(`thread.status.${thread.status}`)}
            sx={{ height: 22, fontSize: 12, '& .MuiChip-label': { px: 0.75 } }}
        />
    );
}

const mono = { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' } as const;

/** One line of backend text where `backticks` mark a literal (a command, a
 * path): the literal shows as code, not as raw backticks. */
export function InlineCode({ text }: { text: string }) {
    const parts = text.split(/`([^`]+)`/);
    return (
        <>
            {parts.map((p, i) => (i % 2 === 1
                ? <Box key={i} component="code" sx={{ ...mono, fontSize: '0.95em', px: 0.5, borderRadius: 0.75, bgcolor: 'action.hover', color: 'text.primary' }}>{p}</Box>
                : p))}
        </>
    );
}

/** The line under a thread's title: what it waits on, where its work landed,
 * or which branch it is on. */
export function ThreadSubtitle({ thread }: { thread: Thread }) {
    const { t } = useTranslation('projects');
    if (thread.status === 'waiting' && thread.wait_reason) return <InlineCode text={thread.wait_reason} />;
    if (thread.merged_at) {
        return (
            <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, color: 'success.main' }}>
                <Merge sx={{ fontSize: 13 }} />{t('thread.mergedInto', { base: thread.base_branch })}
            </Box>
        );
    }
    if (thread.branch) {
        return (
            <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, ...mono }}>
                <Branch sx={{ fontSize: 13 }} />{thread.branch}{thread.commits_ahead > 0 ? ` · ${t('thread.commits', { count: thread.commits_ahead })}` : ''}
            </Box>
        );
    }
    return <>{(thread.last_report || '').split('\n')[0] || t('thread.noRepo')}</>;
}

/** A quiet group heading for the Overview: a status dot, the name, a count. */
export function StatusHeading({ status, count }: { status: ThreadStatus; count: number }) {
    const { t } = useTranslation('projects');
    const c = statusColor(status);
    return (
        <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75 }}>
            <Box sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: c === 'default' ? 'text.disabled' : `${c}.main` }} />
            <Box component="span" sx={{ fontSize: 12, fontWeight: 600, color: c === 'default' ? 'text.secondary' : `${c}.main` }}>{t(`overview.group.${status}`)}</Box>
            <Box component="span" sx={{ fontSize: 12, color: 'text.disabled' }}>{count}</Box>
        </Box>
    );
}
