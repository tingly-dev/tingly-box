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

interface AddProviderDialogProps {
    open: boolean;
    onClose: () => void;
    onSubmit: (e: React.FormEvent) => void;
    providerName: string;
    onProviderNameChange: (value: string) => void;
    providerApiBase: string;
    onProviderApiBaseChange: (value: string) => void;
    providerApiStyle: string;
    onProviderApiStyleChange: (value: string) => void;
    providerToken: string;
    onProviderTokenChange: (value: string) => void;
}

const AddProviderDialog = ({
    open,
    onClose,
    onSubmit,
    providerName,
    onProviderNameChange,
    providerApiBase,
    onProviderApiBaseChange,
    providerApiStyle,
    onProviderApiStyleChange,
    providerToken,
    onProviderTokenChange,
}: AddProviderDialogProps) => {
    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>Add New Provider</DialogTitle>
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
                            <InputLabel id="api-style-label">API Style</InputLabel>
                            <Select
                                labelId="api-style-label"
                                value={providerApiStyle}
                                label="API Style"
                                onChange={(e) => onProviderApiStyleChange(e.target.value)}
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
                    <Button type="submit" variant="contained">Add Provider</Button>
                </DialogActions>
            </form>
        </Dialog>
    );
};

export default AddProviderDialog;