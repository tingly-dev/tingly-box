import { useState, useMemo } from 'react';
import { Box, Tooltip, Typography, IconButton, Button, ButtonGroup } from '@mui/material';
import { ArrowBackIosNew, ArrowForwardIos } from '@mui/icons-material';
import { LocalizationProvider, DateCalendar } from '@mui/x-date-pickers';
import { AdapterDateFns } from '@mui/x-date-pickers/AdapterDateFns';
import { PickersDay, PickersDayProps } from '@mui/x-date-pickers/PickersDay';
import type { SxProps, Theme } from '@mui/material/styles';

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

const RecordingCalendar: React.FC<RecordingCalendarProps> = ({
  currentDate,
  selectedDate,
  recordingCounts,
  onDateSelect,
  onMonthChange,
  onRangeChange,
  sx = {},
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

  // Custom PickersDay with activity color
  const CustomDay = (props: PickersDayProps<Date>) => {
    const { day, outsideCurrentMonth, ...other } = props;
    const dateKey = formatDateKey(day);
    const count = recordingCounts.get(dateKey) || 0;
    const level = getActivityLevel(count);
    const isSelected = isSameDay(day, selectedDate);
    const inRange = isInRange(day);

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
        <PickersDay
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
          {day.getDate()}
        </PickersDay>
      </Tooltip>
    );
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
            defaultCalendarMonth={viewDate}
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

          {/* Legend */}
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Typography variant="caption" color="text.secondary">
              {rangeMode
                ? `Last ${rangeMode} days`
                : selectedDate.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
              }
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <Typography variant="caption" color="text.secondary">
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
              <Typography variant="caption" color="text.secondary">
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
