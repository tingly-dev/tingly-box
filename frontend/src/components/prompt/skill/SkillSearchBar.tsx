import { Box, TextField, FormControl, InputLabel, Select, MenuItem } from '@mui/material';
import { Search } from '@mui/icons-material';
import type { IDESource } from '@/types/prompt';

interface SkillSearchBarProps {
  searchQuery: string;
  onSearchChange: (value: string) => void;
  ideFilter?: IDESource;
  onIdeFilterChange: (value?: IDESource) => void;
}

const IDE_OPTIONS: { value: IDESource; label: string }[] = [
  { value: 'claude-code', label: 'Claude Code' },
  { value: 'opencode', label: 'OpenCode' },
  { value: 'vscode', label: 'VS Code' },
  { value: 'cursor', label: 'Cursor' },
  { value: 'custom', label: 'Custom' },
];

const SkillSearchBar: React.FC<SkillSearchBarProps> = ({
  searchQuery,
  onSearchChange,
  ideFilter,
  onIdeFilterChange,
}) => {
  return (
    <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', flexWrap: 'wrap' }}>
      {/* Search Input */}
      <TextField
        placeholder="Search skills..."
        value={searchQuery}
        onChange={(e) => onSearchChange(e.target.value)}
        InputProps={{
          startAdornment: <Search sx={{ mr: 1, color: 'text.secondary' }} />,
        }}
        sx={{ minWidth: 250, flex: 1 }}
        size="small"
      />

      {/* IDE Filter */}
      <FormControl size="small" sx={{ minWidth: 150 }}>
        <InputLabel>IDE Source</InputLabel>
        <Select
          value={ideFilter || ''}
          label="IDE Source"
          onChange={(e) => onIdeFilterChange((e.target.value || undefined) as IDESource | undefined)}
        >
          <MenuItem value="">All IDEs</MenuItem>
          {IDE_OPTIONS.map((option) => (
            <MenuItem key={option.value} value={option.value}>
              {option.label}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
    </Box>
  );
};

export default SkillSearchBar;
