import { Box, TextField, FormControl, InputLabel, Select, MenuItem, Chip } from '@mui/material';
import { Search } from '@/components/icons';
import type { RecordingType } from '@/types/prompt';
import { useTranslation } from 'react-i18next';

const RECORDING_TYPE_LABELS: Record<RecordingType, string> = {
  'code-review': 'Code Review',
  'debug': 'Debug',
  'refactor': 'Refactor',
  'test': 'Test',
  'custom': 'Custom',
};

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
  const { t } = useTranslation();
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
        placeholder={t('prompt.user.searchPlaceholder')}
        value={searchQuery}
        onChange={(e) => onSearchChange(e.target.value)}
        sx={{ minWidth: 200, flex: 1, maxWidth: 300 }}
        size="small"
        slotProps={{
          input: {
            startAdornment: <Search sx={{ mr: 1, color: 'text.secondary' }} />,
          }
        }}
      />
      {/* User Filter */}
      <FormControl size="small" sx={{ minWidth: 130 }}>
        <InputLabel>{t('prompt.user.userFilter')}</InputLabel>
        <Select
          value={userFilter || ''}
          label={t('prompt.user.userFilter')}
          onChange={(e) => onUserFilterChange(e.target.value || undefined)}
        >
          <MenuItem value="">{t('prompt.user.allUsers')}</MenuItem>
          {users.map((user) => (
            <MenuItem key={user} value={user}>
              {user}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      {/* Workspace filter: the IDE workspace a recording was made in */}
      <FormControl size="small" sx={{ minWidth: 130 }}>
        <InputLabel>{t('prompt.user.projectFilter')}</InputLabel>
        <Select
          value={projectFilter || ''}
          label={t('prompt.user.projectFilter')}
          onChange={(e) => onProjectFilterChange(e.target.value || undefined)}
        >
          <MenuItem value="">{t('prompt.user.allProjects')}</MenuItem>
          {projects.map((project) => (
            <MenuItem key={project} value={project}>
              {project}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      {/* Type Filter */}
      <FormControl size="small" sx={{ minWidth: 130 }}>
        <InputLabel>{t('prompt.user.typeFilter')}</InputLabel>
        <Select
          value={typeFilter || ''}
          label={t('prompt.user.typeFilter')}
          onChange={(e) => onTypeFilterChange((e.target.value || undefined) as RecordingType | undefined)}
        >
          <MenuItem value="">{t('prompt.user.allTypes')}</MenuItem>
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
              label={`${t('prompt.user.userFilter')}: ${userFilter}`}
              onDelete={() => onUserFilterChange(undefined)}
              size="small"
              color="primary"
              variant="outlined"
            />
          )}
          {projectFilter && (
            <Chip
              label={`${t('prompt.user.projectFilter')}: ${projectFilter}`}
              onDelete={() => onProjectFilterChange(undefined)}
              size="small"
              color="primary"
              variant="outlined"
            />
          )}
          {typeFilter && (
            <Chip
              label={`${t('prompt.user.typeFilter')}: ${RECORDING_TYPE_LABELS[typeFilter]}`}
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
