import { Box, Tooltip, Typography, tooltipClasses } from '@mui/material';
import { useMemo } from 'react';
import { format } from 'date-fns';

// Orange color scale for GitHub-style heatmap (like GitHub's contribution graph)
const HEATMAP_COLORS = [
    '#fff7ed',  // Level 0: No activity (lightest cream)
    '#fed7aa',  // Level 1: Low (light orange)
    '#fdba74',  // Level 2: Medium (orange)
    '#f97316',  // Level 3: High (dark orange)
    '#c2410c',  // Level 4: Very high (darkest orange)
];

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

// Format large numbers (T, B, M, K)
const formatTokenTotal = (value: number): string => {
    const units = [
        { size: 1_000_000_000_000, suffix: 'T' },
        { size: 1_000_000_000, suffix: 'B' },
        { size: 1_000_000, suffix: 'M' },
        { size: 1_000, suffix: 'K' },
    ];

    for (const unit of units) {
        if (value >= unit.size) {
            const scaled = value / unit.size;
            const precision = scaled >= 100 ? 0 : scaled >= 10 ? 1 : 2;
            const compact = scaled
                .toFixed(precision)
                .replace(/\.0+$/, '')
                .replace(/(\.\d*[1-9])0+$/, '$1');
            return `${compact}${unit.suffix}`;
        }
    }

    return new Intl.NumberFormat('en-US').format(value);
};

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

// Metric component (center-aligned)
const Metric = ({
    caption,
    value,
}: {
    caption: string;
    value: string;
}) => (
    <Box
        sx={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            minWidth: 120,
        }}
    >
        <Typography
            sx={{
                fontSize: '10px',
                fontWeight: 600,
                textTransform: 'uppercase',
                letterSpacing: '0.5px',
                color: 'text.secondary',
            }}
        >
            {caption}
        </Typography>
        <Typography sx={{ fontSize: '14px', fontWeight: 600 }}>{value}</Typography>
    </Box>
);

export const TokenHeatmap = ({
    data,
    cellSize = 9,
    gap = 2,
    startDate,
    endDate,
    title = '',
}: TokenHeatmapProps) => {
    // Build lookup maps (data is already filled by parent)
    const {
        dayMap,
        valueByDate,
        maxValue,
        totalTokens,
        totalInput,
        totalOutput,
        activeDays,
        longestStreak,
        currentStreak,
    } = useMemo(() => {
        const map = new Map<string, DailyUsage>();
        const values = new Map<string, number>();
        let max = 0;
        let total = 0;
        let input = 0;
        let output = 0;
        let active = 0;

        for (const item of data) {
            map.set(item.date, item);
            values.set(item.date, item.totalTokens);
            if (item.totalTokens > max) max = item.totalTokens;
            total += item.totalTokens;
            input += item.inputTokens;
            output += item.outputTokens;
            if (item.totalTokens > 0) active += 1;
        }

        // Calculate streaks
        const allDays = data.map((d) => d.date);
        const streaks = computeStreaks(allDays, values);

        return {
            dayMap: map,
            valueByDate: values,
            maxValue: max,
            totalTokens: total,
            totalInput: input,
            totalOutput: output,
            activeDays: active,
            longestStreak: streaks.longestStreak,
            currentStreak: streaks.currentStreak,
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
                borderRadius: '12px',
                border: '1px solid',
                borderColor: 'divider',
                bgcolor: 'background.paper',
                p: { xs: 2, md: 3 },
                display: 'flex',
                flexDirection: 'column',
                gap: 2,
            }}
        >
            {/* Header with title and top metrics */}
            <Box
                sx={{
                    display: 'flex',
                    flexWrap: 'wrap',
                    alignItems: 'center',
                    justifyContent: 'center',
                    gap: 2,
                }}
            >
                <Typography sx={{ fontSize: '14px', fontWeight: 600 }}>{title}</Typography>
                <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 2, justifyContent: 'center' }}>
                    <Metric caption="Input tokens" value={formatTokenTotal(totalInput)} />
                    <Metric caption="Cache tokens" value={formatTokenTotal(data.reduce((sum, d) => sum + (d.cacheTokens || 0), 0))} />
                    <Metric caption="Output tokens" value={formatTokenTotal(totalOutput)} />
                    <Metric caption="Total tokens" value={formatTokenTotal(totalTokens)} />
                </Box>
            </Box>

            {/* Heatmap Grid */}
            <Box
                sx={{
                    overflowX: 'hidden',
                    overflowY: 'hidden',
                }}
            >
                <Box
                    sx={{
                        display: 'grid',
                        gap,
                        gridTemplateColumns: `max-content repeat(${weeks.length}, ${cellSize}px)`,
                        gridTemplateRows: `repeat(8, ${cellSize}px)`,
                        margin: '0 auto',
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
                                                    color: '#fed7aa',
                                                    mb: 0.5,
                                                }}
                                            >
                                                {formatTokenTotal(value)} total tokens
                                            </Typography>
                                            {dayData && (dayData.inputTokens > 0 || dayData.outputTokens > 0 || dayData.cacheTokens > 0) && (
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
                                            backgroundColor: fill,
                                            borderRadius: '4px',
                                            border: 'none',
                                            cursor: 'default',
                                            transition: 'transform 0.1s, opacity 0.1s',
                                            '&:hover': {
                                                transform: 'scale(1.15)',
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

            {/* Legend */}
            <Box
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 1,
                    justifyContent: 'center',
                }}
            >
                <Typography
                    sx={{
                        fontSize: '10px',
                        fontWeight: 600,
                        textTransform: 'uppercase',
                        letterSpacing: '0.5px',
                        color: 'text.secondary',
                    }}
                >
                    Less
                </Typography>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                    {HEATMAP_COLORS.map((color, index) => (
                        <Box
                            key={index}
                            sx={{
                                width: cellSize,
                                height: cellSize,
                                backgroundColor: color,
                                borderRadius: '4px',
                            }}
                        />
                    ))}
                </Box>
                <Typography
                    sx={{
                        fontSize: '10px',
                        fontWeight: 600,
                        textTransform: 'uppercase',
                        letterSpacing: '0.5px',
                        color: 'text.secondary',
                    }}
                >
                    More
                </Typography>
            </Box>

            {/* Bottom metrics */}
            <Box
                sx={{
                    display: 'grid',
                    gridTemplateColumns: { xs: '1fr 1fr', md: 'repeat(4, 1fr)' },
                    gap: 2,
                    justifyContent: 'center',
                }}
            >
                <Metric caption="Longest streak" value={`${longestStreak} days`} />
                <Metric caption="Current streak" value={`${currentStreak} days`} />
                <Metric caption="Active days" value={`${activeDays} / ${allDays.length}`} />
                <Metric caption="Max daily" value={formatTokenTotal(maxValue)} />
            </Box>
        </Box>
    );
};

export default TokenHeatmap;
