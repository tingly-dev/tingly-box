import {
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { FileUpload } from '@/components/icons';

type FragmentImportDialogProps = {
    open: boolean;
    importing: boolean;
    importText: string;
    importFileName: string;
    onImportTextChange: (text: string) => void;
    onChooseFile: () => void;
    onClose: () => void;
    onSubmit: () => void;
};

const FragmentImportDialog = ({
    open,
    importing,
    importText,
    importFileName,
    onImportTextChange,
    onChooseFile,
    onClose,
    onSubmit,
}: FragmentImportDialogProps) => (
    <Dialog
        open={open}
        onClose={() => !importing && onClose()}
        disableRestoreFocus
        fullWidth
        maxWidth="md"
    >
        <DialogTitle>Import Policy Fragment</DialogTitle>
        <DialogContent>
            <Stack spacing={2} sx={{ pt: 1 }}>
                <Typography variant="body2" sx={{
                    color: "text.secondary"
                }}>
                    Import a YAML or JSON policy fragment containing one or more policies. Imported policies are appended to `guardrails/custom/import.yaml`.
                </Typography>
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
                    <Button variant="outlined" startIcon={<FileUpload />} onClick={onChooseFile}>
                        Choose File
                    </Button>
                    {importFileName ? (
                        <Chip size="small" label={importFileName} />
                    ) : null}
                </Stack>
                <TextField
                    label="Fragment Content"
                    value={importText}
                    onChange={(e) => onImportTextChange(e.target.value)}
                    multiline
                    minRows={16}
                    fullWidth
                    placeholder={'policies:\n  - id: block-ssh-read\n    name: Block SSH Read\n    kind: resource_access\n    enabled: false\n    groups: [default]\n    ...'}
                />
            </Stack>
        </DialogContent>
        <DialogActions>
            <Button onClick={onClose} disabled={importing}>
                Cancel
            </Button>
            <Button variant="contained" onClick={onSubmit} disabled={importing}>
                Import
            </Button>
        </DialogActions>
    </Dialog>
);

export default FragmentImportDialog;
