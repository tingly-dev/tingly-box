// Ranked Top list for the Team usage page. Follows the active roster axis
// (accounts / models / providers); clicking an entry selects the same subject
// the roster row would — one selection state, no new modes. Numbers carry the
// information; no bar-track decoration.

import { Box, Stack, Typography, alpha, useTheme } from '@mui/material';
import { radius } from '@tingly/ui/theme/radius';

export interface ShareBarItem {
    key: string;
    name: string;
    tokens: number;
}

export interface ShareBarRow extends ShareBarItem {
    /** Percentage of all axis tokens, 0–100. */
    share: number;
    /** 1-based rank within the ranked list (others gets rows.length + 1). */
    rank: number;
}

export interface ShareResult {
    rows: ShareBarRow[];
    /** Aggregated tail beyond topN; null when every subject fits in rows. */
    others: ShareBarRow | null;
    /** Total tokens across every subject (the share denominator). */
    total: number;
}

export const OTHERS_SHARE_KEY = '__others__';

// Zero-usage subjects are dropped before ranking — a 0% entry is noise, and
// the roster table already keeps them listed (registered access).
export function computeShare(items: ShareBarItem[], topN = 6): ShareResult {
    const active = items
        .filter((item) => item.tokens > 0)
        .sort((a, b) => b.tokens - a.tokens);
    const total = active.reduce((sum, item) => sum + item.tokens, 0);
    if (total === 0) {
        return { rows: [], others: null, total: 0 };
    }
    const rows = active.slice(0, topN).map((item, index) => ({
        ...item,
        share: (item.tokens / total) * 100,
        rank: index + 1,
    }));
    const tail = active.slice(topN);
    const othersTokens = tail.reduce((sum, item) => sum + item.tokens, 0);
    const others = tail.length > 0
        ? { key: OTHERS_SHARE_KEY, name: '', tokens: othersTokens, share: (othersTokens / total) * 100, rank: rows.length + 1 }
        : null;
    return { rows, others, total };
}

interface RosterTopListProps {
    items: ShareBarItem[];
    selectedKey?: string;
    onSelect?: (key: string) => void;
    title: string;
    othersLabel: string;
    emptyLabel: string;
    emptyHint?: string;
}

export default function RosterTopList({
    items,
    selectedKey,
    onSelect,
    title,
    othersLabel,
    emptyLabel,
    emptyHint,
}: RosterTopListProps) {
    const theme = useTheme();
    const { rows, others } = computeShare(items);

    const renderRow = (row: ShareBarRow, isOthers: boolean) => {
        const selected = !isOthers && row.key === selectedKey;
        const clickable = !isOthers && !!onSelect;
        return (
            <Box
                key={row.key}
                onClick={clickable ? () => onSelect!(row.key) : undefined}
                role={clickable ? 'button' : undefined}
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 1.25,
                    py: 0.875,
                    px: 1.25,
                    borderRadius: radius.card,
                    cursor: clickable ? 'pointer' : 'default',
                    position: 'relative',
                    transition: 'background-color 0.15s ease',
                    backgroundColor: selected ? alpha(theme.palette.primary.main, 0.08) : 'transparent',
                    boxShadow: selected ? `inset 3px 0 0 ${theme.palette.primary.main}` : 'none',
                    '&:hover': clickable && !selected ? { bgcolor: 'action.hover' } : undefined,
                }}
            >
                <Typography
                    variant="body2"
                    sx={{
                        width: 18,
                        flexShrink: 0,
                        textAlign: 'right',
                        color: 'text.secondary',
                        fontVariantNumeric: 'tabular-nums',
                    }}
                >
                    {row.rank}
                </Typography>
                <Typography
                    variant="body2"
                    noWrap
                    sx={{
                        flex: 1,
                        minWidth: 0,
                        fontWeight: selected ? 650 : 500,
                        color: isOthers ? 'text.secondary' : 'text.primary',
                    }}
                >
                    {isOthers ? othersLabel : row.name}
                </Typography>
                {/* Share % only — the absolute token counts sit one glance away
                    in the adjacent table; repeating them here is noise. */}
                <Typography variant="body2" sx={{ flexShrink: 0, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>
                    {row.share.toFixed(1)}%
                </Typography>
            </Box>
        );
    };

    return (
        <Box
            sx={{
                width: '100%',
                flexGrow: 1,
                display: 'flex',
                flexDirection: 'column',
                borderRadius: radius.card,
                border: '1px solid',
                borderColor: 'divider',
                backgroundColor: 'background.paper',
                overflow: 'hidden',
            }}
        >
            {/* Header bar mirrors the table cards' header (same padding and
                minHeight, 0.875rem title) so a Top list and its sibling table
                read as one family — borders land at the same height. */}
            <Box sx={{
                minHeight: 72,
                display: 'flex',
                alignItems: 'center',
                p: 2.5,
                borderBottom: '1px solid',
                borderColor: 'divider',
            }}>
                <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '0.875rem' }}>
                    {title}
                </Typography>
            </Box>
            {rows.length === 0 ? (
                <Box sx={{ flex: 1, minHeight: 200, display: 'grid', placeItems: 'center', textAlign: 'center' }}>
                    <Box>
                        <Typography variant="body1" sx={{ color: 'text.secondary' }}>{emptyLabel}</Typography>
                        {emptyHint && (
                            <Typography variant="caption" sx={{ color: 'text.disabled', mt: 0.5, display: 'block' }}>
                                {emptyHint}
                            </Typography>
                        )}
                    </Box>
                </Box>
            ) : (
                <Stack spacing={0.25} sx={{ flex: 1, alignContent: 'center', p: 2 }}>
                    {rows.map((row) => renderRow(row, false))}
                    {others && renderRow(others, true)}
                </Stack>
            )}
        </Box>
    );
}
