import { Box } from '@mui/material';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { getReadableAccent } from '@tingly/ui/theme/status';
import { useTranslation } from 'react-i18next';
import { Branch, CheckCircle, CircleDashed, Merge, Spinner } from '../icons';
import type { Thread } from '../../client';
import { radius } from '@tingly/ui/theme/radius';

/** Overview groups, in the order the Overview shows them: what needs the
 * user first. */
export const STATUS_ORDER = ['waiting', 'ready', 'working', 'idle', 'resolved'] as const;
export type ThreadStatus = (typeof STATUS_ORDER)[number];

/** How a state reads: amber when it needs the user, red when it failed,
 * otherwise neutral, told apart by its mark and word. */
export type StateTone = 'attention' | 'failed' | 'neutral';

export function stateTone(status: string): StateTone {
    return status === 'waiting' ? 'attention' : status === 'failed' ? 'failed' : 'neutral';
}

const toneOf = (theme: Parameters<typeof getReadableAccent>[0], tone: StateTone) =>
    tone === 'attention' ? getReadableAccent(theme, 'warning') : tone === 'failed' ? getReadableAccent(theme, 'error') : theme.palette.text.secondary;

/** The text colour of a state: amber, red, or the secondary text colour. */
export const stateColor = (status: string) => (theme: Parameters<typeof getReadableAccent>[0]) => toneOf(theme, stateTone(status));

const dot = { width: 8, height: 8, borderRadius: '50%', boxSizing: 'border-box' } as const;

/** The mark of a state, in a 14px slot: a filled dot for what needs the user
 * (amber) or failed (red), a spinner for work, a ring for ready, a check for
 * done. */
export function ThreadStateMark({ status }: { status: string }) {
    const sx = { fontSize: 14 };
    let mark;
    switch (status) {
        case 'working':
            mark = <Spinner sx={{ ...sx, animation: 'spin 1s linear infinite' }} />;
            break;
        case 'ready':
            mark = <Box component="span" sx={{ ...dot, border: '1.5px solid currentColor' }} />;
            break;
        case 'resolved':
            mark = <CheckCircle sx={sx} />;
            break;
        case 'waiting':
        case 'failed':
            mark = <Box component="span" sx={{ ...dot, bgcolor: 'currentColor' }} />;
            break;
        default:
            mark = <CircleDashed sx={sx} />;
    }
    return (
        <Box component="span" aria-hidden sx={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: 14, height: 14, flexShrink: 0, color: stateColor(status) }}>
            {mark}
        </Box>
    );
}

/** One state, one look, wherever a thread's status shows: its mark (the only
 * coloured part) and its word unless `compact` (the word then sits in the
 * title attribute). */
export function ThreadState({ status, compact }: { status: string; compact?: boolean }) {
    const { t } = useTranslation('projects');
    const word = t(`thread.status.${status}`);
    return (
        <Box
            component="span"
            data-status={status}
            title={compact ? word : undefined}
            sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, flexShrink: 0, fontSize: fontSizes.sm, fontWeight: 500, whiteSpace: 'nowrap', color: status === 'waiting' ? 'text.primary' : 'text.secondary' }}
        >
            <ThreadStateMark status={status} />
            {!compact && word}
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
                ? <Box key={i} component="code" sx={{ ...mono, fontSize: '0.95em', px: 0.5, borderRadius: radius.chip, bgcolor: 'action.hover', color: 'text.primary' }}>{p}</Box>
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
    display: 'inline-flex', alignItems: 'center', gap: 0.5, height: 22, px: 0.875, borderRadius: radius.chip, flexShrink: 0,
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
                sx={pill}>
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
