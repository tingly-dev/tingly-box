import {
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControl,
    FormControlLabel,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    Switch,
    TextField,
} from '@mui/material';

export interface ProviderFormData {
    name: string;
    apiBase: string;
    apiStyle: 'openai' | 'anthropic';
    token: string;
    enabled?: boolean;
}

interface ProviderFormDialogProps {
    open: boolean;
    onClose: () => void;
    onSubmit: (e: React.FormEvent) => void;
    data: ProviderFormData;
    onChange: (field: keyof ProviderFormData, value: any) => void;
    mode: 'add' | 'edit';
    title?: string;
    submitText?: string;
}

const ProviderFormDialog = ({
    open,
    onClose,
    onSubmit,
    data,
    onChange,
    mode,
    title,
    submitText,
}: ProviderFormDialogProps) => {
    const defaultTitle = mode === 'add' ? 'Add New API Key' : 'Edit API Key';
    const defaultSubmitText = mode === 'add' ? 'Add API Key' : 'Save Changes';

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>{title || defaultTitle}</DialogTitle>
            <form onSubmit={onSubmit}>
                <DialogContent>
                    <Stack spacing={2} mt={1}>
                        <TextField
                            fullWidth
                            label="Name"
                            value={data.name}
                            onChange={(e) => onChange('name', e.target.value)}
                            required
                            placeholder="e.g., openai, anthropic"
                            autoFocus={mode === 'add'}
                        />
                        <FormControl fullWidth>
                            <InputLabel id="api-style-label">API Style</InputLabel>
                            <Select
                                labelId="api-style-label"
                                value={data.apiStyle}
                                label="API Style"
                                onChange={(e) => onChange('apiStyle', e.target.value)}
                            >
                                <MenuItem value="openai">OpenAI</MenuItem>
                                <MenuItem value="anthropic">Anthropic</MenuItem>
                            </Select>
                        </FormControl>

                        <TextField
                            fullWidth
                            label="API Base URL"
                            value={data.apiBase}
                            onChange={(e) => onChange('apiBase', e.target.value)}
                            required
                            placeholder="e.g., https://api.openai.com/v1"
                        />

                        <TextField
                            fullWidth
                            label="API Key"
                            type="password"
                            value={data.token}
                            onChange={(e) => onChange('token', e.target.value)}
                            required={mode === 'add'}
                            placeholder={mode === 'add' ? 'Your API token' : 'Leave empty to keep current token'}
                            helperText={mode === 'edit' && 'Leave empty to keep current token'}
                        />
                        {mode === 'edit' && (
                            <FormControlLabel
                                control={
                                    <Switch
                                        checked={data.enabled || false}
                                        onChange={(e) => onChange('enabled', e.target.checked)}
                                    />
                                }
                                label="Enabled"
                            />
                        )}
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={onClose}>Cancel</Button>
                    <Button type="submit" variant="contained">
                        {submitText || defaultSubmitText}
                    </Button>
                </DialogActions>
            </form>
        </Dialog>
    );
};

export default ProviderFormDialog;