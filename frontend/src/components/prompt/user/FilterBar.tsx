import { Box, TextField, FormControl, InputLabel, Select, MenuItem, Chip, Button, Stack } from '@mui/material';
import { Search, CalendarToday } from '@mui/icons-material';
import { LocalizationProvider, DateRangePicker } from '@mui/x-date-pickers';
import { AdapterDateFns } from '@mui/x-date-pickers/AdapterDateFns';
import { SingleInputDateRangeField } from '@mui/x-date-pickers/SingleInputDateRangeField';
import type { RecordingType } from '@/types/prompt';
import { RECORDING_TYPE_LABELS } from '@/types/prompt';

interface FilterBarProps {
  searchQuery: string;
  onSearchChange: (value: string) => void;
  userFilter?: string;
  onUserFilterChange: (value?: string) => void;
  projectFilter?: string;
  onProjectFilterChange: (value?: string) => void;
  typeFilter?: RecordingType;
  onTypeFilterChange: (value?: RecordingType) => void;
  users: string[];
  projects: string[];
  dateRange?: [Date | null, Date | null];
  onDateRangeChange: (range: [Date | null, Date | null] | undefined) => void;
}

const FilterBar: React.FC<FilterBarProps> = ({
  searchQuery,
  onSearchChange,
  userFilter,
  onUserFilterChange,
  projectFilter,
  onProjectFilterChange,
  typeFilter,
  onTypeFilterChange,
  users,
  projects,
  dateRange,
  onDateRangeChange,
}) => {
  const handleClearFilters = () => {
    onUserFilterChange(undefined);
    onProjectFilterChange(undefined);
    onTypeFilterChange(undefined);
    onDateRangeChange(undefined);
  };

  const handleQuickDate = (days: number | 'today' | 'clear') => {
    if (days === 'clear') {
      onDateRangeChange(undefined);
      return;
    }

    const today = new Date();
    today.setHours(23, 59, 59, 999);

    if (days === 'today') {
      const start = new Date(today);
      start.setHours(0, 0, 0, 0);
      onDateRangeChange([start, today]);
    } else {
      const start = new Date(today);
      start.setDate(start.getDate() - days);
      start.setHours(0, 0, 0, 0);
      onDateRangeChange([start, today]);
    }
  };

  const hasActiveFilters = userFilter || projectFilter || typeFilter || dateRange;

  const formatDateRangeLabel = () => {
    if (!dateRange || !dateRange[0] || !dateRange[1]) return '';
    const format = (d: Date) => d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    return `${format(dateRange[0])} - ${format(dateRange[1])}`;
  };

  return (
    <LocalizationProvider dateAdapter={AdapterDateFns}>
      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
        {/* First Row: Main filters */}
        <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', flexWrap: 'wrap' }}>
          {/* Search Input */}
          <TextField
            placeholder="Search recordings..."
            value={searchQuery}
            onChange={(e) => onSearchChange(e.target.value)}
            InputProps={{
              startAdornment: <Search sx={{ mr: 1, color: 'text.secondary' }} />,
            }}
            sx={{ minWidth: 200, flex: 1, maxWidth: 300 }}
            size="small"
          />

          {/* User Filter */}
          <FormControl size="small" sx={{ minWidth: 130 }}>
            <InputLabel>User</InputLabel>
            <Select
              value={userFilter || ''}
              label="User"
              onChange={(e) => onUserFilterChange(e.target.value || undefined)}
            >
              <MenuItem value="">All Users</MenuItem>
              {users.map((user) => (
                <MenuItem key={user} value={user}>
                  {user}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          {/* Project Filter */}
          <FormControl size="small" sx={{ minWidth: 130 }}>
            <InputLabel>Project</InputLabel>
            <Select
              value={projectFilter || ''}
              label="Project"
              onChange={(e) => onProjectFilterChange(e.target.value || undefined)}
            >
              <MenuItem value="">All Projects</MenuItem>
              {projects.map((project) => (
                <MenuItem key={project} value={project}>
                  {project}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          {/* Type Filter */}
          <FormControl size="small" sx={{ minWidth: 130 }}>
            <InputLabel>Type</InputLabel>
            <Select
              value={typeFilter || ''}
              label="Type"
              onChange={(e) => onTypeFilterChange((e.target.value || undefined) as RecordingType | undefined)}
            >
              <MenuItem value="">All Types</MenuItem>
              {(Object.keys(RECORDING_TYPE_LABELS) as RecordingType[]).map((type) => (
                <MenuItem key={type} value={type}>
                  {RECORDING_TYPE_LABELS[type]}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          {/* Active Filters Display */}
          <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', ml: 'auto', flexWrap: 'wrap' }}>
            {userFilter && (
              <Chip
                label={`User: ${userFilter}`}
                onDelete={() => onUserFilterChange(undefined)}
                size="small"
                color="primary"
                variant="outlined"
              />
            )}
            {projectFilter && (
              <Chip
                label={`Project: ${projectFilter}`}
                onDelete={() => onProjectFilterChange(undefined)}
                size="small"
                color="primary"
                variant="outlined"
              />
            )}
            {typeFilter && (
              <Chip
                label={`Type: ${RECORDING_TYPE_LABELS[typeFilter]}`}
                onDelete={() => onTypeFilterChange(undefined)}
                size="small"
                color="primary"
                variant="outlined"
              />
            )}
          </Box>
        </Box>

        {/* Second Row: Date Range */}
        <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', flexWrap: 'wrap' }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1, minWidth: 0 }}>
            <CalendarToday sx={{ color: 'text.secondary', fontSize: 18 }} />
            <DateRangePicker
              value={dateRange || [null, null]}
              onChange={(newValue) => onDateRangeChange(newValue as [Date | null, Date | null] | undefined)}
              slots={{ field: SingleInputDateRangeField }}
              slotProps={{
                textField: {
                  size: 'small',
                  placeholder: 'Select date range',
                  sx={{
                    minWidth: 200,
                    flex: 1,
                    maxWidth: 300,
                  },
                },
              }}
            />
          </Box>

          {/* Quick Date Buttons */}
          <Stack direction="row" spacing={ 0.5}>
            <Button size="small" variant="outlined" onClick={() => handleQuickDate('today')}>
              Today
            </Button>
            <Button size="small" variant="outlined" onClick={() => handleQuickDate(7)}>
              7 Days
            </Button>
            <Button size="small" variant="outlined" onClick={() => handleQuickDate(30)}>
              30 Days
            </Button>
            {(dateRange || hasActiveFilters) && (
              <Button size="small" onClick={handleClearFilters} color={hasActiveFilters ? 'error' : 'inherit'}>
                Clear All
              </Button>
            )}
          </Stack>

          {/* Date Range Chip */}
          {dateRange && dateRange[0] && dateRange[1] && (
            <Chip
              icon={<CalendarToday sx={{ fontSize: 14 }} />}
              label={formatDateRangeLabel()}
              onDelete={() => onDateRangeChange(undefined)}
              size="small"
              color="info"
              variant="outlined"
              sx={{ ml: 'auto' }}
            />
          )}
        </Box>
      </Box>
    </LocalizationProvider>
  );
};

export default FilterBar;
