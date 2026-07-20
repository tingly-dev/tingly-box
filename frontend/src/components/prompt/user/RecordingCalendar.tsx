import { ArrowBackIosNew, ArrowForwardIos } from '@/components/icons';
import { Box, Button, ButtonGroup, IconButton, Tooltip, Typography } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import { DateCalendar, LocalizationProvider, PickerDay } from '@mui/x-date-pickers';
import type { PickerDayProps } from '@mui/x-date-pickers';
import { AdapterDateFns } from '@mui/x-date-pickers/AdapterDateFns';
import { useState } from 'react';
import { createContext, useContext } from 'react';
import { EMPTY_SX } from '@/constants/defaults';

interface RecordingCalendarProps {
    currentDate: Date;
    selectedDate: Date;
    recordingCounts: Map<string, number>; // date string -> count
    onDateSelect: (date: Date) => void;
    onMonthChange: (date: Date) => void;
    onRangeChange?: (days: number | null) => void; // null = single date mode, number = range mode
    sx?: SxProps<Theme>;
}

// Activity levels for color intensity
const getActivityLevel = (count: number): number => {
    if (count === 0) return 0;
    if (count <= 2) return 1;
    if (count <= 4) return 2;
    if (count <= 6) return 3;
    return 4;
};

interface CustomDayContextValue {
    recordingCounts: Map<string, number>;
    selectedDate: Date;
    formatDateKey: (date: Date) => string;
    isSameDay: (date1: Date, date2: Date) => boolean;
    isInRange: (date: Date) => boolean;
}

const CustomDayContext = createContext<CustomDayContextValue | null>(null);

// Custom PickersDay with activity color — kept at module scope so it has a
// stable identity across renders (defining it inside the parent re-mounts it
// every render and breaks referential equality).
const CustomDay = (props: PickerDayProps) => {
    const { day, outsideCurrentMonth, ...other } = props;
    const ctx = useContext(CustomDayContext);
    const dateKey = ctx ? ctx.formatDateKey(day as Date) : '';
    const count = ctx ? ctx.recordingCounts.get(dateKey) || 0 : 0;
    const level = getActivityLevel(count);
    const isSelected = ctx ? ctx.isSameDay(day as Date, ctx.selectedDate) : false;
    const inRange = ctx ? ctx.isInRange(day as Date) : false;

    // Color mapping
    const getBgColor = () => {
        if (isSelected) return '#3b82f6';
        if (outsideCurrentMonth) return 'transparent';
        if (inRange) return '#e0f2fe'; // Light blue for range
        switch (level) {
            case 0: return 'transparent';
            case 1: return '#dcfce7';
            case 2: return '#86efac';
            case 3: return '#22c55e';
            case 4: return '#15803d';
            default: return 'transparent';
        }
    };

    const getTextColor = () => {
        if (isSelected) return '#ffffff';
        if (inRange) return '#0369a1';
        if (level >= 3) return '#ffffff';
        if (level >= 1) return '#166534';
        return 'inherit';
    };

    return (
        <Tooltip
            title={
                count > 0
                    ? `${count} recording${count > 1 ? 's' : ''} on ${day.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })}`
                    : day.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
            }
            arrow
        >
            <PickerDay
                {...other}
                day={day}
                outsideCurrentMonth={outsideCurrentMonth}
                sx={{
                    backgroundColor: getBgColor(),
                    color: getTextColor(),
                    fontWeight: count > 0 || inRange ? 600 : 400,
                    '&:hover': {
                        backgroundColor: isSelected ? '#2563eb' : (count > 0 ? '#bbf7d0' : inRange ? '#bae6fd' : undefined),
                    },
                }}
            >
                {(day as Date).getDate()}
            </PickerDay>
        </Tooltip>
    );
};

