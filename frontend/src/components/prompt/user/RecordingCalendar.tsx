import { useState, useMemo } from 'react';
import { Box, Tooltip, Typography, IconButton } from '@mui/material';
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
  sx = {},
}) => {
  const [viewDate, setViewDate] = useState(currentDate);

  const formatDateKey = (date: Date): string => {
    return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
  };

  const isSameDay = (date1: Date, date2: Date): boolean => {
    return formatDateKey(date1) === formatDateKey(date2);
  };

  // Custom PickersDay with activity color
  const CustomDay = (props: PickersDayProps<Date>) => {
    const { day, outsideCurrentMonth, ...other } = props;
    const dateKey = formatDateKey(day);
    const count = recordingCounts.get(dateKey) || 0;
    const level = getActivityLevel(count);
    const isSelected = isSameDay(day, selectedDate);

    // Color mapping
    const getBgColor = () => {
      if (isSelected) return '#3b82f6';
      if (outsideCurrentMonth) return 'transparent';
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
            fontWeight: count > 0 ? 600 : 400,
            '&:hover': {
              backgroundColor: isSelected ? '#2563eb' : (count > 0 ? '#bbf7d0' : undefined),
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

  const monthNames = ['January', 'February', 'March', 'April', 'May', 'June',
    'July', 'August', 'September', 'October', 'November', 'December'];

  return (
    <LocalizationProvider dateAdapter={AdapterDateFns}>
      <Box sx={{ width: '100%', ...sx }}>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          {/* Month header with navigation */}
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <IconButton onClick={handlePrevMonth} size="small">
              <ArrowBackIosNew fontSize="small" />
            </IconButton>
            <Typography sx={{ fontSize: '1rem', fontWeight: 600 }}>
              {monthNames[viewDate.getMonth()]} {viewDate.getFullYear()}
            </Typography>
            <IconButton onClick={handleNextMonth} size="small">
              <ArrowForwardIos fontSize="small" />
            </IconButton>
          </Box>

          {/* MUI DateCalendar */}
          <DateCalendar
            value={selectedDate}
            onChange={(newDate) => newDate && onDateSelect(newDate)}
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
              {selectedDate.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })}
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
