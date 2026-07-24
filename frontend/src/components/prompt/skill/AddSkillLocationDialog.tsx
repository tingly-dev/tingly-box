import {
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    FormControl,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    TextField,
} from '@mui/material';
import { useState, useEffect } from 'react';
import { type IDESource } from '@/types/prompt';
import { IDE_SOURCES } from '@/constants/ideSources';
import DialogHeader from '@/components/DialogHeader';

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
    const dialogTitle = mode === 'add' ? 'Add Skill Location' : 'Edit Skill Location';

    return (
        <Dialog
            open={open}
            onClose={submitting ? undefined : handleClose}
            aria-labelledby="skill-location-dialog-title"
            maxWidth="sm"
            fullWidth
        >
            <DialogHeader
                title={dialogTitle}
                titleId="skill-location-dialog-title"
                closeLabel={`Close ${dialogTitle.toLowerCase()}`}
                onClose={handleClose}
                closeDisabled={submitting}
            />
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
                            <InputLabel id="skill-location-ide-source-label">
                                IDE Source
                            </InputLabel>
                            <Select
                                labelId="skill-location-ide-source-label"
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
                                            {IDE_SOURCES[key].name}
                                        </MenuItem>
                                    )
                                )}
                            </Select>
                        </FormControl>
                    </Stack>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={handleClose} size="small" disabled={submitting}>
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
