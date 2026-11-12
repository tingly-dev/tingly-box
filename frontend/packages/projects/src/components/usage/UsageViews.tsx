// The usage card family: stat tiles, a card with the roster-table header bar,
// and the share meter — the same anatomy as the host app's user-usage cards
// (Paper on a divider border, radius 2, header bar with title + count chip,
// right-aligned tabular numbers), so a usage table reads as one family whether
// it lives in the dashboard or in a project's settings.
import Chip from '@mui/material/Chip';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import Box from '@mui/material/Box';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { alpha, useTheme } from '@mui/material/styles';
import type { ReactNode } from 'react';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import type { UsageEntry } from './usageData';

const mono = { fontFamily: fontMono, fontSize: fontSizes.sm } as const;

/** A headline number: label on top, the value under it. Values are compact
 * ("305M") and keep the font's proportional figures — tabular digits loosen a
 * number this large; the columns below use tabular instead. */
export function UsageStatTile({ label, value, title }: { label: string; value: string; title?: string }) {
    return (
        <Paper
            elevation={0}
            sx={{ p: 1.5, px: 2, border: '1px solid', borderColor: 'divider', borderRadius: 2, bgcolor: 'background.paper' }}
        >
            <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary', display: 'block' }}>{label}</Typography>
            <Typography title={title} sx={{ fontWeight: 600, fontSize: '1.25rem', lineHeight: 1.4, color: 'text.primary' }}>{value}</Typography>
        </Paper>
    );
}

/** A row of headline numbers (total, input, output, threads). */
export function UsageStatStrip({ stats }: { stats: { label: string; value: string; title?: string }[] }) {
    return (
        <Box sx={{ display: 'grid', gap: 1.5, gridTemplateColumns: { xs: 'repeat(2, 1fr)', sm: `repeat(${stats.length}, 1fr)` } }}>
            {stats.map((s) => <UsageStatTile key={s.label} {...s} />)}
        </Box>
    );
}

/** The card the tables sit in: the roster card's header bar (title, count
 * chip) over a bordered table, sticky header inside a scrollable body. */
export function UsageCard({ title, count, maxHeight, children }: { title: string; count: number; maxHeight?: number; children: ReactNode }) {
    return (
        <Paper
            elevation={0}
            sx={{ width: '100%', borderRadius: 2, border: '1px solid', borderColor: 'divider', bgcolor: 'background.paper', overflow: 'hidden' }}
        >
            <Stack direction="row" spacing={1.5} sx={{ alignItems: 'center', px: 2.5, py: 1.75, borderBottom: '1px solid', borderColor: 'divider' }}>
                <Typography sx={{ fontWeight: 600, fontSize: fontSizes.sm, color: 'text.primary' }}>{title}</Typography>
                {count > 0 && (
                    <Chip
                        size="small"
                        variant="outlined"
                        label={count}
                        sx={{ height: 20, fontSize: fontSizes.xs, color: 'text.secondary', '.MuiChip-label': { px: 0.75 } }}
                    />
                )}
            </Stack>
            <TableContainer sx={{ maxHeight }}>
                <Table size="small" stickyHeader sx={{ minWidth: 560 }}>
                    {children}
                </Table>
            </TableContainer>
        </Paper>
    );
}

/** Header cells: small secondary labels, numbers to the right. */
export function UsageHeadCell({ label, numeric = false }: { label: string; numeric?: boolean }) {
    return (
        <TableCell
            align={numeric ? 'right' : undefined}
            sx={{ fontWeight: 600, fontSize: fontSizes.sm, color: 'text.secondary', whiteSpace: 'nowrap', py: 1.25 }}
        >
            {label}
        </TableCell>
    );
}

/** A right-aligned number in the tabular style: every digit the same width,
 * so In/Out columns line up down the table. */
export function UsageNumCell({ value, strong = false, title }: { value: string; strong?: boolean; title?: string }) {
    return (
        <TableCell
            align="right"
            title={title}
            sx={{ fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap', py: 1.25, ...(strong && { fontWeight: 600 }) }}
        >
            {value}
        </TableCell>
    );
}

/** The share meter: a primary fill on a lighter step of the same hue (the
 * track stays in the ramp, so the bar reads as one shape), the percent beside
 * it in text ink — color carries only the mark, never the number. */
export function UsageShareCell({ share }: { share: number }) {
    const theme = useTheme();
    return (
        <TableCell align="right" sx={{ py: 1.25 }}>
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center', justifyContent: 'flex-end' }}>
                <Box
                    aria-hidden
                    sx={{
                        width: 64,
                        height: 6,
                        borderRadius: 3,
                        overflow: 'hidden',
                        bgcolor: alpha(theme.palette.primary.main, theme.palette.mode === 'dark' ? 0.2 : 0.12),
                    }}
                >
                    <Box sx={{ width: `${share}%`, height: '100%', bgcolor: 'primary.main' }} />
                </Box>
                <Typography component="span" sx={{ fontSize: fontSizes.sm, color: 'text.secondary', fontVariantNumeric: 'tabular-nums', width: 42, textAlign: 'right' }}>
                    {share.toFixed(1)}%
                </Typography>
            </Stack>
        </TableCell>
    );
}

/** The identity cell: a model name in mono, or a thread's title with the
 * model it ran on under it. */
export function UsageNameCell({ entry }: { entry: UsageEntry }) {
    return (
        <TableCell sx={{ py: 1.25 }}>
            {entry.secondary ? (
                <>
                    <Typography sx={{ fontWeight: 500, color: 'text.primary' }}>{entry.label}</Typography>
                    <Typography variant="caption" component="div" sx={{ ...mono, color: 'text.secondary' }}>
                        {entry.secondary}
                    </Typography>
                </>
            ) : (
                <Typography component="span" sx={{ ...mono, fontWeight: 500, color: 'text.primary' }}>{entry.label}</Typography>
            )}
        </TableCell>
    );
}

/** A full table body: identity, count, share, in, out — the column order both
 * cards use, so the eye finds each figure at the same x in either card. */
export function UsageRows({ rows, countTitle, formatTokens }: { rows: UsageEntry[]; countTitle: string; formatTokens: (n: number) => string }) {
    return (
        <TableBody>
            {rows.map((row) => (
                <TableRow key={row.key} sx={{ '&:last-child td': { borderBottom: 0 } }}>
                    <UsageNameCell entry={row} />
                    <UsageNumCell value={String(row.count)} title={countTitle} />
                    <UsageShareCell share={row.share} />
                    <UsageNumCell value={formatTokens(row.inputTokens)} title={String(row.inputTokens)} />
                    <UsageNumCell value={formatTokens(row.outputTokens)} title={String(row.outputTokens)} />
                </TableRow>
            ))}
        </TableBody>
    );
}
