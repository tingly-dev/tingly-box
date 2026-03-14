import { ContentPaste as PasteIcon, Upload as UploadIcon, Code as CodeIcon } from '@mui/icons-material';
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
} from '@mui/material';
import { useState } from 'react';

interface ImportModalProps {
    open: boolean;
    onClose: () => void;
    onImport: (data: string) => void;
    loading?: boolean;
}

const TabPanel = styled(Box)<{ value: number; index: number }>(
    ({ theme, value, index }) => ({
        display: value !== index ? 'none' : 'block',
        padding: theme.spacing(2),
    })
);

export const ImportModal = ({ open, onClose, onImport, loading = false }: ImportModalProps) => {
    const [tabValue, setTabValue] = useState(0);
    const [jsonlData, setJsonlData] = useState('');
    const [base64Data, setBase64Data] = useState('');
    const [fileName, setFileName] = useState<string>('');

    const handleClose = () => {
        setJsonlData('');
        setBase64Data('');
        setFileName('');
        setTabValue(0);
        onClose();
    };

    const handleJsonlImport = () => {
        const trimmed = jsonlData.trim();
        if (trimmed) onImport(trimmed);
    };

    const handleBase64Import = () => {
        const trimmed = base64Data.trim();
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

    return (
        <Dialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
            <DialogTitle>Import Rule</DialogTitle>
            <DialogContent>
                <Tabs
                    value={tabValue}
                    onChange={(_, newValue) => setTabValue(newValue)}
                    sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}
                >
                    <Tab label="JSONL" icon={<CodeIcon />} disabled={loading} />
                    <Tab label="Base64" icon={<PasteIcon />} disabled={loading} />
                    <Tab label="Upload File" icon={<UploadIcon />} disabled={loading} />
                </Tabs>

                <TabPanel value={tabValue} index={0}>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                        Paste the JSONL formatted rule export data below.
                    </Typography>
                    <TextField
                        fullWidth
                        multiline
                        rows={8}
                        placeholder='{"type":"metadata","version":"1.0",...}\n{"type":"rule",...}'
                        value={jsonlData}
                        onChange={(e) => setJsonlData(e.target.value)}
                        disabled={loading}
                        sx={{ fontFamily: 'monospace', fontSize: '0.85rem' }}
                    />
                </TabPanel>

                <TabPanel value={tabValue} index={1}>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                        Paste the base64 encoded rule export data below.
                    </Typography>
                    <TextField
                        fullWidth
                        multiline
                        rows={8}
                        placeholder="TGB64:1.0:..."
                        value={base64Data}
                        onChange={(e) => setBase64Data(e.target.value)}
                        disabled={loading}
                        sx={{ fontFamily: 'monospace', fontSize: '0.85rem' }}
                    />
                </TabPanel>

                <TabPanel value={tabValue} index={2}>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                        Upload a file containing the rule export data (JSONL or Base64 format).
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
            <DialogActions>
                <Button onClick={handleClose} disabled={loading}>
                    Cancel
                </Button>
                {tabValue === 0 && (
                    <Button
                        onClick={handleJsonlImport}
                        variant="contained"
                        disabled={!jsonlData.trim() || loading}
                    >
                        {loading ? 'Importing...' : 'Import'}
                    </Button>
                )}
                {tabValue === 1 && (
                    <Button
                        onClick={handleBase64Import}
                        variant="contained"
                        disabled={!base64Data.trim() || loading}
                    >
                        {loading ? 'Importing...' : 'Import'}
                    </Button>
                )}
            </DialogActions>
        </Dialog>
    );
};

export default ImportModal;
