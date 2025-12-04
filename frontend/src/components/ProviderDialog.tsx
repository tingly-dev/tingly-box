import {
    Dialog,
    DialogTitle,
    DialogContent,
    DialogActions,
    Button,
    Stack,
    TextField,
    FormControl,
    InputLabel,
    Select,
    MenuItem,
} from '@mui/material';

interface ProviderDialogProps {
    open: boolean;
    onClose: () => void;
    onSubmit: (e: React.FormEvent) => void;
    providerName: string;
    onProviderNameChange: (value: string) => void;
    providerApiBase: string;
    onProviderApiBaseChange: (value: string) => void;
    providerApiVersion: string;
    onProviderApiVersionChange: (value: string) => void;
    providerToken: string;
    onProviderTokenChange: (value: string) => void;
    title?: string;
    submitText?: string;
}

export const ProviderDialog = ({
    open,
    onClose,
    onSubmit,
    providerName,
    onProviderNameChange,
    providerApiBase,
    onProviderApiBaseChange,
    providerApiVersion,
    onProviderApiVersionChange,
    providerToken,
    onProviderTokenChange,
    title = "Add New Provider",
    submitText = "Add Provider",
}: ProviderDialogProps) => {
    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>{title}</DialogTitle>
            <form onSubmit={onSubmit}>
                <DialogContent>
                    <Stack spacing={2} mt={1}>
                        <TextField
                            fullWidth
                            label="Provider Name"
                            value={providerName}
                            onChange={(e) => onProviderNameChange(e.target.value)}
                            required
                            placeholder="e.g., openai, anthropic"
                            autoFocus
                        />
                        <TextField
                            fullWidth
                            label="API Base URL"
                            value={providerApiBase}
                            onChange={(e) => onProviderApiBaseChange(e.target.value)}
                            required
                            placeholder="e.g., https://api.openai.com/v1"
                        />
                        <FormControl fullWidth>
                            <InputLabel id="api-version-label">API Version</InputLabel>
                            <Select
                                labelId="api-version-label"
                                value={providerApiVersion}
                                label="API Version"
                                onChange={(e) => onProviderApiVersionChange(e.target.value)}
                            >
                                <MenuItem value="openai">OpenAI</MenuItem>
                                <MenuItem value="anthropic">Anthropic</MenuItem>
                            </Select>
                        </FormControl>
                        <TextField
                            fullWidth
                            label="API Token"
                            type="password"
                            value={providerToken}
                            onChange={(e) => onProviderTokenChange(e.target.value)}
                            required
                            placeholder="Your API token"
                        />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={onClose}>Cancel</Button>
                    <Button type="submit" variant="contained">{submitText}</Button>
                </DialogActions>
            </form>
        </Dialog>
    );
};

export default ProviderDialog;