import { Box, Tooltip, Typography, tooltipClasses } from '@mui/material';
import { useEffect, useMemo, useRef, useState } from 'react';
import { formatNumber } from './chartStyles';

// Green color scale for GitHub-style heatmap (like GitHub's contribution graph).
// Level 0 (no activity) is rendered with a theme-aware neutral (see emptyCellBg)
// so it reads as an empty slot instead of a near-white green — and stays visible
// in dark mode. The value here is only a fallback.
const HEATMAP_COLORS = [
    '#ebedf0',  // Level 0: No activity (neutral, GitHub-style)
    '#9be9a8',  // Level 1: Low
    '#40c463',  // Level 2: Medium
    '#30a14e',  // Level 3: High
    '#216e39',  // Level 4: Very high
];

// Theme-aware background for the "no activity" cells / legend swatch.
const emptyCellBg = (theme: { palette: { mode: string } }) =>
    theme.palette.mode === 'dark' ? 'rgba(255,255,255,0.08)' : '#ebedf0';

// Faint hairline so individual cells stay defined against the card background.
const cellBorder = (theme: { palette: { mode: string } }) =>
    `1px solid ${theme.palette.mode === 'dark' ? 'rgba(255,255,255,0.06)' : 'rgba(27,31,35,0.06)'}`;

export interface DailyUsage {
    date: string;           // YYYY-MM-DD format
    inputTokens: number;
    outputTokens: number;
    cacheTokens?: number;
    totalTokens: number;
    breakdown?: {
        name: string;
        provider: string;
        tokens: number;
    }[];
}

interface TokenHeatmapProps {
    data: DailyUsage[];
}

const formatTokenTotal = formatNumber;

// Fixed gap between cells; cell size itself is responsive (see below).
const CELL_GAP = 3;
const MIN_CELL = 10;
const MAX_CELL = 16;
// Approximate width reserved for the Mon/Sun labels column.
const DAY_LABEL_WIDTH = 34;

// Calculate streaks
const computeStreaks = (allDays: string[], valueByDate: Map<string, number>) => {
    let longestStreak = 0;
    let running = 0;

    for (const day of allDays) {
        const active = (valueByDate.get(day) ?? 0) > 0;
        if (active) {
            running += 1;
            if (running > longestStreak) {
                longestStreak = running;
            }
        } else {
            running = 0;
        }
    }

    let currentStreak = 0;
    for (let i = allDays.length - 1; i >= 0; i -= 1) {
        const day = allDays[i];
        const active = (valueByDate.get(day) ?? 0) > 0;
        if (!active) break;
        currentStreak += 1;
    }

    return { longestStreak, currentStreak };
};

// Get Monday-based weekday (0 = Monday, 6 = Sunday).
// Parse as local time (T00:00:00) — a bare YYYY-MM-DD is parsed as UTC
// midnight, which shifts the weekday by one in timezones behind UTC.
const getMondayBasedWeekday = (dateStr: string): number => {
    const date = new Date(`${dateStr}T00:00:00`);
    const sundayBased = date.getDay();
    return (sundayBased + 6) % 7;
};

// Pad days to align with Monday
const padToWeekStart = (days: string[]): (string | null)[] => {
    const firstDay = getMondayBasedWeekday(days[0]);
    const padding = new Array(firstDay).fill(null);
    return [...padding, ...days];
};

// Chunk into weeks
const chunkByWeek = (days: (string | null)[]): (string | null)[][] => {
    const weeks: (string | null)[][] = [];
    for (let i = 0; i < days.length; i += 7) {
        weeks.push(days.slice(i, i + 7));
    }
    return weeks;
};

// Get month label for a week
const getMonthLabel = (week: (string | null)[]): string | null => {
    const lastDay = [...week].reverse().find(Boolean);
    if (!lastDay) return null;
    return new Date(`${lastDay}T00:00:00`).toLocaleString('en-US', { month: 'short' });
};

// Build month labels (show only when month changes)
const buildMonthLabels = (weeks: (string | null)[][]): (string | null)[] => {
    return weeks.map((week, i) => {
        const label = getMonthLabel(week);
        const previous = i > 0 ? getMonthLabel(weeks[i - 1]) : null;
        return label !== previous ? label : null;
    });
};

