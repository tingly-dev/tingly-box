import { alpha, Box, type ChipProps } from '@mui/material';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { getReadableAccent } from '@tingly/ui/theme/status';
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

export function StatusIcon({ status }: { status: string }) {
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

/** A tone's readable text colour, or the secondary text colour for none. */
export const toneColor = (color: ChipProps['color']) => (theme: Parameters<typeof getReadableAccent>[0]) =>
    color === 'default' || color === 'primary' || color === 'secondary' || !color
        ? theme.palette.text.secondary
        : getReadableAccent(theme, color);

/** A thread's status as quiet text: its icon and name in the status colour,
 * no fill, so the title stays the subject. */
export function ThreadStatusChip({ thread }: { thread: Pick<Thread, 'status'> }) {
    const { t } = useTranslation('projects');
    return (
        <Box
            component="span"
            data-status={thread.status}
            sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, flexShrink: 0, fontSize: fontSizes.sm, fontWeight: 500, whiteSpace: 'nowrap', color: toneColor(statusColor(thread.status)) }}
        >
            <StatusIcon status={thread.status} />
            {t(`thread.status.${thread.status}`)}
        </Box>
    );
}

const mono = { fontFamily: fontMono } as const;

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

/** The line under a thread's title: what it waits on, else what it last
 * reported, else where it works. Branch and merge state live in its badges. */
export function ThreadSubtitle({ thread }: { thread: Thread }) {
    const { t } = useTranslation('projects');
    if (thread.status === 'waiting' && thread.wait_reason) return <InlineCode text={thread.wait_reason} />;
    const report = (thread.last_report || '').split('\n').find((l) => l.trim() !== '');
    if (report) return <InlineCode text={report.replace(/^#+\s*/, '')} />;
    if (thread.branch) return <>{thread.branch}</>;
    return <>{t('thread.noRepo')}</>;
}

const pill = {
    display: 'inline-flex', alignItems: 'center', gap: 0.5, height: 22, px: 0.875, borderRadius: 0.75, flexShrink: 0,
    fontSize: fontSizes.sm, fontWeight: 500, whiteSpace: 'nowrap', border: 1, borderColor: 'divider', color: 'text.secondary',
    '& svg': { fontSize: 13 },
} as const;

/** A thread's work at a glance, as small pills: its commits, or that it was
 * merged. Nothing for a thread without a branch. */
export function ThreadBadges({ thread }: { thread: Thread }) {
    const { t } = useTranslation('projects');
    if (thread.merged_at) {
        return (
            <Box component="span" title={t('thread.mergedInto', { base: thread.base_branch })}
                sx={{ ...pill, borderColor: 'transparent', color: toneColor('success'), bgcolor: (th) => alpha(th.palette.success.main, 0.1) }}>
                <Merge />{t('thread.mergedBadge')}
            </Box>
        );
    }
    if (!thread.branch || thread.commits_ahead === 0) return null;
    return (
        <Box component="span" title={thread.branch} sx={pill}>
            <Branch />{t('thread.commits', { count: thread.commits_ahead })}
        </Box>
    );
}
