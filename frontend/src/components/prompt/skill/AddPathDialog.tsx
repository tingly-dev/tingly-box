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

interface AddPathDialogProps {
  open: boolean;
  onClose: () => void;
  onAdd: (data: { name: string; path: string; ideSource: IDESource }) => void;
}

const IDE_OPTIONS: { value: IDESource; label: string }[] = [
  { value: 'claude-code', label: 'Claude Code' },
  { value: 'opencode', label: 'OpenCode' },
  { value: 'vscode', label: 'VS Code' },
  { value: 'cursor', label: 'Cursor' },
  { value: 'custom', label: 'Custom' },
];

const AddPathDialog: React.FC<AddPathDialogProps> = ({ open, onClose, onAdd }) => {
  const [name, setName] = useState('');
  const [path, setPath] = useState('');
  const [ideSource, setIdeSource] = useState<IDESource>('claude-code');

  const handleAdd = () => {
    if (name.trim() && path.trim()) {
      onAdd({ name: name.trim(), path: path.trim(), ideSource });
      // Reset form
      setName('');
      setPath('');
      setIdeSource('claude-code');
    }
  };

  const handleClose = () => {
    // Reset form
    setName('');
    setPath('');
    setIdeSource('claude-code');
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
              {IDE_OPTIONS.map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  {option.label}
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
