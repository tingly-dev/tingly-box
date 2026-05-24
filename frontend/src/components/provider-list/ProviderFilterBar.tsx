import React from 'react';
import { Box, TextField, InputAdornment, MenuItem, Select, FormControl, InputLabel } from '@mui/material';
import { Search as SearchIcon } from '@/components/icons';

interface FilterState {
  search: string;
  protocol: 'all' | 'openai' | 'anthropic' | 'both';
  sort: 'default' | 'name' | 'nameZh';
}

interface ProviderFilterBarProps {
  filter: FilterState;
  resultCount: number;
  totalCount: number;
  onFilterChange: (updates: Partial<FilterState>) => void;
}

const ProviderFilterBar: React.FC<ProviderFilterBarProps> = ({
  filter,
  resultCount,
  totalCount,
  onFilterChange,
}) => {
  const isFiltering = filter.search || filter.protocol !== 'all' || filter.sort !== 'default';

  return (
    <Box sx={{ mb: 2, display: 'flex', flexWrap: 'wrap', gap: 2, alignItems: 'center' }}>
      {/* Search */}
      <TextField
        size="small"
        placeholder="Search providers..."
        value={filter.search}
        onChange={(e) => onFilterChange({ search: e.target.value })}
        InputProps={{
          startAdornment: (
            <InputAdornment position="start">
              <SearchIcon fontSize="small" />
            </InputAdornment>
          ),
        }}
        sx={{
          minWidth: 200,
          flexGrow: 1,
          maxWidth: 400,
          '& .MuiInputBase-root': {
            borderRadius: 1,
          },
        }}
      />

      {/* Protocol Filter */}
      <FormControl size="small" sx={{ minWidth: 140 }}>
        <InputLabel>Protocol</InputLabel>
        <Select
          value={filter.protocol}
          label="Protocol"
          onChange={(e) => onFilterChange({ protocol: e.target.value as FilterState['protocol'] })}
        >
          <MenuItem value="all">All</MenuItem>
          <MenuItem value="openai">OpenAI Only</MenuItem>
          <MenuItem value="anthropic">Anthropic Only</MenuItem>
          <MenuItem value="both">Both</MenuItem>
        </Select>
      </FormControl>

      {/* Sort */}
      <FormControl size="small" sx={{ minWidth: 140 }}>
        <InputLabel>Sort</InputLabel>
        <Select
          value={filter.sort}
          label="Sort"
          onChange={(e) => onFilterChange({ sort: e.target.value as FilterState['sort'] })}
        >
          <MenuItem value="default">Default</MenuItem>
          <MenuItem value="name">Name (A-Z)</MenuItem>
          <MenuItem value="nameZh">中文拼音</MenuItem>
        </Select>
      </FormControl>

      {/* Result count */}
      <Box sx={{ ml: 'auto', color: 'text.secondary', fontSize: '0.875rem' }}>
        {isFiltering ? `${resultCount} of ${totalCount} providers` : `${totalCount} providers`}
      </Box>
    </Box>
  );
};

export default ProviderFilterBar;
