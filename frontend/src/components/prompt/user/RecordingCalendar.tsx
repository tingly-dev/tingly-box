import { useState } from 'react';
import { Box, Tooltip, Typography, IconButton, ButtonGroup, Button } from '@mui/material';
import { ArrowBackIosNew, ArrowForwardIos } from '@mui/icons-material';
import { LocalizationProvider, DateCalendar } from '@mui/x-date-pickers';
import { AdapterDateFns } from '@mui/x-date-pickers/AdapterDateFns';
import { PickersDay, PickersDayProps } from '@mui/x-date-pickers/PickersDay';
import type { SxProps, Theme } from '@mui/material/styles';

interface RecordingCalendarProps {
  currentDate: Date;
  selectedDate: Date;
  recordingCounts: Map<string, number>; // date string -> count (for dot indicator)
  onDateSelect: (date: Date) => void;
  onMonthChange: (date: Date) => void;
  onRangeChange?: (days: number | null) => void; // null = single date mode, number = range mode
  rangeMode?: number | null; // External range mode for highlighting
  sx?: SxProps<Theme>;
}

const RecordingCalendar: React.FC<RecordingCalendarProps> = ({
  currentDate,
  selectedDate,
  recordingCounts,
  onDateSelect,
  onMonthChange,
  onRangeChange,
  rangeMode: externalRangeMode,
  sx = {},
}) => {
  const [viewDate, setViewDate] = useState(currentDate);
  // Use external range mode if provided, otherwise use internal state
  const [internalRangeMode, setInternalRangeMode] = useState<number | null>(null);
  const rangeMode = externalRangeMode !== undefined ? externalRangeMode : internalRangeMode;

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

  // Custom PickersDay with range highlight and activity indicator
  const CustomDay = (props: PickersDayProps<Date>) => {
    const { day, outsideCurrentMonth, ...other } = props;
    const dateKey = formatDateKey(day);
    const count = recordingCounts.get(dateKey) || 0;
    const isSelected = isSameDay(day, selectedDate);
    const inRange = isInRange(day);
    const hasActivity = count > 0;

    return (
      <Tooltip
        title={
          hasActivity
            ? `${count} recording${count > 1 ? 's' : ''} on ${day.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })}`
            : day.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
        }
        arrow
      >
        <Box sx={{ position: 'relative', display: 'inline-block' }}>
          <PickersDay
            {...other}
            day={day}
            outsideCurrentMonth={outsideCurrentMonth}
            sx={{
              backgroundColor: isSelected
                ? '#3b82f6'
                : inRange
                ? '#e0f2fe'
                : 'transparent',
              color: isSelected
                ? '#ffffff'
                : inRange
                ? '#0369a1'
                : 'inherit',
              fontWeight: isSelected || inRange ? 600 : 400,
              '&:hover': {
                backgroundColor: isSelected
                  ? '#2563eb'
                  : inRange
                  ? '#bae6fd'
                  : undefined,
              },
            }}
          >
            {day.getDate()}
          </PickersDay>
          {/* Activity dot indicator */}
          {hasActivity && !isSelected && (
            <Box
              sx={{
                position: 'absolute',
                bottom: 2,
                left: '50%',
                transform: 'translateX(-50%)',
                width: 4,
                height: 4,
                borderRadius: '50%',
                backgroundColor: inRange ? '#0369a1' : '#22c55e',
              }}
            />
          )}
        </Box>
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
    if (externalRangeMode !== undefined) {
      // External range mode is controlled, just notify parent
      if (externalRangeMode === days) {
        onRangeChange?.(null);
      } else {
        onRangeChange?.(days);
      }
    } else {
      // Internal range mode is uncontrolled
      if (internalRangeMode === days) {
        setInternalRangeMode(null);
        onRangeChange?.(null);
      } else {
        setInternalRangeMode(days);
        onRangeChange?.(days);
      }
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
                // Clear range mode when selecting a specific date
                if (externalRangeMode !== undefined) {
                  onRangeChange?.(null);
                } else {
                  setInternalRangeMode(null);
                }
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

          {/* Status */}
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Typography variant="caption" color="text.secondary">
              {rangeMode
                ? `Last ${rangeMode} days`
                : selectedDate.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
              }
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <Box
                sx={{
                  width: 4,
                  height: 4,
                  borderRadius: '50%',
                  backgroundColor: '#22c55e',
                }}
              />
              <Typography variant="caption" color="text.secondary">
                Has recordings
              </Typography>
            </Box>
          </Box>
        </Box>
      </Box>
    </LocalizationProvider>
  );
};

export default RecordingCalendar;
