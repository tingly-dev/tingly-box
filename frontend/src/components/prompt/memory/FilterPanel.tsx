import { useState } from 'react';
import {
  Box,
  Button,
  Popover,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Chip,
  Divider,
  Typography,
  IconButton,
  Tooltip,
} from '@mui/material';
import {
  FilterList,
  Close,
  Event,
} from '@mui/icons-material';
import RecordingCalendar from '../user/RecordingCalendar';

interface FilterValues {
  scenario: string;
  protocol: string | undefined;
  dateRange: {
    mode: 'all' | 'range' | 'date';
    rangeDays?: number;
    selectedDate?: Date;
  };
}

interface FilterPanelProps {
  values: FilterValues;
  onChange: (values: FilterValues) => void;
  availableScenarios: { value: string; label: string }[];
  availableProtocols: { value: string | undefined; label: string }[];
}

const SCENARIOS = [
  { value: '', label: 'All Scenarios' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'claude_code', label: 'Claude Code' },
  { value: 'opencode', label: 'OpenCode' },
  { value: 'openai', label: 'OpenAI' },
];

const PROTOCOLS: { value: string | undefined; label: string }[] = [
  { value: undefined, label: 'All Protocols' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'google', label: 'Google' },
];

const FilterPanel: React.FC<FilterPanelProps> = ({
  values,
  onChange,
  availableScenarios = SCENARIOS,
  availableProtocols = PROTOCOLS,
}) => {
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);
  const [calendarAnchorEl, setCalendarAnchorEl] = useState<HTMLElement | null>(null);
  const [calendarDate, setCalendarDate] = useState<Date>(new Date());

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
  };

  const handleClearFilters = () => {
    onChange({
      scenario: '',
      protocol: undefined,
      dateRange: { mode: 'all' },
    });
  };

  const activeFilterCount = [
    values.scenario,
    values.protocol,
    values.dateRange.mode !== 'all',
  ].filter(Boolean).length;

  const getDateLabel = () => {
    if (values.dateRange.mode === 'all') return null;
    if (values.dateRange.mode === 'range' && values.dateRange.rangeDays) {
      return `Last ${values.dateRange.rangeDays} days`;
    }
    if (values.dateRange.mode === 'date' && values.dateRange.selectedDate) {
      return values.dateRange.selectedDate.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
    }
    return null;
  };

  const dateLabel = getDateLabel();

  return (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
      {/* Active Filter Chips */}
      {values.scenario && (
        <Chip
          label={SCENARIOS.find(s => s.value === values.scenario)?.label || values.scenario}
          onDelete={() => onChange({ ...values, scenario: '' })}
          size="small"
          color="primary"
          variant="outlined"
        />
      )}
      {values.protocol && (
        <Chip
          label={values.protocol}
          onDelete={() => onChange({ ...values, protocol: undefined })}
          size="small"
          color="secondary"
          variant="outlined"
        />
      )}
      {dateLabel && (
        <Chip
          label={dateLabel}
          onDelete={() => onChange({ ...values, dateRange: { mode: 'all' } })}
          size="small"
          color="info"
          variant="outlined"
        />
      )}

      {/* Filter Button */}
      <Button
        variant={activeFilterCount > 0 ? 'contained' : 'outlined'}
        size="small"
        startIcon={<FilterList />}
        onClick={handleClick}
        sx={{ textTransform: 'none' }}
        color={activeFilterCount > 0 ? 'primary' : 'inherit'}
      >
        Filters {activeFilterCount > 0 && `(${activeFilterCount})`}
      </Button>

      {/* Clear All Button */}
      {activeFilterCount > 1 && (
        <Tooltip title="Clear all filters">
          <IconButton size="small" onClick={handleClearFilters}>
            <Close fontSize="small" />
          </IconButton>
        </Tooltip>
      )}

      {/* Filter Popover */}
      <Popover
        open={Boolean(anchorEl)}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
        PaperProps={{
          sx: { p: 2, minWidth: 280 },
        }}
      >
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          {/* Date Range */}
          <Box>
            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, mb: 1, display: 'block' }}>
              Date Range
            </Typography>
            <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
              <Chip
                label="All Time"
                size="small"
                onClick={() => onChange({ ...values, dateRange: { mode: 'all' } })}
                color={values.dateRange.mode === 'all' ? 'primary' : 'default'}
                variant={values.dateRange.mode === 'all' ? 'filled' : 'outlined'}
              />
              <Chip
                label="Today"
                size="small"
                onClick={() => onChange({ ...values, dateRange: { mode: 'range', rangeDays: 0 } })}
                color={values.dateRange.rangeDays === 0 ? 'primary' : 'default'}
                variant={values.dateRange.rangeDays === 0 ? 'filled' : 'outlined'}
              />
              <Chip
                label="7 Days"
                size="small"
                onClick={() => onChange({ ...values, dateRange: { mode: 'range', rangeDays: 7 } })}
                color={values.dateRange.rangeDays === 7 ? 'primary' : 'default'}
                variant={values.dateRange.rangeDays === 7 ? 'filled' : 'outlined'}
              />
              <Chip
                label="30 Days"
                size="small"
                onClick={() => onChange({ ...values, dateRange: { mode: 'range', rangeDays: 30 } })}
                color={values.dateRange.rangeDays === 30 ? 'primary' : 'default'}
                variant={values.dateRange.rangeDays === 30 ? 'filled' : 'outlined'}
              />
              <Chip
                icon={<Event />}
                label="Pick Date"
                size="small"
                onClick={(e) => setCalendarAnchorEl(e.currentTarget as HTMLElement)}
                color={values.dateRange.mode === 'date' ? 'primary' : 'default'}
                variant={values.dateRange.mode === 'date' ? 'filled' : 'outlined'}
              />
            </Box>
          </Box>

          <Divider />

          {/* Scenario */}
          <FormControl size="small" fullWidth>
            <InputLabel>Scenario</InputLabel>
            <Select
              value={values.scenario}
              label="Scenario"
              onChange={(e) => onChange({ ...values, scenario: e.target.value })}
            >
              {availableScenarios.map((s) => (
                <MenuItem key={s.value} value={s.value}>
                  {s.label}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          {/* Protocol */}
          <FormControl size="small" fullWidth>
            <InputLabel>Protocol</InputLabel>
            <Select
              value={values.protocol || ''}
              label="Protocol"
              onChange={(e) => onChange({ ...values, protocol: e.target.value || undefined })}
            >
              {availableProtocols.map((p) => (
                <MenuItem key={p.value || 'all'} value={p.value || ''}>
                  {p.label}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Box>
      </Popover>

      {/* Calendar Popover */}
      <Popover
        open={Boolean(calendarAnchorEl)}
        anchorEl={calendarAnchorEl}
        onClose={() => setCalendarAnchorEl(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        transformOrigin={{ vertical: 'top', horizontal: 'left' }}
      >
        <Box sx={{ p: 2 }}>
          <RecordingCalendar
            currentDate={calendarDate}
            selectedDate={values.dateRange.selectedDate || new Date()}
            recordingCounts={new Map()}
            onDateSelect={(date) => {
              onChange({ ...values, dateRange: { mode: 'date', selectedDate: date } });
              setCalendarAnchorEl(null);
            }}
            onMonthChange={setCalendarDate}
          />
        </Box>
      </Popover>
    </Box>
  );
};

export default FilterPanel;
