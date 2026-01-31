import {
  Box,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Button,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
} from '@mui/material';
import { useState } from 'react';
import type { IDESource } from '@/types/prompt';

// Minimal list for dropdown selection (backend provides full data)
const IDE_SOURCE_OPTIONS: { value: IDESource; label: string; icon: string }[] = [
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
  { value: 'custom', label: 'Custom', icon: '📂' },
];

interface AddPathDialogProps {
  open: boolean;
  onClose: () => void;
  onAdd: (data: { name: string; path: string; ideSource: IDESource }) => void;
}

const AddPathDialog: React.FC<AddPathDialogProps> = ({ open, onClose, onAdd }) => {
  const [name, setName] = useState('');
  const [path, setPath] = useState('');
  const [ideSource, setIdeSource] = useState<IDESource>('claude_code');

  const handleAdd = () => {
    if (name.trim() && path.trim()) {
      onAdd({ name: name.trim(), path: path.trim(), ideSource });
      // Reset form
      setName('');
      setPath('');
      setIdeSource('claude_code');
    }
  };

  const handleClose = () => {
    // Reset form
    setName('');
    setPath('');
    setIdeSource('claude_code');
    onClose();
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Add Skill Path</DialogTitle>
      <DialogContent>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, mt: 1 }}>
          <TextField
            label="Display Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g., My Claude Code Skills"
            fullWidth
            autoFocus
          />
          <TextField
            label="Path"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder="/path/to/skills"
            fullWidth
          />
          <FormControl fullWidth>
            <InputLabel>IDE Source</InputLabel>
            <Select
              value={ideSource}
              label="IDE Source"
              onChange={(e) => setIdeSource(e.target.value as IDESource)}
            >
              {IDE_SOURCE_OPTIONS.map((option) => (
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
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        <Button
          onClick={handleAdd}
          variant="contained"
          disabled={!name.trim() || !path.trim()}
        >
          Add
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default AddPathDialog;
