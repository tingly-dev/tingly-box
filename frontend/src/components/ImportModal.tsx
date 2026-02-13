import { ContentPaste as PasteIcon, Upload as UploadIcon } from '@mui/icons-material';
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
import React, { useState, useCallback } from 'react';

interface ImportModalProps {
    open: boolean;
    onClose: () => void;
    onImport: (data: string) => void;
    loading?: boolean;
}

// Styled Tab Panel component
const TabPanel = styled(Box)<{ value: number; index: number }>(({ theme, value, index }) => ({
    display: value !== index ? 'none' : 'block',
    padding: theme.spacing(2),
}));

export const ImportModal: React.FC<ImportModalProps> = ({
    open,
    onClose,
    onImport,
    loading = false,
}) => {
    const [tabValue, setTabValue] = useState(0);
    const [pasteData, setPasteData] = useState('');
    const [fileName, setFileName] = useState<string>('');

    const handleTabChange = useCallback((_: React.SyntheticEvent, newValue: number) => {
        setTabValue(newValue);
    }, []);

    const handleFileChange = useCallback((event: React.ChangeEvent<HTMLInputElement>) => {
        const file = event.target.files?.[0];
        if (file) {
            setFileName(file.name);
            const reader = new FileReader();
            reader.onload = (e) => {
                const content = e.target?.result as string;
                onImport(content);
            };
            reader.readAsText(file);
        }
    }, [onImport]);

    const handlePasteImport = useCallback(() => {
        const trimmed = pasteData.trim();
        if (trimmed) {
            onImport(trimmed);
        }
    }, [pasteData, onImport]);

    const handleClose = useCallback(() => {
        setPasteData('');
        setFileName('');
        setTabValue(0);
        onClose();
    }, [onClose]);

    return (
        <Dialog
            open={open}
            onClose={handleClose}
            maxWidth="md"
            fullWidth
        >
            <DialogTitle>Import Rule</DialogTitle>
            <DialogContent>
                <Tabs
                    value={tabValue}
                    onChange={handleTabChange}
                    sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}
                >
                    <Tab
                        label="Paste Data"
                        icon={<PasteIcon />}
                        disabled={loading}
                    />
                    <Tab
                        label="Upload File"
                        icon={<UploadIcon />}
                        disabled={loading}
                    />
                </Tabs>

                <TabPanel value={tabValue} index={0}>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                        Paste the base64 encoded rule export data below.
                    </Typography>
                    <TextField
                        fullWidth
                        multiline
                        rows={8}
                        placeholder="TGB64:1.0:..."
                        value={pasteData}
                        onChange={(e) => setPasteData(e.target.value)}
                        disabled={loading}
                        placeholder="TGB64:1.0:..."
                        sx={{
                            fontFamily: 'monospace',
                            fontSize: '0.85rem',
                        }}
                    />
                </TabPanel>

                <TabPanel value={tabValue} index={1}>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                        Upload a file containing base64 encoded rule export data.
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
                        onClick={handlePasteImport}
                        variant="contained"
                        disabled={!pasteData.trim() || loading}
                    >
                        {loading ? 'Importing...' : 'Import'}
                    </Button>
                )}
            </DialogActions>
        </Dialog>
    );
};

export default ImportModal;
