import {Box, ButtonBase, Chip, Stack, Tooltip, Typography} from '@mui/material';
import type {Theme} from '@mui/material/styles';
import {Fragment, useMemo, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {fontMono, fontSizes} from '@/theme/fonts';
import {getReadableAccent} from '@/theme/status';
import type {TranscriptBlock} from './deskUtils';
import type {TrajectoryAnchor, TrajectoryFilter, TrajectoryKind, TrajectoryRow} from './trajectoryRows';
import {matchesFilter, trajectoryRows, trajectorySummary} from './trajectoryRows';

interface TrajectoryProps {
    blocks: TranscriptBlock[];
    working: boolean;
    // The request still waiting on the user, read as such rather than unanswered.
    pendingRequestId?: string;
    // The session's folder; paths under it are shown relative to it.
    project?: string;
    // Shows the row's message in the chat.
    onOpen: (anchor: TrajectoryAnchor) => void;
}

// The kind badge says what a row is; time order says what it belongs to, so
// only a subagent's own steps are indented (that nesting is real).
const kindColor = (theme: Theme, kind: TrajectoryKind): string => {
    switch (kind) {
        case 'user':
            return theme.palette.primary.main;
        case 'agent':
            return getReadableAccent(theme, 'info');
        case 'approval':
        case 'ask':
            return getReadableAccent(theme, 'warning');
        case 'error':
            return getReadableAccent(theme, 'error');
        case 'reply':
            return theme.palette.text.primary;
        default:
            return theme.palette.text.secondary;
    }
};

const clock = (timestamp?: string): string => {
    if (!timestamp) return '';
    const d = new Date(timestamp);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
};

// relative shortens paths under the session's folder, the way the folder chip
// already names it: the rest of the path is what tells calls apart.
const relative = (text: string, project?: string): string => {
    if (!project) return text;
    const root = project.endsWith('/') ? project : `${project}/`;
    return text.split(root).join('');
};

const Trajectory = ({blocks, working, pendingRequestId, project, onOpen}: TrajectoryProps) => {
    const {t} = useTranslation();
    const rows = useMemo(() => trajectoryRows(blocks, working), [blocks, working]);
    const summary = useMemo(() => trajectorySummary(rows), [rows]);
    const [filter, setFilter] = useState<TrajectoryFilter | null>(null);
    const shown = filter ? rows.filter((r) => matchesFilter(r, filter)) : rows;
    // A turn's time is when its message was sent, also when a filter hides it.
    const turnTimes = useMemo(() => new Map(rows.filter((r) => r.time).map((r) => [r.turn, clock(r.time)])), [rows]);

    const kindLabel: Record<TrajectoryKind, string> = {
        user: t('desk.trajectoryKindUser', {defaultValue: 'You'}),
        tool: t('desk.trajectoryKindTool', {defaultValue: 'Tool'}),
        agent: t('desk.trajectoryKindAgent', {defaultValue: 'Agent'}),
        approval: t('desk.trajectoryKindApproval', {defaultValue: 'Approval'}),
        ask: t('desk.trajectoryKindAsk', {defaultValue: 'Question'}),
        error: t('desk.trajectoryKindError', {defaultValue: 'Error'}),
        note: t('desk.trajectoryKindNote', {defaultValue: 'Note'}),
        reply: t('desk.trajectoryKindReply', {defaultValue: 'Reply'}),
        system: t('desk.trajectoryKindSystem', {defaultValue: 'System'}),
    };

    const chips: {filter: TrajectoryFilter; label: string; count: number; hint?: string}[] = [
        {
            filter: 'files', label: t('desk.trajectoryFiles', {defaultValue: 'Files changed'}), count: summary.files.length,
            hint: t('desk.trajectoryFilesHint', {defaultValue: 'Files written by Edit / Write; changes made by shell commands are not counted'}),
        },
        {filter: 'commands', label: t('desk.trajectoryCommands', {defaultValue: 'Commands'}), count: summary.commands},
        {filter: 'failures', label: t('desk.trajectoryFailures', {defaultValue: 'Failures'}), count: summary.failures},
        {filter: 'requests', label: t('desk.trajectoryRequests', {defaultValue: 'Approvals & questions'}), count: summary.requests},
    ];

    if (rows.length === 0) {
        return (
            <Typography variant="body2" sx={{color: 'text.secondary', textAlign: 'center', py: 4}}>
                {t('desk.trajectoryEmpty', {defaultValue: 'Nothing to review yet.'})}
            </Typography>
        );
    }

    return (
        <Stack spacing={2}>
            {chips.some((c) => c.count > 0) && (
                <Stack direction="row" sx={{flexWrap: 'wrap', gap: 1}}>
                    {chips.filter((c) => c.count > 0).map((c) => {
                        const active = filter === c.filter;
                        const chip = (
                            <Chip
                                size="small"
                                variant={active ? 'filled' : 'outlined'}
                                color={active ? 'primary' : 'default'}
                                label={`${c.label} ${c.count}`}
                                onClick={() => setFilter(active ? null : c.filter)}
                                aria-pressed={active}
                            />
                        );
                        return c.hint ? <Tooltip key={c.filter} title={c.hint}>{chip}</Tooltip> : <Fragment key={c.filter}>{chip}</Fragment>;
                    })}
                </Stack>
            )}
            <Box role="list" aria-label={t('desk.viewTrajectory', {defaultValue: 'Trajectory'})}>
                {shown.map((r, i) => {
                    const turnStart = r.turn > 0 && (i === 0 || shown[i - 1].turn !== r.turn);
                    const time = turnTimes.get(r.turn);
                    return (
                        <Fragment key={i}>
                            {turnStart && (
                                <Stack direction="row" spacing={1} sx={{alignItems: 'center', mt: i === 0 ? 0 : 2, mb: 0.5}}>
                                    <Typography variant="caption" sx={{color: 'text.secondary', whiteSpace: 'nowrap'}}>
                                        {t('desk.trajectoryTurn', {defaultValue: 'Turn {{turn}}', turn: r.turn})}
                                        {time ? ` · ${time}` : ''}
                                    </Typography>
                                    <Box sx={{flex: 1, borderTop: 1, borderColor: 'divider'}}/>
                                </Stack>
                            )}
                            <ButtonBase
                                role="listitem"
                                onClick={() => onOpen(r.anchor)}
                                aria-label={`${kindLabel[r.kind]}: ${r.title}${r.detail ? ` ${r.detail}` : ''} — ${t('desk.trajectoryOpen', {defaultValue: 'show in chat'})}`}
                                sx={{
                                    display: 'grid', gridTemplateColumns: '72px minmax(0, 1fr) auto', columnGap: 1.5, alignItems: 'baseline',
                                    width: '100%', textAlign: 'left', px: 1, py: 0.625, pl: 1 + r.depth * 2.5, borderRadius: 1,
                                    '&:hover': {bgcolor: 'action.hover'},
                                }}
                            >
                                <Typography component="span" sx={{fontFamily: fontMono, fontSize: fontSizes.xs, fontWeight: 600, textTransform: 'uppercase', color: (theme) => kindColor(theme, r.kind), whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis'}}>
                                    {kindLabel[r.kind]}
                                </Typography>
                                <Typography component="span" variant="body2" noWrap sx={{color: r.kind === 'note' || r.kind === 'system' ? 'text.secondary' : 'text.primary', minWidth: 0}}>
                                    {r.kind === 'tool' || r.kind === 'agent' ? <Box component="span" sx={{fontWeight: 600}}>{r.title}</Box> : r.title}
                                    {r.detail && (
                                        <Box component="span" sx={{fontFamily: fontMono, fontSize: fontSizes.sm, color: 'text.secondary', ml: 1}}>{relative(r.detail, project)}</Box>
                                    )}
                                </Typography>
                                <Typography component="span" variant="caption" noWrap sx={{maxWidth: 200, color: (theme) => r.failed ? getReadableAccent(theme, 'error') : theme.palette.text.secondary}}>
                                    {r.kind === 'approval' || r.kind === 'ask'
                                        ? `→ ${r.outcome ?? ('request' in r.anchor && r.anchor.request === pendingRequestId
                                            ? t('desk.trajectoryWaiting', {defaultValue: 'waiting for you'})
                                            : t('desk.unanswered', {defaultValue: 'not answered'}))}`
                                        : r.kind === 'agent'
                                            ? {
                                                running: t('desk.taskRunning', {defaultValue: 'running'}),
                                                completed: t('desk.taskDone', {defaultValue: 'done'}),
                                                stopped: t('desk.taskStopped', {defaultValue: 'stopped'}),
                                                failed: t('desk.taskFailed', {defaultValue: 'failed'}),
                                            }[r.status ?? 'running']
                                            : r.kind === 'tool' && r.failed ? t('desk.trajectoryFailed', {defaultValue: 'failed'}) : ''}
                                </Typography>
                            </ButtonBase>
                        </Fragment>
                    );
                })}
            </Box>
        </Stack>
    );
};

export default Trajectory;
