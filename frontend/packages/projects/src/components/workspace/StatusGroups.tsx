import { useState, type ReactNode } from 'react';
import { Box, Collapse, Typography } from '@mui/material';
import { DisclosureRow } from '../common/Disclosure';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import { ListRow } from '../ListRow';
import { ThreadStateMark } from '../project/threadStatus';

/** One group of the list: what needs the user comes first. */
export interface StatusGroup {
    key: string;
    label: string;
    /** Closed until clicked (finished work). */
    collapsed?: boolean;
}

/** One row: a thread, a session. */
export interface StatusRow {
    id: string;
    group: string;
    title: string;
    /** The line under the title: what it waits on, where its work is. */
    subtitle?: ReactNode;
    /** At the right: small pills about its work (commits, merged). */
    badges?: ReactNode;
    /** At the far right: when it last moved. */
    meta?: ReactNode;
    selected?: boolean;
    /** Changed since the user last looked. */
    unseen?: boolean;
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

function Row({ row, onOpen }: { row: StatusRow; onOpen: (id: string) => void }) {
    return <ListRow title={row.title} subtitle={row.subtitle} badges={row.badges} meta={row.meta} selected={row.selected} unseen={row.unseen} onClick={() => onOpen(row.id)} />;
}

/**
 * Work grouped by status, most urgent first: a project's threads, a working
 * directory's sessions. Groups with no rows are left out. Given
 * `onGroupClick`, a group heading opens its group's list (the status
 * area's click-through: a state → its rows).
 */
export function StatusGroups({ groups, rows, onOpen, onGroupClick, empty }: { groups: StatusGroup[]; rows: StatusRow[]; onOpen: (id: string) => void; /** Opens a status group's list from its heading. */ onGroupClick?: (key: string, label: string, anchor: HTMLElement) => void; empty?: ReactNode }) {
    const [expanded, setExpanded] = useState<Record<string, boolean>>({});
    if (rows.length === 0) {
        return <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>{empty}</Typography>;
    }
    return (
        <Box sx={{ py: 1 }}>
            {groups.map((g) => {
                const list = rows.filter((r) => r.group === g.key);
                if (list.length === 0) return null;
                // A collapsed group opens by itself while it holds the open row.
                const open = !g.collapsed || (expanded[g.key] ?? list.some((r) => r.selected));
                return (
                    <Box key={g.key} component="section" aria-label={g.label} sx={{ mb: 1.5 }}>
                        <DisclosureRow open={open} onToggle={g.collapsed ? () => setExpanded((e) => ({ ...e, [g.key]: !open })) : undefined} sx={{ gap: 1, py: 0.5 }}>
                            {/* A collapsed group's heading is its toggle; only open groups open their list from here. */}
                            <StatusHeading label={g.label} status={g.key} count={list.length}
                                onClick={onGroupClick && !g.collapsed ? ((e) => { e.stopPropagation(); onGroupClick(g.key, g.label, e.currentTarget); }) : undefined} />
                        </DisclosureRow>
                        <Collapse in={open}>
                            {list.map((r) => <Row key={r.id} row={r} onOpen={onOpen} />)}
                        </Collapse>
                    </Box>
                );
            })}
        </Box>
    );
}
