import { CheckCircle, Error, Refresh } from '@mui/icons-material';
import {
    Alert,
    Autocomplete,
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControlLabel,
    IconButton,
    Stack,
    Switch,
    Tab,
    Tabs,
    TextField,
    Typography,
} from '@mui/material';
import React, { useState } from 'react';
import { getProviderBaseUrl } from '../data/providerUtils';
import { getProvidersByStyle, getServiceProvider } from '../data/serviceProviders';
import api from '../services/api';

interface TabPanelProps {
    children?: React.ReactNode;
    index: number;
    value: number;
}

function TabPanel(props: TabPanelProps) {
    const { children, value, index, ...other } = props;

    return (
        <div
            role="tabpanel"
            hidden={value !== index}
            id={`provider-tabpanel-${index}`}
            aria-labelledby={`provider-tab-${index}`}
            {...other}
        >
            {value === index && <Box sx={{ pt: 1 }}>{children}</Box>}
        </div>
    );
}

export interface EnhancedProviderFormData {
    name: string;
    apiBase: string;
    apiStyle: 'openai' | 'anthropic';
    token: string;
    enabled?: boolean;
}

interface EnhancedProviderFormDialogProps {
    open: boolean;
    onClose: () => void;
    onSubmit: (e: React.FormEvent) => void;
    data: EnhancedProviderFormData;
    onChange: (field: keyof EnhancedProviderFormData, value: any) => void;
    mode: 'add' | 'edit';
    title?: string;
    submitText?: string;
}

