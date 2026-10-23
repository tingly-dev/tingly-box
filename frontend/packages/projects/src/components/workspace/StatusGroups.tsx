import { useState, type ReactNode } from 'react';
import { Box, Collapse, Typography } from '@mui/material';
import { ExpandMore } from '../icons';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { stateColor, ThreadStateMark } from '../project/threadStatus';
import { radius } from '@tingly/ui/theme/radius';

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
 * group is the status it lists. */
export function StatusHeading({ label, status, count }: { label: string; status: string; count: number }) {
    return (
        <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75 }}>
            <ThreadStateMark status={status} />
            <Box component="span" sx={{ fontSize: fontSizes.sm, fontWeight: 600, color: stateColor(status) }}>{label}</Box>
            <Box component="span" sx={{ fontSize: fontSizes.sm, color: 'text.disabled' }}>{count}</Box>
        </Box>
    );
}

function Row({ row, onOpen }: { row: StatusRow; onOpen: (id: string) => void }) {
    return (
        <Box
            role="button"
            tabIndex={0}
            aria-current={row.selected ? 'page' : undefined}
            onClick={() => onOpen(row.id)}
            onKeyDown={(e) => e.key === 'Enter' && onOpen(row.id)}
            sx={{
                px: 1.5, py: 1, borderRadius: radius.row, outline: 'none', cursor: 'pointer',
                bgcolor: row.selected ? 'action.selected' : undefined,
                '&:hover': { bgcolor: row.selected ? 'action.selected' : 'action.hover' },
                '&:focus-visible': { boxShadow: (t) => `inset 0 0 0 2px ${t.palette.primary.main}` },
            }}
        >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography noWrap sx={{ fontSize: fontSizes.base, fontWeight: row.unseen ? 650 : 500, color: 'text.primary' }}>{row.title}</Typography>
                    {row.subtitle != null && <Typography noWrap component="div" sx={{ fontSize: fontSizes.sm, color: 'text.secondary' }}>{row.subtitle}</Typography>}
                </Box>
                {row.badges}
                {row.meta != null && <Typography component="span" sx={{ flexShrink: 0, fontSize: fontSizes.sm, color: 'text.disabled', minWidth: 28, textAlign: 'right' }}>{row.meta}</Typography>}
            </Box>
        </Box>
    );
}

/**
 * Work grouped by status, most urgent first: a project's threads, a working
 * directory's sessions. Groups with no rows are left out.
 */
export function StatusGroups({ groups, rows, onOpen, empty }: { groups: StatusGroup[]; rows: StatusRow[]; onOpen: (id: string) => void; empty?: ReactNode }) {
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
                        <Box
                            role={g.collapsed ? 'button' : undefined}
                            aria-expanded={g.collapsed ? open : undefined}
                            onClick={g.collapsed ? () => setExpanded((e) => ({ ...e, [g.key]: !open })) : undefined}
                            sx={{ display: 'flex', alignItems: 'center', gap: 1, px: 1.5, py: 0.5, cursor: g.collapsed ? 'pointer' : undefined }}
                        >
                            <StatusHeading label={g.label} status={g.key} count={list.length} />
                            {g.collapsed && <ExpandMore sx={{ fontSize: 16, color: 'text.secondary', transform: open ? 'rotate(180deg)' : 'none' }} />}
                        </Box>
                        <Collapse in={open}>
                            {list.map((r) => <Row key={r.id} row={r} onOpen={onOpen} />)}
                        </Collapse>
                    </Box>
                );
            })}
        </Box>
    );
}