const RecordingCalendar: React.FC<RecordingCalendarProps> = ({
    currentDate,
    selectedDate,
    recordingCounts,
    onDateSelect,
    onMonthChange,
    onRangeChange,
    sx = EMPTY_SX,
}) => {
    const [viewDate, setViewDate] = useState(currentDate);
    const [rangeMode, setRangeMode] = useState<number | null>(null);

    const formatDateKey = (date: Date): string => {
        return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
    };

    const isSameDay = (date1: Date, date2: Date): boolean => {
        return formatDateKey(date1) === formatDateKey(date2);
    };

    const isToday = (date: Date): boolean => {
        const today = new Date();
        return isSameDay(date, today);
    };

    const isInRange = (date: Date): boolean => {
        if (!rangeMode) return false;
        const today = new Date();
        today.setHours(23, 59, 59, 999);
        const startDate = new Date(today);
        startDate.setDate(startDate.getDate() - rangeMode);
        startDate.setHours(0, 0, 0, 0);
        return date >= startDate && date <= today;
    };

    const handlePrevMonth = () => {
        const newDate = new Date(viewDate);
        newDate.setMonth(newDate.getMonth() - 1);
        setViewDate(newDate);
        onMonthChange(newDate);
    };

    const handleNextMonth = () => {
        const newDate = new Date(viewDate);
        newDate.setMonth(newDate.getMonth() + 1);
        setViewDate(newDate);
        onMonthChange(newDate);
    };

    const handleRangeClick = (days: number) => {
        if (rangeMode === days) {
            // Toggle off
            setRangeMode(null);
            onRangeChange?.(null);
        } else {
            setRangeMode(days);
            onRangeChange?.(days);
        }
    };

    const monthNames = ['January', 'February', 'March', 'April', 'May', 'June',
        'July', 'August', 'September', 'October', 'November', 'December'];

    return (
        <LocalizationProvider dateAdapter={AdapterDateFns}>
            <Box sx={{ width: '100%', ...sx }}>
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                    {/* Month header with navigation */}
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <IconButton onClick={handlePrevMonth} size="small">
                                <ArrowBackIosNew fontSize="small" />
                            </IconButton>
                            <Typography sx={{ fontSize: '1rem', fontWeight: 600, minWidth: 140 }}>
                                {monthNames[viewDate.getMonth()]} {viewDate.getFullYear()}
                            </Typography>
                            <IconButton onClick={handleNextMonth} size="small">
                                <ArrowForwardIos fontSize="small" />
                            </IconButton>
                        </Box>
                    </Box>

                    {/* Range Buttons */}
                    <ButtonGroup size="small" variant="outlined" sx={{ justifyContent: 'center' }}>
                        <Button
                            onClick={() => handleRangeClick(0)}
                            variant={rangeMode === 0 ? 'contained' : 'outlined'}
                        >
                            Today
                        </Button>
                        <Button
                            onClick={() => handleRangeClick(7)}
                            variant={rangeMode === 7 ? 'contained' : 'outlined'}
                        >
                            7D
                        </Button>
                        <Button
                            onClick={() => handleRangeClick(30)}
                            variant={rangeMode === 30 ? 'contained' : 'outlined'}
                        >
                            30D
                        </Button>
                        <Button
                            onClick={() => handleRangeClick(90)}
                            variant={rangeMode === 90 ? 'contained' : 'outlined'}
                        >
                            90D
                        </Button>
                    </ButtonGroup>

                    {/* MUI DateCalendar */}
                    <CustomDayContext.Provider value={{ recordingCounts, selectedDate, formatDateKey, isSameDay, isInRange }}>
                    <DateCalendar
                        value={selectedDate}
                        onChange={(newDate) => {
                            if (newDate) {
                                setRangeMode(null);
                                onRangeChange?.(null);
                                onDateSelect(newDate);
                            }
                        }}
                        views={['day']}
                        openTo="day"
                        referenceDate={viewDate}
                        onMonthChange={(newDate) => {
                            setViewDate(newDate);
                            onMonthChange(newDate);
                        }}
                        slots={{
                            day: CustomDay,
                        }}
                        sx={{
                            '& .MuiDayCalendar-header': {
                                display: 'none',
                            },
                            maxWidth: 320,
                        }}
                    />
                    </CustomDayContext.Provider>

                    {/* Legend */}
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                        <Typography variant="caption" sx={{
                            color: "text.secondary"
                        }}>
                            {rangeMode
                                ? `Last ${rangeMode} days`
                                : selectedDate.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
                            }
                        </Typography>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                            <Typography variant="caption" sx={{
                                color: "text.secondary"
                            }}>
                                Less
                            </Typography>
                            {[
                                { color: '#f3f4f6', label: '0' },
                                { color: '#dcfce7', label: '1-2' },
                                { color: '#86efac', label: '3-4' },
                                { color: '#22c55e', label: '5-6' },
                                { color: '#15803d', label: '7+' },
                            ].map((item) => (
                                <Tooltip key={item.label} title={item.label}>
                                    <Box
                                        sx={{
                                            width: 14,
                                            height: 14,
                                            borderRadius: 1,
                                            backgroundColor: item.color,
                                            border: '1px solid rgba(0,0,0,0.1)',
                                        }}
                                    />
                                </Tooltip>
                            ))}
                            <Typography variant="caption" sx={{
                                color: "text.secondary"
                            }}>
                                More
                            </Typography>
                        </Box>
                    </Box>
                </Box>
            </Box>
        </LocalizationProvider>
    );
};

export default RecordingCalendar;
