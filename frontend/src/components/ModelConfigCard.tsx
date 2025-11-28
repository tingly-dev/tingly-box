import {
    Box,
    Button,
    FormControl,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    TextField,
} from '@mui/material';
import { useState } from 'react';
import UnifiedCard from './UnifiedCard';
import { api } from '../services/api';

interface ModelConfigCardProps {
    defaults: any;
    providers: any[];
    providerModels: any;
    onLoadDefaults: () => Promise<void>;
    onLoadProviderSelectionPanel: () => Promise<void>;
}

const ModelConfigCard = ({
    defaults,
    providers,
    providerModels,
    onLoadDefaults,
    onLoadProviderSelectionPanel,
}: ModelConfigCardProps) => {
    const [requestModelName, setRequestModelName] = useState(defaults.requestModel || 'tingly');
    const [responseModelName, setResponseModelName] = useState(defaults.responseModel || '');
    const [defaultProvider, setDefaultProvider] = useState(defaults.defaultProvider || '');
    const [defaultModel, setDefaultModel] = useState(defaults.defaultModel || '');
    const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

    const handleProviderChange = (provider: string) => {
        setDefaultProvider(provider);
        if (provider) {
            const providerData = providerModels[provider];
            const models = providerData ? providerData.models : [];
            if (models.length > 0) {
                setDefaultModel(models[0]);
            }
        } else {
            setDefaultModel('');
        }
    };

    const handleSaveDefaults = async () => {
        if (!requestModelName) {
            setMessage({ type: 'error', text: 'Request model name is required' });
            return;
        }

        if (defaultProvider && !defaultModel) {
            setMessage({ type: 'error', text: 'Please select a model when setting a default provider' });
            return;
        }

        const result = await api.setDefaults({
            requestModel: requestModelName,
            responseModel: responseModelName,
            defaultProvider,
            defaultModel,
        });

        if (result.success) {
            setMessage({ type: 'success', text: 'Defaults saved successfully' });
            await onLoadProviderSelectionPanel();
        } else {
            setMessage({ type: 'error', text: result.error });
        }
    };

    return (
        <UnifiedCard
            title="Default Model Configuration"
            subtitle="Configure default provider and model settings"
            size="fullw"
            message={message}
            onClearMessage={() => setMessage(null)}
        >
            <Stack spacing={2}>
                <Box
                    sx={{
                        width: '100%',
                        overflowX: 'auto',
                        py: 1,
                        '&::-webkit-scrollbar': {
                            height: 8,
                        },
                        '&::-webkit-scrollbar-track': {
                            backgroundColor: 'grey.100',
                            borderRadius: 1,
                        },
                        '&::-webkit-scrollbar-thumb': {
                            backgroundColor: 'grey.300',
                            borderRadius: 1,
                        },
                    }}
                >
                    <Stack
                        direction="row"
                        spacing={2}
                        sx={{
                            minWidth: 'max-content',
                            alignItems: 'flex-start',
                        }}
                    >
                        <TextField
                            label="Request Model Name"
                            value={requestModelName}
                            onChange={(e) => setRequestModelName(e.target.value)}
                            helperText="When requests use this model name"
                            sx={{ minWidth: 250, width: 250 }}
                            size="small"
                        />

                        <TextField
                            label="Response Model"
                            value={responseModelName}
                            onChange={(e) => setResponseModelName(e.target.value)}
                            helperText="Response as model. Empty for as it is."
                            sx={{ minWidth: 250, width: 250 }}
                            size="small"
                        />

                        <FormControl sx={{ minWidth: 250, width: 250 }} size="small">
                            <InputLabel>Default Provider</InputLabel>
                            <Select
                                value={defaultProvider}
                                onChange={(e) => handleProviderChange(e.target.value)}
                                label="Default Provider"
                            >
                                <MenuItem value="">Select a provider</MenuItem>
                                {providers.map((provider) => (
                                    <MenuItem key={provider.name} value={provider.name}>
                                        {provider.name} ({provider.enabled ? 'enabled' : 'disabled'})
                                    </MenuItem>
                                ))}
                            </Select>
                        </FormControl>

                        <FormControl sx={{ minWidth: 250, width: 250 }} size="small" disabled={!defaultProvider}>
                            <InputLabel>Default Model</InputLabel>
                            <Select
                                value={defaultModel}
                                onChange={(e) => setDefaultModel(e.target.value)}
                                label="Default Model"
                            >
                                <MenuItem value="">Select a model</MenuItem>
                                {providerModels[defaultProvider]?.models.map((model: string) => (
                                    <MenuItem key={model} value={model}>
                                        {model}
                                    </MenuItem>
                                ))}
                            </Select>
                        </FormControl>
                    </Stack>
                </Box>

                <Stack direction="row" spacing={2}>
                    <Button variant="contained" onClick={handleSaveDefaults}>
                        Save Defaults
                    </Button>
                    <Button variant="outlined" onClick={onLoadDefaults}>
                        Refresh Models
                    </Button>
                </Stack>
            </Stack>
        </UnifiedCard>
    );
};

export default ModelConfigCard;
