import { Close } from '@mui/icons-material';
import {
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControl,
    IconButton,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { useState, useEffect } from 'react';
import { type IDESource } from '@/types/prompt';
import { IDE_SOURCES } from '@/constants/ideSources';

interface AddSkillLocationData {
    name: string;
    path: string;
    ide_source: IDESource;
}

interface AddSkillLocationDialogProps {
    open: boolean;
    onClose: () => void;
    onSubmit: (data: AddSkillLocationData) => Promise<void>;
    initialData?: AddSkillLocationData;
    mode?: 'add' | 'edit';
}

const AddSkillLocationDialog = ({
    open,
    onClose,
    onSubmit,
    initialData,
    mode = 'add',
}: AddSkillLocationDialogProps) => {
    const [formData, setFormData] = useState<AddSkillLocationData>({
        name: '',
        path: '',
        ide_source: 'claude_code' as IDESource,
    });
    const [submitting, setSubmitting] = useState(false);

    useEffect(() => {
        if (open) {
            if (initialData) {
                setFormData(initialData);
            } else {
                setFormData({
                    name: '',
                    path: '',
                    ide_source: 'claude_code' as IDESource,
                });
            }
        }
    }, [open, initialData]);

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();

        if (!formData.name.trim() || !formData.path.trim() || !formData.ide_source) {
            return;
        }

        setSubmitting(true);
        try {
            await onSubmit(formData);
            handleClose();
        } finally {
            setSubmitting(false);
        }
    };

    const handleClose = () => {
        setFormData({
            name: '',
            path: '',
            ide_source: 'claude_code' as IDESource,
        });
        onClose();
    };

    const isValid = formData.name.trim() && formData.path.trim() && formData.ide_source;

    return (
        <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
            <DialogTitle>
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <Typography variant="h6">
                        {mode === 'add' ? 'Add Skill Location' : 'Edit Skill Location'}
                    </Typography>
                    <IconButton aria-label="close" onClick={handleClose} size="small">
                        <Close />
                    </IconButton>
                </Box>
            </DialogTitle>
            <form onSubmit={handleSubmit}>
                <DialogContent sx={{ pb: 1 }}>
                    <Stack spacing={2.5}>
                        <TextField
                            size="small"
                            fullWidth
                            label="Name"
                            value={formData.name}
                            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                            required
                            placeholder="My Custom Skills"
                        />

                        <TextField
                            size="small"
                            fullWidth
                            label="Path"
                            value={formData.path}
                            onChange={(e) => setFormData({ ...formData, path: e.target.value })}
                            required
                            placeholder="/Users/username/.claude/skills"
                            helperText="Full path to the skills directory"
                        />

                        <FormControl size="small" fullWidth required>
                            <InputLabel>IDE Source</InputLabel>
                            <Select
                                value={formData.ide_source}
                                label="IDE Source"
                                onChange={(e) =>
                                    setFormData({
                                        ...formData,
                                        ide_source: e.target.value as IDESource,
                                    })
                                }
                            >
                                {(Object.keys(IDE_SOURCES) as Array<keyof typeof IDE_SOURCES>).map(
                                    (key) => (
                                        <MenuItem key={key} value={key}>
                                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                                <span>{IDE_SOURCES[key].icon}</span>
                                                <span>{IDE_SOURCES[key].name}</span>
                                            </Box>
                                        </MenuItem>
                                    )
                                )}
                            </Select>
                        </FormControl>
                    </Stack>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={handleClose} size="small">
                        Cancel
                    </Button>
                    <Button
                        type="submit"
                        variant="contained"
                        size="small"
                        disabled={!isValid || submitting}
                    >
                        {submitting ? (
                            <>
                                <CircularProgress size={16} sx={{ mr: 1 }} />
                                {mode === 'add' ? 'Adding...' : 'Saving...'}
                            </>
                        ) : mode === 'add' ? (
                            'Add Location'
                        ) : (
                            'Save Changes'
                        )}
                    </Button>
                </DialogActions>
            </form>
        </Dialog>
    );
};

export default AddSkillLocationDialog;
