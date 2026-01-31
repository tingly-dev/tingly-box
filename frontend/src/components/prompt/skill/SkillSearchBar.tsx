import { Box, TextField, FormControl, InputLabel, Select, MenuItem } from '@mui/material';
import { Search } from '@mui/icons-material';
import type { IDESource } from '@/types/prompt';

// Minimal list for filter dropdown (backend provides full data)
const IDE_FILTER_OPTIONS: { value: IDESource; label: string; icon: string }[] = [
  { value: 'claude_code', label: 'Claude Code', icon: '🎨' },
  { value: 'opencode', label: 'OpenCode', icon: '💻' },
  { value: 'vscode', label: 'VS Code', icon: '💡' },
  { value: 'cursor', label: 'Cursor', icon: '🎯' },
  { value: 'codex', label: 'Codex', icon: '📜' },
  { value: 'antigravity', label: 'Antigravity', icon: '🔄' },
  { value: 'amp', label: 'Amp', icon: '⚡' },
  { value: 'kilo_code', label: 'Kilo Code', icon: '🪜' },
  { value: 'roo_code', label: 'Roo Code', icon: '🦘' },
  { value: 'goose', label: 'Goose', icon: '🪿' },
  { value: 'gemini_cli', label: 'Gemini CLI', icon: '💎' },
  { value: 'github_copilot', label: 'GitHub Copilot', icon: '🐙' },
  { value: 'clawdbot', label: 'Clawdbot', icon: '🦞' },
  { value: 'droid', label: 'Droid', icon: '🤖' },
  { value: 'windsurf', label: 'Windsurf', icon: '🌊' },
];

interface SkillSearchBarProps {
  searchQuery: string;
  onSearchChange: (value: string) => void;
  ideFilter?: IDESource;
  onIdeFilterChange: (value?: IDESource) => void;
}

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
          {IDE_FILTER_OPTIONS.map((option) => (
            <MenuItem key={option.value} value={option.value}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <span>{option.icon}</span>
                <span>{option.label}</span>
              </Box>
            </MenuItem>
          ))}
        </Select>
      </FormControl>
    </Box>
  );
};

export default SkillSearchBar;
