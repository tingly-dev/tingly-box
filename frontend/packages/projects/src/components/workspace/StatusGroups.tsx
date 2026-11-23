import { useId, type ReactNode } from 'react';
import { Box, Collapse, Typography, useMediaQuery } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { DisclosureRow } from '../common/Disclosure';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { mobile } from '@tingly/ui/theme/mobile';
import { radius } from '@tingly/ui/theme/radius';
import { ListRow } from '../ListRow';
import { ThreadStateMark } from '../project/threadStatus';

/** One group of the list: what needs the user comes first. */
export interface StatusGroup {
    key: string;
    label: string;
    /** Folded until the user opens it (finished work); their choice, once made, wins. */
    collapsed?: boolean;
}

/** One row: a thread, a session. */
export interface StatusRow {
    id: string;
    group: string;
    title: ReactNode;
    /** The line under the title: what it waits on, where its work is. */
    subtitle?: ReactNode;
    /** At the right: small pills about its work (commits, merged). */
    badges?: ReactNode;
    /** At the far right: when it last moved. */
    meta?: ReactNode;
    /** Under the text: a control of this row (what the thread waits on, answered in place). */
    footer?: ReactNode;
    /** Under the whole row: its detail (the thread's issues and PRs). */
    trail?: ReactNode;
    selected?: boolean;
    /** Changed since the user last looked. */
    unseen?: boolean;
    /** Waits on the user (an answer, or a failed turn): shown on the group's heading while it is folded. */
    needsYou?: boolean;
    /** Overrides opening the thread: the select mode's toggle. */
    onClick?: () => void;
    /** At the right edge, before the meta: the select mode's checkbox. */
    end?: ReactNode;
}

/** A quiet group heading: the state's mark, the name, a count. The key of a
 * group is the status it lists. Given `onClick`, the heading opens what the
 * count counts (the status area's click-through: a state → its rows). */
export function StatusHeading({ label, status, count, onClick }: { label: string; status: string; count: number; onClick?: (e: { stopPropagation(): void; currentTarget: HTMLElement }) => void }) {
    return (
        <Box onClick={onClick}
            role={onClick ? 'button' : undefined} tabIndex={onClick ? 0 : undefined}
            aria-label={onClick ? `${label} ${count}` : undefined}
            onKeyDown={onClick ? (e) => { if (e.target === e.currentTarget && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); onClick(e); } } : undefined}
            sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, ...(onClick && { cursor: 'pointer', '&:hover': { bgcolor: 'action.hover' }, borderRadius: radius.row, px: 0.5 }) }}>
            <ThreadStateMark status={status} />
            <Box component="span" sx={{ fontSize: fontSizes.sm, fontWeight: 600, color: status === 'waiting' ? 'text.primary' : 'text.secondary' }}>{label}</Box>
            <Box component="span" sx={{ fontSize: fontSizes.sm, color: 'text.disabled' }}>{count}</Box>
        </Box>
    );
}

/** How the groups of a list or a board are folded: whether one is open (a folded
 * group that holds the open thread shows it), and what a click on its heading does. */
export interface GroupFold {
    isOpen: (key: string, holdsSelected: boolean) => boolean;
    onToggle: (key: string, open: boolean) => void;
}

/** A short fold, none for those who ask for less motion. */
export function useFoldTimeout() {
    return useMediaQuery('(prefers-reduced-motion: reduce)') ? 0 : 150;
}

/** The heading of a group that folds: the whole row is the toggle, with the
 * chevron at its end. While folded it still says how many threads it holds
 * and whether any of them waits on the user. */
export function GroupHeader({ label, status, count, needsYou = 0, open, onToggle, controls, variant = 'header', sx }: {
    label: string; status: string; count: number; needsYou?: number; open: boolean; onToggle: () => void; controls: string;
    variant?: 'header' | 'inline'; sx?: Parameters<typeof DisclosureRow>[0]['sx'];
}) {
    const { t } = useTranslation('projects');
    // A waiting or failed group already says so with its mark; only other groups need the extra dot.
    const attention = !open && needsYou > 0 && status !== 'waiting' && status !== 'failed';
    const needs = t('overview.needsYouIn', { count: needsYou });
    return (
        <DisclosureRow open={open} onToggle={onToggle} variant={variant} aria-controls={controls}
            aria-label={attention ? `${label} ${count} · ${needs}` : `${label} ${count}`}
            sx={[{ gap: 1, py: 0.5, '@media (pointer: coarse)': { minHeight: mobile.tapMin } }, ...(Array.isArray(sx) ? sx : [sx])]}>
            <StatusHeading label={label} status={status} count={count} />
            {attention && (
                <Box component="span" title={needs} sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5, fontSize: fontSizes.sm, color: 'text.primary' }}>
                    <ThreadStateMark status="waiting" />{needsYou}
                </Box>
            )}
        </DisclosureRow>
    );
}

/** Rows sit one step in from their group heading, so the group → row level reads at a glance. */
export const GROUP_INDENT = 2;

function Row({ row, onOpen }: { row: StatusRow; onOpen: (id: string) => void }) {
    return <ListRow title={row.title} subtitle={row.subtitle} badges={row.badges} meta={row.meta} footer={row.footer} trail={row.trail} end={row.end} selected={row.selected} unseen={row.unseen} onClick={row.onClick ?? (() => onOpen(row.id))} />;
}

/**
 * Work grouped by status, most urgent first: a project's threads, a working
 * directory's sessions. Groups with no rows are left out. Every group folds
 * from its heading; `fold` says which are open and records the user's choice.
 */
export function StatusGroups({ groups, rows, onOpen, fold, empty }: { groups: StatusGroup[]; rows: StatusRow[]; onOpen: (id: string) => void; fold: GroupFold; empty?: ReactNode }) {
    const base = useId();
    const timeout = useFoldTimeout();
    if (rows.length === 0) {
        return <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>{empty}</Typography>;
    }
    return (
        <Box sx={{ py: 1 }}>
            {groups.map((g) => {
                const list = rows.filter((r) => r.group === g.key);
                if (list.length === 0) return null;
                const open = fold.isOpen(g.key, list.some((r) => r.selected));
                const bodyId = `${base}-${g.key}`;
                return (
                    <Box key={g.key} component="section" aria-label={g.label} sx={{ mb: 1.5 }}>
                        <GroupHeader label={g.label} status={g.key} count={list.length} needsYou={list.filter((r) => r.needsYou).length}
                            open={open} onToggle={() => fold.onToggle(g.key, !open)} controls={bodyId} />
                        <Collapse in={open} timeout={timeout} id={bodyId} sx={{ pl: GROUP_INDENT }}>
                            {list.map((r) => <Row key={r.id} row={r} onOpen={onOpen} />)}
                        </Collapse>
                    </Box>
                );
            })}
        </Box>
    );
}