const DAYS_OF_WEEK = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

// One inline stat in the footer strip: bold value followed by a muted label.
const StatInline = ({ value, label }: { value: string; label: string }) => (
    <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 0.5, whiteSpace: 'nowrap' }}>
        <Typography sx={{ fontSize: '0.78rem', fontWeight: 700 }}>{value}</Typography>
        <Typography sx={{ fontSize: '0.71rem', color: 'text.secondary' }}>{label}</Typography>
    </Box>
);

export const TokenHeatmap = ({ data }: TokenHeatmapProps) => {
    // Build lookup maps (data is already filled by parent)
    const {
        dayMap,
        thresholds,
        totalTokens,
        activeDays,
        longestStreak,
        maxValue,
    } = useMemo(() => {
        const map = new Map<string, DailyUsage>();
        const values = new Map<string, number>();
        let max = 0;
        let total = 0;
        let active = 0;

        for (const item of data) {
            map.set(item.date, item);
            values.set(item.date, item.totalTokens);
            if (item.totalTokens > max) max = item.totalTokens;
            total += item.totalTokens;
            if (item.totalTokens > 0) active += 1;
        }

        // Quantile thresholds (p25 / p50 / p75 of active days) so the four
        // green levels are evenly distributed and the grid shows texture even
        // when every day has activity — a value/max linear scale collapses
        // into a single shade in that case.
        const nonZero = data.map((d) => d.totalTokens).filter((v) => v > 0).sort((a, b) => a - b);
        const quantile = (p: number) =>
            nonZero.length ? nonZero[Math.min(nonZero.length - 1, Math.floor(p * nonZero.length))] : 0;
        const qs: [number, number, number] = [quantile(0.25), quantile(0.5), quantile(0.75)];

        // Calculate streaks
        const allDays = data.map((d) => d.date);
        const streaks = computeStreaks(allDays, values);

        return {
            dayMap: map,
            thresholds: qs,
            totalTokens: total,
            activeDays: active,
            longestStreak: streaks.longestStreak,
            maxValue: max,
        };
    }, [data]);

    const levelFor = (value: number): number => {
        if (value <= 0) return 0;
        return 1 + thresholds.filter((t) => value > t).length;
    };

    // Build grid data
    const { weeks, monthLabels, allDays } = useMemo(() => {
        const days = data.map((d) => d.date);
        const padded = padToWeekStart(days);
        const weekChunks = chunkByWeek(padded);
        const labels = buildMonthLabels(weekChunks);

        return {
            weeks: weekChunks,
            monthLabels: labels,
            allDays: days,
        };
    }, [data]);

    // Responsive cell size: fill the available width with the fixed number of
    // week columns, clamped to a sane range. Falls back to horizontal scroll
    // below the minimum.
    const containerRef = useRef<HTMLDivElement>(null);
    const scrollRef = useRef<HTMLDivElement>(null);
    const [cellSize, setCellSize] = useState(12);
    const weekCount = weeks.length;
    useEffect(() => {
        const el = containerRef.current;
        if (!el || weekCount === 0) return;
        const update = () => {
            const pitch = Math.floor((el.clientWidth - DAY_LABEL_WIDTH) / weekCount);
            setCellSize(Math.max(MIN_CELL, Math.min(MAX_CELL, pitch - CELL_GAP)));
        };
        update();
        const observer = new ResizeObserver(update);
        observer.observe(el);
        return () => observer.disconnect();
    }, [weekCount]);

    // When the grid is wider than the pane (horizontal scroll active), start
    // scrolled to the right so the most recent weeks are visible — matching
    // where the user's attention is (GitHub does the same).
    useEffect(() => {
        const el = scrollRef.current;
        if (!el) return;
        if (el.scrollWidth > el.clientWidth) {
            el.scrollLeft = el.scrollWidth - el.clientWidth;
        }
    }, [weekCount, cellSize]);

    return (
        // Outer box measures the available width and centers the group.
        <Box ref={containerRef} sx={{ width: '100%', display: 'flex', justifyContent: 'center' }}>
            {/* Grid + footer strip form one self-contained group of the same
                width, so the composition holds together instead of scattering
                across the pane. */}
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1, maxWidth: '100%', minWidth: 0 }}>
                <Box ref={scrollRef} sx={{ overflowX: 'auto', overflowY: 'hidden', pb: 0.5 }}>
                    {/* The label column is the same fixed width the cell-size
                        calculation assumes, so the grid fits the measured pane
                        exactly — a max-content column can differ by a few px
                        and leave a phantom scrollbar that flickers on resize. */}
                    <Box
                        sx={{
                            display: 'grid',
                            gap: `${CELL_GAP}px`,
                            gridTemplateColumns: `${DAY_LABEL_WIDTH - CELL_GAP}px repeat(${weeks.length}, ${cellSize}px)`,
                            gridTemplateRows: `repeat(8, ${cellSize}px)`,
                        }}
                    >
                        {/* Day of week labels */}
                        {DAYS_OF_WEEK.map((day, dayIndex) => {
                            const showLabel = dayIndex === 0 || dayIndex === 6;
                            return (
                                <Typography
                                    key={day}
                                    sx={{
                                        fontSize: '10px',
                                        color: 'text.secondary',
                                        pr: 1,
                                        gridColumn: 1,
                                        gridRow: dayIndex + 2,
                                        display: 'flex',
                                        alignItems: 'center',
                                        justifyContent: 'flex-end',
                                    }}
                                >
                                    {showLabel ? day : ''}
                                </Typography>
                            );
                        })}

                        {/* Month labels */}
                        {weeks.map((_, weekIndex) => {
                            const label = monthLabels[weekIndex];
                            return (
                                <Typography
                                    key={`month-${weekIndex}`}
                                    sx={{
                                        fontSize: '10px',
                                        color: 'text.secondary',
                                        gridColumn: weekIndex + 2,
                                        gridRow: 1,
                                        display: 'flex',
                                        alignItems: 'center',
                                    }}
                                >
                                    {label}
                                </Typography>
                            );
                        })}

                        {/* Heatmap cells */}
                        {weeks.map((week, weekIndex) =>
                            week.map((day, dayIndex) => {
                                if (!day) {
                                    return (
                                        <Box
                                            key={`empty-${weekIndex}-${dayIndex}`}
                                            sx={{
                                                gridColumn: weekIndex + 2,
                                                gridRow: dayIndex + 2,
                                            }}
                                        />
                                    );
                                }

                                const dayData = dayMap.get(day);
                                const value = dayData?.totalTokens || 0;
                                const colorIndex = levelFor(value);
                                const fill = HEATMAP_COLORS[colorIndex];

                                return (
                                    <Tooltip
                                        key={day}
                                        title={
                                            <Box
                                                sx={{
                                                    px: 2,
                                                    py: 1.5,
                                                    bgcolor: 'grey.900',
                                                    borderRadius: 1.5,
                                                    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.3)',
                                                    border: '1px solid',
                                                    borderColor: 'grey.700',
                                                    minWidth: 200,
                                                }}
                                            >
                                                <Typography
                                                    sx={{
                                                        fontSize: '13px',
                                                        fontWeight: 600,
                                                        mb: 1,
                                                        color: '#ffffff',
                                                    }}
                                                >
                                                    {new Date(`${day}T00:00:00`).toLocaleDateString('en-US', {
                                                        weekday: 'short',
                                                        month: 'short',
                                                        day: 'numeric',
                                                        year: 'numeric',
                                                    })}
                                                </Typography>
                                                <Typography
                                                    sx={{
                                                        fontSize: '13px',
                                                        fontWeight: 600,
                                                        color: '#86efac',
                                                        mb: 0.5,
                                                    }}
                                                >
                                                    {formatTokenTotal(value)} total tokens
                                                </Typography>
                                                {dayData && (dayData.inputTokens > 0 || dayData.outputTokens > 0 || (dayData.cacheTokens ?? 0) > 0) && (
                                                    <Typography
                                                        sx={{
                                                            fontSize: '12px',
                                                            color: 'rgba(255, 255, 255, 0.85)',
                                                            mt: 0.5,
                                                        }}
                                                    >
                                                        Input: {formatTokenTotal(dayData.inputTokens)} | Cache:{' '}
                                                        {formatTokenTotal(dayData.cacheTokens ?? 0)} | Output:{' '}
                                                        {formatTokenTotal(dayData.outputTokens)}
                                                    </Typography>
                                                )}
                                                {dayData?.breakdown && dayData.breakdown.length > 0 && (
                                                    <Box sx={{ mt: 1, pt: 1, borderTop: '1px solid', borderColor: 'rgba(255,255,255,0.1)' }}>
                                                        {dayData.breakdown.slice(0, 3).map((model) => (
                                                            <Typography
                                                                key={`${day}-${model.name}`}
                                                                sx={{
                                                                    fontSize: '12px',
                                                                    color: 'rgba(255, 255, 255, 0.75)',
                                                                }}
                                                            >
                                                                {model.name}: {formatTokenTotal(model.tokens)}
                                                            </Typography>
                                                        ))}
                                                    </Box>
                                                )}
                                            </Box>
                                        }
                                        arrow
                                        slotProps={{
                                            popper: {
                                                sx: {
                                                    [`& .${tooltipClasses.tooltip}`]: {
                                                        bgcolor: 'transparent',
                                                        boxShadow: 'none',
                                                        padding: 0,
                                                    },
                                                },
                                            },
                                        }}
                                    >
                                        <Box
                                            component="button"
                                            type="button"
                                            sx={{
                                                gridColumn: weekIndex + 2,
                                                gridRow: dayIndex + 2,
                                                width: cellSize,
                                                height: cellSize,
                                                backgroundColor: colorIndex === 0 ? emptyCellBg : fill,
                                                border: cellBorder,
                                                borderRadius: '3px',
                                                cursor: 'default',
                                                transition: 'opacity 0.1s',
                                                // Highlight with outline/opacity only. A hover
                                                // transform: scale() extends the scrollable
                                                // overflow of the overflowX container, so edge
                                                // cells made the scrollbar flicker in and out
                                                // and the grid jump under the cursor.
                                                '&:hover': {
                                                    opacity: 0.85,
                                                    outline: '1.5px solid',
                                                    outlineColor: 'text.primary',
                                                    outlineOffset: '1px',
                                                    zIndex: 1,
                                                },
                                                p: 0,
                                            }}
                                            aria-label={`${day}: ${value} total tokens`}
                                        />
                                    </Tooltip>
                                );
                            })
                        )}
                    </Box>
                </Box>

                {/* Footer strip: stats on the left, legend on the right — the
                    same width as the grid, GitHub-style. */}
                <Box
                    sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        flexWrap: 'wrap',
                        gap: 1,
                        pl: `${DAY_LABEL_WIDTH}px`,
                    }}
                >
                    <Box sx={{ display: 'flex', alignItems: 'baseline', columnGap: 2, rowGap: 0.5, flexWrap: 'wrap' }}>
                        <StatInline value={formatTokenTotal(totalTokens)} label="tokens" />
                        <StatInline value={`${activeDays}/${allDays.length}`} label="active days" />
                        <StatInline value={`${longestStreak}d`} label="longest streak" />
                        <StatInline value={formatTokenTotal(maxValue)} label="max/day" />
                    </Box>

                    {/* Legend */}
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                        <Typography sx={{ fontSize: '0.625rem', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.4px', color: 'text.secondary' }}>
                            Less
                        </Typography>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.375 }}>
                            {HEATMAP_COLORS.map((color, index) => (
                                <Box
                                    key={index}
                                    sx={{
                                        width: 10,
                                        height: 10,
                                        backgroundColor: index === 0 ? emptyCellBg : color,
                                        border: cellBorder,
                                        borderRadius: '2px',
                                    }}
                                />
                            ))}
                        </Box>
                        <Typography sx={{ fontSize: '0.625rem', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.4px', color: 'text.secondary' }}>
                            More
                        </Typography>
                    </Box>
                </Box>
            </Box>
        </Box>
    );
};

export default TokenHeatmap;
