import { Box, Tooltip, Typography, tooltipClasses } from '@mui/material';
import { useMemo } from 'react';
import { format } from 'date-fns';
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

export interface HeatmapMetrics {
    totalTokens: number;
    totalInput: number;
    totalOutput: number;
    longestStreak: number;
    currentStreak: number;
    activeDays: number;
    maxValue: number;
}

interface TokenHeatmapProps {
    data: DailyUsage[];
    cellSize?: number;
    gap?: number;
    startDate?: string;
    endDate?: string;
    title?: string;
}

const formatTokenTotal = formatNumber;

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

// Get Monday-based weekday (0 = Monday, 6 = Sunday)
const getMondayBasedWeekday = (dateStr: string): number => {
    const date = new Date(dateStr);
    const sundayBased = date.getDay();
    return (sundayBased + 6) % 7;
};

// Format local date
const formatLocalDate = (date: Date): string => {
    const y = date.getFullYear();
    const m = String(date.getMonth() + 1).padStart(2, '0');
    const d = String(date.getDate()).padStart(2, '0');
    return `${y}-${m}-${d}`;
};

// Get all days in range
const getAllDays = (data: DailyUsage[], startDate?: string, endDate?: string): string[] => {
    if (data.length === 0) return [];

    const dates = data.map((d) => d.date);
    const minDate = startDate || dates.reduce((a, b) => (a < b ? a : b));
    const maxDate = endDate || dates.reduce((a, b) => (a > b ? a : b));

    const days: string[] = [];
    const current = new Date(`${minDate}T00:00:00`);
    const end = new Date(`${maxDate}T00:00:00`);

    while (current <= end) {
        days.push(formatLocalDate(current));
        current.setDate(current.getDate() + 1);
    }

    return days;
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

// Calculate color level (0-4) based on value and max
const defaultColourMap = (value: number, max: number, colorCount: number): number => {
    if (max <= 0 || value <= 0) return 0;
    const index = Math.ceil((value / max) * (colorCount - 1));
    return Math.min(Math.max(index, 0), colorCount - 1);
};

const DAYS_OF_WEEK = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

// Stat row for the right-hand rail: label on the left, value on the right,
// so the summary reads as a tidy vertical list beside the grid.
const StatRow = ({ label, value }: { label: string; value: string }) => (
    <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 1 }}>
        <Typography
            sx={{
                fontSize: '0.6875rem',
                fontWeight: 600,
                textTransform: 'uppercase',
                letterSpacing: '0.4px',
                color: 'text.secondary',
                whiteSpace: 'nowrap',
            }}
        >
            {label}
        </Typography>
        <Typography sx={{ fontSize: '0.8125rem', fontWeight: 700, whiteSpace: 'nowrap' }}>{value}</Typography>
    </Box>
);

export const TokenHeatmap = ({
    data,
    cellSize = 9,
    gap = 2,
}: TokenHeatmapProps) => {
    // Build lookup maps (data is already filled by parent)
    const {
        dayMap,
        maxValue,
        totalTokens,
        activeDays,
        longestStreak,
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

        // Calculate streaks
        const allDays = data.map((d) => d.date);
        const streaks = computeStreaks(allDays, values);

        return {
            dayMap: map,
            maxValue: max,
            totalTokens: total,
            activeDays: active,
            longestStreak: streaks.longestStreak,
        };
    }, [data]);

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

    return (
        <Box
            sx={{
                width: '100%',
                display: 'flex',
                flexDirection: { xs: 'column', md: 'row' },
                alignItems: { xs: 'stretch', md: 'center' },
                gap: { xs: 2, md: 4 },
            }}
        >
            {/* Heatmap Grid (left) */}
            <Box
                sx={{
                    flex: 1,
                    minWidth: 0,
                    overflowX: 'auto',
                    overflowY: 'hidden',
                    display: 'flex',
                    justifyContent: 'center',
                    pb: 0.5,
                }}
            >
                <Box
                    sx={{
                        display: 'grid',
                        gap,
                        gridTemplateColumns: `max-content repeat(${weeks.length}, ${cellSize}px)`,
                        gridTemplateRows: `repeat(8, ${cellSize}px)`,
                        margin: { xs: 0, md: '0 auto' },
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
                            const colorIndex = defaultColourMap(value, maxValue, HEATMAP_COLORS.length);
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
                                            transition: 'transform 0.1s, opacity 0.1s',
                                            '&:hover': {
                                                transform: 'scale(1.25)',
                                                opacity: 0.9,
                                                outline: '1px solid',
                                                outlineColor: 'text.primary',
                                                outlineOffset: '1px',
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

            {/* Stats rail (right) */}
            <Box
                sx={{
                    flexShrink: 0,
                    width: { xs: '100%', md: 200 },
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 1,
                }}
            >
                <StatRow label="Total" value={formatTokenTotal(totalTokens)} />
                <StatRow label="Active" value={`${activeDays}/${allDays.length}`} />
                <StatRow label="Longest streak" value={`${longestStreak}d`} />
                <StatRow label="Max / day" value={formatTokenTotal(maxValue)} />

                {/* Legend */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, mt: 0.5 }}>
                    <Typography sx={{ fontSize: '0.625rem', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.4px', color: 'text.secondary' }}>
                        Less
                    </Typography>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.375 }}>
                        {HEATMAP_COLORS.map((color, index) => (
                            <Box
                                key={index}
                                sx={{
                                    width: 11,
                                    height: 11,
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
    );
};

export default TokenHeatmap;
