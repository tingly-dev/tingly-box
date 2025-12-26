import { Info, VpnKey } from '@mui/icons-material';
import {
    Alert,
    Box,
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
import { useState } from 'react';
import { OpenAI } from '@lobehub/icons';
import { Anthropic } from '@lobehub/icons';
import { Provider } from '../types/provider';

interface OAuthEditFormData {
    name: string;
    apiBase: string;
    apiStyle: 'openai' | 'anthropic';
    enabled: boolean;
}

interface OAuthDetailDialogProps {
    open: boolean;
    provider: Provider | null;
    onClose: () => void;
    onSubmit: (data: OAuthEditFormData) => Promise<void>;
}

const OAuthDetailDialog = ({ open, provider, onClose, onSubmit }: OAuthDetailDialogProps) => {
    const [formData, setFormData] = useState<OAuthEditFormData>({
        name: provider?.name || '',
        apiBase: provider?.api_base || '',
        apiStyle: provider?.api_style || 'openai',
        enabled: provider?.enabled || false,
    });
    const [submitting, setSubmitting] = useState(false);
    const [submitError, setSubmitError] = useState<string | null>(null);

    // Update form data when provider changes
    if (provider && (formData.name !== provider.name || formData.apiBase !== provider.api_base)) {
        setFormData({
            name: provider.name,
            apiBase: provider.api_base,
            apiStyle: provider.api_style || 'openai',
            enabled: provider.enabled,
        });
    }

    const formatDate = (dateStr?: string) => {
        if (!dateStr) return 'N/A';
        try {
            const date = new Date(dateStr);
            return date.toLocaleString();
        } catch {
            return dateStr;
        }
    };

    const isExpired = provider?.oauth_detail?.expires_at
        ? new Date(provider.oauth_detail.expires_at) < new Date()
        : false;

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        setSubmitting(true);
        setSubmitError(null);

        try {
            await onSubmit(formData);
            onClose();
        } catch (error) {
            setSubmitError(error instanceof Error ? error.message : 'Failed to update provider');
        } finally {
            setSubmitting(false);
        }
    };

    if (!provider) return null;

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>Edit OAuth Provider</DialogTitle>
            <form onSubmit={handleSubmit}>
                <DialogContent sx={{ pb: 1 }}>
                    <Stack spacing={2.5}>
                        {/* OAuth Badge */}
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <Chip
                                icon={<VpnKey fontSize="small" />}
                                label="OAuth"
                                color="primary"
                                size="small"
                            />
                            <Typography variant="caption" color="text.secondary">
                                OAuth credentials are managed automatically
                            </Typography>
                        </Box>

                        {/* API Style Selection */}
                        <TextField
                            select
                            fullWidth
                            size="small"
                            label="API Style"
                            value={formData.apiStyle}
                            onChange={(e) => setFormData(prev => ({
                                ...prev,
                                apiStyle: e.target.value as 'openai' | 'anthropic',
                            }))}
                            SelectProps={{ native: true }}
                        >
                            <option value="openai">OpenAI Compatible</option>
                            <option value="anthropic">Anthropic Compatible</option>
                        </TextField>

                        {/* Editable Fields */}
                        <TextField
                            size="small"
                            fullWidth
                            label="Provider Name"
                            value={formData.name}
                            onChange={(e) => setFormData(prev => ({ ...prev, name: e.target.value }))}
                            required
                            placeholder="e.g., claude-personal"
                        />

                        <TextField
                            size="small"
                            fullWidth
                            label="API Base URL"
                            value={formData.apiBase}
                            onChange={(e) => setFormData(prev => ({ ...prev, apiBase: e.target.value }))}
                            required
                            placeholder={
                                formData.apiStyle === 'openai'
                                    ? "https://api.openai.com/v1"
                                    : "https://api.anthropic.com"
                            }
                        />

                        {/* Read-only OAuth Credentials */}
                        <Alert severity="info" icon={<Info fontSize="small" />}>
                            <Typography variant="caption" display="block">
                                <strong>OAuth Credentials (Read-only)</strong>
                            </Typography>
                        </Alert>

                        <Stack spacing={1.5}>
                            <TextField
                                size="small"
                                fullWidth
                                label="Provider Type"
                                value={provider.oauth_detail?.provider_type || 'N/A'}
                                disabled
                            />

                            <TextField
                                size="small"
                                fullWidth
                                label="User ID"
                                value={provider.oauth_detail?.user_id || 'N/A'}
                                disabled
                            />

                            <TextField
                                size="small"
                                fullWidth
                                label="Access Token"
                                value={
                                    provider.oauth_detail?.access_token
                                        ? `${provider.oauth_detail.access_token.substring(0, 12)}...`
                                        : 'N/A'
                                }
                                disabled
                            />

                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <TextField
                                    size="small"
                                    fullWidth
                                    label="Expires At"
                                    value={formatDate(provider.oauth_detail?.expires_at)}
                                    disabled
                                    error={isExpired}
                                    helperText={isExpired ? 'Token has expired' : ''}
                                />
                                {isExpired && (
                                    <Chip label="Expired" color="error" size="small" />
                                )}
                            </Box>
                        </Stack>

                        {/* Submit Error */}
                        {submitError && (
                            <Alert severity="error" onClose={() => setSubmitError(null)}>
                                {submitError}
                            </Alert>
                        )}
                    </Stack>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={onClose}>Cancel</Button>
                    <Button
                        type="submit"
                        variant="contained"
                        size="small"
                        disabled={submitting}
                    >
                        {submitting ? 'Saving...' : 'Save Changes'}
                    </Button>
                </DialogActions>
            </form>
        </Dialog>
    );
};

export default OAuthDetailDialog;
