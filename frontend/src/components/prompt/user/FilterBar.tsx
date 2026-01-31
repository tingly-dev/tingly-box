import { Box, TextField, FormControl, InputLabel, Select, MenuItem, Chip } from '@mui/material';
import { Search } from '@mui/icons-material';
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
}) => {
  const handleClearFilters = () => {
    onUserFilterChange(undefined);
    onProjectFilterChange(undefined);
    onTypeFilterChange(undefined);
  };

  const hasActiveFilters = userFilter || projectFilter || typeFilter;

  return (
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
      {hasActiveFilters && (
        <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', ml: 'auto' }}>
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
          <Chip
            label="Clear all"
            onClick={handleClearFilters}
            size="small"
            color="error"
            variant="outlined"
            sx={{ cursor: 'pointer' }}
          />
        </Box>
      )}
    </Box>
  );
};

export default FilterBar;
