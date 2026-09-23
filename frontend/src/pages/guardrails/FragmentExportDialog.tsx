import {
    Button,
    Checkbox,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Stack,
    Typography,
} from '@mui/material';

export type GuardrailsImportRef = {
    path: string;
    name: string;
    policy_ids?: string[];
    policy_count?: number;
};

type FragmentExportDialogProps = {
    open: boolean;
    exporting: boolean;
    imports: GuardrailsImportRef[];
    selectedExportPaths: string[];
    onTogglePath: (path: string) => void;
    onSelectAll: () => void;
    onClear: () => void;
    onClose: () => void;
    onSubmit: () => void;
};

const FragmentExportDialog = ({
    open,
    exporting,
    imports,
    selectedExportPaths,
    onTogglePath,
    onSelectAll,
    onClear,
    onClose,
    onSubmit,
}: FragmentExportDialogProps) => (
    <Dialog
        open={open}
        onClose={() => !exporting && onClose()}
        disableRestoreFocus
        fullWidth
        maxWidth="sm"
    >
        <DialogTitle>Export Imported Fragments</DialogTitle>
        <DialogContent>
            <Stack spacing={2} sx={{ pt: 1 }}>
                <Typography variant="body2" sx={{
                    color: "text.secondary"
                }}>
                    Choose one or more imported fragment files to download as-is.
                </Typography>
                <Stack direction="row" spacing={1}>
                    <Button size="small" variant="outlined" onClick={onSelectAll}>
                        Select All
                    </Button>
                    <Button size="small" variant="outlined" onClick={onClear}>
                        Clear
                    </Button>
                </Stack>
                <Stack spacing={1}>
                    {imports.map((item) => (
                        <Stack
                            key={item.path}
                            direction="row"
                            spacing={1.5}
                            sx={{
                                alignItems: "flex-start",
                                border: '1px solid',
                                borderColor: 'divider',
                                borderRadius: 2,
                                p: 1.5
                            }}>
                            <Checkbox
                                checked={selectedExportPaths.includes(item.path)}
                                onChange={() => onTogglePath(item.path)}
                                sx={{ mt: -0.5 }}
                            />
                            <Stack spacing={0.5} sx={{ minWidth: 0 }}>
                                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                    {item.name || item.path}
                                </Typography>
                                <Typography variant="caption" sx={{
                                    color: "text.secondary"
                                }}>
                                    {item.path}
                                </Typography>
                                <Typography variant="caption" sx={{
                                    color: "text.secondary"
                                }}>
                                    {`${item.policy_count || 0} policies`}
                                    {item.policy_ids && item.policy_ids.length > 0 ? ` · ${item.policy_ids.join(', ')}` : ''}
                                </Typography>
                            </Stack>
                        </Stack>
                    ))}
                </Stack>
            </Stack>
        </DialogContent>
        <DialogActions>
            <Button onClick={onClose} disabled={exporting}>
                Cancel
            </Button>
            <Button variant="contained" onClick={onSubmit} disabled={exporting}>
                Export
            </Button>
        </DialogActions>
    </Dialog>
);

export default FragmentExportDialog;
