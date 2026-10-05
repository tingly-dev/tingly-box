import { ContentPaste as PasteIcon, Upload as UploadIcon, Code as CodeIcon, CheckCircle as CheckIcon, Edit as EditIcon } from '@/components/icons';
import {
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    TextField,
    Typography,
    Tabs,
    Tab,
    styled,
    List,
    ListItem,
    ListItemIcon,
    ListItemText,
    IconButton,
} from '@mui/material';
import { useState } from 'react';
import { fontMono, fontSizes } from '@/theme/fonts';

export interface ImportResultItem {
    uuid: string;
    name: string;
    action: string;
    /** True when the imported name collided with an existing one and was auto-suffixed. */
    renamed?: boolean;
}

interface ImportModalProps {
    open: boolean;
    onClose: () => void;
    onImport: (data: string) => void;
    loading?: boolean;
    /** Set once import succeeds; switches the dialog into the result-list view. */
    result?: ImportResultItem[] | null;
    /** Opens the provider-edit dialog for one imported row. */
    onEditProvider?: (uuid: string) => void;
    /** Dismisses the result view and closes the dialog. */
    onDone?: () => void;
}

const TabPanel = styled(Box)<{ value: number; index: number }>(
    ({ theme, value, index }) => ({
        display: value !== index ? 'none' : 'block',
        padding: theme.spacing(2),
    })
);

export const ImportModal = ({
    open,
    onClose,
    onImport,
    loading = false,
    result = null,
    onEditProvider,
    onDone,
}: ImportModalProps) => {
    const [tabValue, setTabValue] = useState(0);
    const [base64Data, setBase64Data] = useState('');
    const [jsonlData, setJsonlData] = useState('');
    const [fileName, setFileName] = useState<string>('');

    const handleClose = () => {
        setBase64Data('');
        setJsonlData('');
        setFileName('');
        setTabValue(0);
        onClose();
    };

    const handleBase64Import = () => {
        const trimmed = base64Data.trim();
        if (trimmed) onImport(trimmed);
    };

    const handleJsonlImport = () => {
        const trimmed = jsonlData.trim();
        if (trimmed) onImport(trimmed);
    };

    const handleFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
        const file = event.target.files?.[0];
        if (!file) return;

        setFileName(file.name);
        const reader = new FileReader();
        reader.onload = (e) => {
            const content = e.target?.result as string;
            onImport(content);
        };
        reader.readAsText(file);
    };

    const handleDone = () => {
        setBase64Data('');
        setJsonlData('');
        setFileName('');
        setTabValue(0);
        onDone?.();
    };

    return (
        <Dialog
            open={open}
            onClose={result ? handleDone : handleClose}
            maxWidth="sm"
            fullWidth
            slotProps={{
                paper: {sx: {maxHeight: '82vh', display: 'flex', flexDirection: 'column'}}
            }}
        >
            <DialogTitle>{result ? 'Import Complete' : 'Import'}</DialogTitle>
            {result ? (
                <DialogContent>
                    <Typography variant="body2" sx={{ color: 'text.secondary', mb: 1 }}>
                        {result.length} provider{result.length === 1 ? '' : 's'} created.
                        Edit any of them below if you'd like to adjust the name or settings.
                    </Typography>
                    <List dense>
                        {result.map((item) => (
                            <ListItem
                                key={item.uuid}
                                secondaryAction={
                                    <IconButton
                                        edge="end"
                                        size="small"
                                        aria-label={`Edit ${item.name}`}
                                        onClick={() => onEditProvider?.(item.uuid)}
                                    >
                                        <EditIcon fontSize="small" />
                                    </IconButton>
                                }
                            >
                                <ListItemIcon sx={{ minWidth: 36 }}>
                                    <CheckIcon fontSize="small" color="success" />
                                </ListItemIcon>
                                <ListItemText
                                    primary={item.name}
                                    secondary={item.renamed ? 'Renamed to avoid a duplicate name' : undefined}
                                />
                            </ListItem>
                        ))}
                    </List>
                </DialogContent>
            ) : (
                <DialogContent>
                    <Tabs
                        value={tabValue}
                        onChange={(_, newValue) => setTabValue(newValue)}
                        sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}
                    >
                        <Tab label="Base64" icon={<PasteIcon />} disabled={loading} />
                        <Tab label="JSONL" icon={<CodeIcon />} disabled={loading} />
                        <Tab label="Upload File" icon={<UploadIcon />} disabled={loading} />
                    </Tabs>

                    <TabPanel value={tabValue} index={0}>
                        <Typography
                            variant="body2"
                            sx={{
                                color: "text.secondary",
                                mb: 2
                            }}>
                            Paste exported data in Base64 format below.
                        </Typography>
                        <TextField
                            fullWidth
                            multiline
                            rows={8}
                            placeholder="TGB64:1.0:..."
                            value={base64Data}
                            onChange={(e) => setBase64Data(e.target.value)}
                            disabled={loading}
                            sx={{ fontFamily: fontMono, fontSize: fontSizes.lg }}
                        />
                    </TabPanel>

                    <TabPanel value={tabValue} index={1}>
                        <Typography
                            variant="body2"
                            sx={{
                                color: "text.secondary",
                                mb: 2
                            }}>
                            Paste exported data in JSONL format below.
                        </Typography>
                        <TextField
                            fullWidth
                            multiline
                            rows={8}
                            placeholder='{"type":"metadata","version":"1.0",...}\n{"type":"rule",...}'
                            value={jsonlData}
                            onChange={(e) => setJsonlData(e.target.value)}
                            disabled={loading}
                            sx={{ fontFamily: fontMono, fontSize: fontSizes.lg }}
                        />
                    </TabPanel>

                    <TabPanel value={tabValue} index={2}>
                        <Typography
                            variant="body2"
                            sx={{
                                color: "text.secondary",
                                mb: 2
                            }}>
                            Upload a file containing exported data (JSONL or Base64 format).
                        </Typography>
                        <Button
                            variant="outlined"
                            component="label"
                            startIcon={<UploadIcon />}
                            disabled={loading}
                            sx={{ mb: 2 }}
                        >
                            Select File
                            <input
                                type="file"
                                accept=".txt,.jsonl,.json"
                                onChange={handleFileChange}
                                style={{ display: 'none' }}
                            />
                        </Button>
                        {fileName && (
                            <Typography variant="body2" sx={{ color: 'text.primary' }}>
                                Selected: {fileName}
                            </Typography>
                        )}
                    </TabPanel>
                </DialogContent>
            )}
            <DialogActions>
                {result ? (
                    <Button onClick={handleDone} variant="contained">
                        Done
                    </Button>
                ) : (
                    <>
                        <Button onClick={handleClose} disabled={loading}>
                            Cancel
                        </Button>
                        {tabValue === 0 && (
                            <Button
                                onClick={handleBase64Import}
                                variant="contained"
                                disabled={!base64Data.trim() || loading}
                            >
                                {loading ? 'Importing...' : 'Import'}
                            </Button>
                        )}
                        {tabValue === 1 && (
                            <Button
                                onClick={handleJsonlImport}
                                variant="contained"
                                disabled={!jsonlData.trim() || loading}
                            >
                                {loading ? 'Importing...' : 'Import'}
                            </Button>
                        )}
                    </>
                )}
            </DialogActions>
        </Dialog>
    );
};

export default ImportModal;