const PresetProviderFormDialog = ({
    open,
    onClose,
    onSubmit,
    data,
    onChange,
    mode,
    title,
    submitText,
}: EnhancedProviderFormDialogProps) => {
    const defaultTitle = mode === 'add' ? 'Add New API Key' : 'Edit API Key';
    const defaultSubmitText = mode === 'add' ? 'Add API Key' : 'Save Changes';

    const [tabValue, setTabValue] = useState(data.apiStyle === 'anthropic' ? 1 : 0);
    const [verifying, setVerifying] = useState(false);
    const [verificationResult, setVerificationResult] = useState<{
        success: boolean;
        message: string;
        details?: string;
        responseTime?: number;
        modelsCount?: number;
    } | null>(null);

    const openaiProviders = getProvidersByStyle('openai');
    const anthropicProviders = getProvidersByStyle('anthropic');

    const handleTabChange = (_event: React.SyntheticEvent, newValue: number) => {
        setTabValue(newValue);
        const apiStyle = newValue === 0 ? 'openai' : 'anthropic';
        onChange('apiStyle', apiStyle);
        // Clear verification result when changing tabs
        setVerificationResult(null);
    };

    // Handle provider selection
    const handleProviderSelect = (providerValue: string, apiStyle: 'openai' | 'anthropic') => {
        if (!providerValue) return;

        const [providerId] = providerValue.split(':');
        const provider = getServiceProvider(providerId);

        if (provider) {
            onChange('name', provider.name);
            onChange('apiBase', getProviderBaseUrl(provider, apiStyle));
        }
        // Clear verification result when changing provider
        setVerificationResult(null);
    };

    // Handle verification
    const handleVerify = async () => {
        if (!data.name || !data.apiBase || !data.token) {
            setVerificationResult({
                success: false,
                message: 'Please fill in all required fields (Name, API Base URL, API Key)',
            });
            return;
        }

        setVerifying(true);
        setVerificationResult(null);

        try {
            const result = await api.probeProvider(
                data.apiStyle,
                data.apiBase,
                data.token,
            )

            if (result.success && result.data) {
                setVerificationResult({
                    success: true,
                    message: result.data.message,
                    details: `Test result: ${result.data.test_result}`,
                    responseTime: result.data.response_time_ms,
                    modelsCount: result.data.models_count,
                });
            } else {
                setVerificationResult({
                    success: false,
                    message: result.error?.message || 'Verification failed',
                    details: result.error?.type,
                });
            }
        } catch (error) {
            setVerificationResult({
                success: false,
                message: 'Network error or unable to connect to verification service',
                details: error instanceof Error ? error.message : 'Unknown error',
            });
        } finally {
            setVerifying(false);
        }
    };

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>{title || defaultTitle}</DialogTitle>
            <form onSubmit={onSubmit}>
                <DialogContent sx={{ pb: 1 }}>
                    <Stack spacing={2.5}>
                        {/* API Style Tabs */}
                        <Tabs
                            value={tabValue}
                            onChange={handleTabChange}
                            variant="fullWidth"
                            sx={{ borderBottom: 1, borderColor: 'divider' }}
                        >
                            <Tab label="OpenAI Compatible" />
                            <Tab label="Anthropic Compatible" />
                        </Tabs>

                        <TabPanel value={tabValue} index={0}>
                            <Autocomplete
                                size="small"
                                options={openaiProviders}
                                getOptionLabel={(option) => option.title}
                                onChange={(_event, newValue) => {
                                    if (newValue) {
                                        handleProviderSelect(newValue.value, 'openai');
                                    }
                                }}
                                renderInput={(params) => (
                                    <TextField
                                        {...params}
                                        label="Choose a preset or config manually"
                                        placeholder="Select to auto-fill..."
                                    />
                                )}
                            />
                        </TabPanel>

                        <TabPanel value={tabValue} index={1}>
                            <Autocomplete
                                size="small"
                                options={anthropicProviders}
                                getOptionLabel={(option) => option.title}
                                onChange={(_event, newValue) => {
                                    if (newValue) {
                                        handleProviderSelect(newValue.value, 'anthropic');
                                    }
                                }}
                                renderInput={(params) => (
                                    <TextField
                                        {...params}
                                        label="Choose a preset or config manually"
                                        placeholder="Select to auto-fill..."
                                    />
                                )}
                            />
                        </TabPanel>

                        {/* Configuration Fields */}
                        <Stack spacing={2}>
                            <TextField
                                size="small"
                                fullWidth
                                label="API Key Name"
                                value={data.name}
                                onChange={(e) => {
                                    onChange('name', e.target.value);
                                    setVerificationResult(null);
                                }}
                                required
                                placeholder="e.g., OpenAI"
                            />
                            <TextField
                                size="small"
                                fullWidth
                                label="API Base URL"
                                value={data.apiBase}
                                onChange={(e) => {
                                    onChange('apiBase', e.target.value);
                                    setVerificationResult(null);
                                }}
                                required
                                placeholder={
                                    tabValue === 0
                                        ? "https://api.openai.com/v1"
                                        : "https://api.anthropic.com"
                                }
                            />
                        </Stack>

                        {/* API Key Field */}
                        <TextField
                            size="small"
                            fullWidth
                            label="API Key"
                            type="password"
                            value={data.token}
                            onChange={(e) => {
                                onChange('token', e.target.value);
                                // Clear verification result when token changes
                                setVerificationResult(null);
                            }}
                            required={mode === 'add'}
                            placeholder={mode === 'add' ? 'Your API token' : 'Leave empty to keep current token'}
                            helperText={mode === 'edit' && 'Leave empty to keep current token'}
                        />

                        {/* Verification Result */}
                        {verificationResult && (
                            <Alert
                                severity={verificationResult.success ? 'success' : 'error'}
                                sx={{ mt: 1 }}
                                action={
                                    <IconButton
                                        aria-label="close"
                                        color="inherit"
                                        size="small"
                                        onClick={() => setVerificationResult(null)}
                                    >
                                        ×
                                    </IconButton>
                                }
                            >
                                <Box>
                                    <Typography variant="body2" fontWeight="bold">
                                        {verificationResult.message}
                                    </Typography>
                                    {verificationResult.details && (
                                        <Typography variant="caption" display="block">
                                            {verificationResult.details}
                                        </Typography>
                                    )}
                                    {verificationResult.responseTime && (
                                        <Typography variant="caption" display="block">
                                            Response time: {verificationResult.responseTime}ms
                                            {verificationResult.modelsCount && ` • ${verificationResult.modelsCount} models available`}
                                        </Typography>
                                    )}
                                </Box>
                            </Alert>
                        )}

                        {/* Enabled Toggle (Edit mode only) */}
                        {mode === 'edit' && (
                            <FormControlLabel
                                control={
                                    <Switch
                                        size="small"
                                        checked={data.enabled || false}
                                        onChange={(e) => onChange('enabled', e.target.checked)}
                                    />
                                }
                                label="Enabled"
                            />
                        )}
                    </Stack>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={onClose}>Cancel</Button>
                    <Button
                        variant="outlined"
                        onClick={handleVerify}
                        disabled={verifying}
                        size="small"
                        startIcon={
                            verifying ? (
                                <CircularProgress size={16} />
                            ) : verificationResult?.success ? (
                                <CheckCircle color="success" />
                            ) : verificationResult?.success === false ? (
                                <Error color="error" />
                            ) : (
                                <Refresh />
                            )
                        }
                    >
                        {verifying ? 'Verifying...' : verificationResult?.success ? 'Verified' : verificationResult?.success === false ? 'Verify' : 'Verify'}
                    </Button>
                    <Button type="submit" variant="contained" size="small">
                        {submitText || defaultSubmitText}
                    </Button>

                </DialogActions>
            </form>
        </Dialog>
    );
};

export default PresetProviderFormDialog;