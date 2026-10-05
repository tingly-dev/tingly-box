import { Info, Visibility, VisibilityOff, VpnKey } from '@/components/icons';
import {
    Alert,
    Box,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    InputAdornment,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import {type Provider } from '../types/provider';
import ProviderExportButton from '@/components/ProviderExportButton';
import { CopyIconButton } from '@/components/CopyIconButton';
import { fontMono } from '@/theme/fonts';

interface OAuthEditFormData {
    name: string;
    apiBase: string;
    apiStyle: string; // 'openai' | 'anthropic' | 'google'
    // Dual providers (e.g. ZCode) expose one URL per protocol instead of a single apiBase.
    apiBaseOpenAI?: string;
    apiBaseAnthropic?: string;
    enabled: boolean;
    proxyUrl?: string;
}

interface OAuthDetailDialogProps {
    open: boolean;
    provider: Provider | null;
    onClose: () => void;
    onSubmit: (data: OAuthEditFormData) => Promise<void>;
    onNotification?: (message: string, severity: 'success' | 'error') => void;
}

const OAuthDetailDialog = ({ open, provider, onClose, onSubmit, onNotification }: OAuthDetailDialogProps) => {
    const [formData, setFormData] = useState<OAuthEditFormData>({
        name: provider?.name || '',
        apiBase: provider?.api_base || '',
        apiStyle: provider?.api_style || 'openai',
        apiBaseOpenAI: provider?.api_base_openai || '',
        apiBaseAnthropic: provider?.api_base_anthropic || '',
        enabled: provider?.enabled || false,
        proxyUrl: provider?.proxy_url || '',
    });
    const [submitting, setSubmitting] = useState(false);
    const [submitError, setSubmitError] = useState<string | null>(null);
    const [visibleTokens, setVisibleTokens] = useState<Record<string, boolean>>({});

    // Update form data when provider changes
    useEffect(() => {
        if (provider) {
            setFormData({
                name: provider.name,
                apiBase: provider.api_base,
                apiStyle: provider.api_style || 'openai',
                apiBaseOpenAI: provider.api_base_openai || '',
                apiBaseAnthropic: provider.api_base_anthropic || '',
                enabled: provider.enabled,
                proxyUrl: provider.proxy_url || '',
            });
        }
    }, [provider?.name, provider?.api_base, provider?.api_style, provider?.api_base_openai, provider?.api_base_anthropic, provider?.enabled, provider?.proxy_url]);

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

    const toggleTokenVisibility = (tokenKey: string) => {
        setVisibleTokens(prev => ({ ...prev, [tokenKey]: !prev[tokenKey] }));
    };

    const maskToken = (token: string) => {
        if (token.length <= 12) return token;
        return `${token.substring(0, 12)}...`;
    };

    const isTokenVisible = (tokenKey: string) => visibleTokens[tokenKey] || false;

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

    const isDual = !!(provider.api_base_openai && provider.api_base_anthropic);

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="sm"
            fullWidth
            slotProps={{
                paper: { sx: { maxHeight: '88vh', display: 'flex', flexDirection: 'column' } },
            }}
        >
            <DialogTitle sx={{ flexShrink: 0 }}>Edit OAuth Provider</DialogTitle>
            <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', flex: 1, overflow: 'hidden' }}>
                <DialogContent sx={{ pb: 1, overflowY: 'auto', flex: 1 }}>
                    <Stack spacing={2.5}>
                        {/* OAuth Badge */}
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <Chip
                                icon={<VpnKey fontSize="small" />}
                                label="OAuth"
                                color="primary"
                                size="small"
                            />
                            <Typography variant="caption" sx={{
                                color: "text.secondary"
                            }}>
                                OAuth credentials are managed automatically
                            </Typography>
                        </Box>

                        {/* API Style Selection — a dual provider has no single style */}
                        {!isDual && (
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
                                slotProps={{
                                    select: { native: true }
                                }}
                            >
                                <option value="openai">OpenAI Compatible</option>
                                <option value="anthropic">Anthropic Compatible</option>
                            </TextField>
                        )}

                        {/* Editable Fields */}
                        <TextField
                            size="small"
                            fullWidth
                            label="Custom Name"
                            value={formData.name}
                            onChange={(e) => setFormData(prev => ({ ...prev, name: e.target.value }))}
                            required
                            placeholder="e.g., claude-personal"
                        />

                        {isDual ? (
                            <>
                                <TextField
                                    size="small"
                                    fullWidth
                                    label="OpenAI API Base URL"
                                    value={formData.apiBaseOpenAI || ''}
                                    onChange={(e) => setFormData(prev => ({ ...prev, apiBaseOpenAI: e.target.value }))}
                                    required
                                    placeholder="https://api.example.com/openai/v1"
                                    helperText="Used for OpenAI-compatible clients"
                                />
                                <TextField
                                    size="small"
                                    fullWidth
                                    label="Anthropic API Base URL"
                                    value={formData.apiBaseAnthropic || ''}
                                    onChange={(e) => setFormData(prev => ({ ...prev, apiBaseAnthropic: e.target.value }))}
                                    required
                                    placeholder="https://api.example.com/anthropic"
                                    helperText="Used for Anthropic-compatible clients"
                                />
                            </>
                        ) : (
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
                        )}

                        <TextField
                            size="small"
                            fullWidth
                            label="Proxy URL"
                            value={formData.proxyUrl || ''}
                            onChange={(e) => setFormData(prev => ({ ...prev, proxyUrl: e.target.value }))}
                            placeholder="http://proxy.example.com:8080"
                            helperText="Optional: HTTP/HTTPS proxy URL for requests"
                        />

                        {/* Read-only OAuth Credentials */}
                        <Alert severity="info" icon={<Info fontSize="small" />}>
                            <Typography variant="caption" sx={{
                                display: "block"
                            }}>
                                <strong>OAuth Credentials (Read-only)</strong>
                            </Typography>
                        </Alert>

                        <Stack spacing={1.5}>
                            <TextField
                                size="small"
                                fullWidth
                                label="Provider Type"
                                value={provider.oauth_detail?.issuer || 'N/A'}
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
                                        ? (isTokenVisible('access_token')
                                            ? provider.oauth_detail.access_token
                                            : maskToken(provider.oauth_detail.access_token))
                                        : 'N/A'
                                }
                                disabled
                                slotProps={{
                                    input: {
                                        endAdornment: provider.oauth_detail?.access_token && (
                                            <InputAdornment position="end">
                                                <IconButton
                                                    edge="end"
                                                    size="small"
                                                    onClick={() => toggleTokenVisibility('access_token')}
                                                    title={isTokenVisible('access_token') ? 'Hide' : 'Show'}
                                                >
                                                    {isTokenVisible('access_token') ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}
                                                </IconButton>
                                                <CopyIconButton
                                                    edge="end"
                                                    value={provider.oauth_detail!.access_token}
                                                />
                                            </InputAdornment>
                                        ),
                                    },
                                }}
                            />

                            {provider.oauth_detail?.refresh_token && (
                                <TextField
                                    size="small"
                                    fullWidth
                                    label="Refresh Token"
                                    value={
                                        isTokenVisible('refresh_token')
                                            ? provider.oauth_detail.refresh_token
                                            : maskToken(provider.oauth_detail.refresh_token)
                                    }
                                    disabled
                                    slotProps={{
                                        input: {
                                            endAdornment: (
                                                <InputAdornment position="end">
                                                    <IconButton
                                                        edge="end"
                                                        size="small"
                                                        onClick={() => toggleTokenVisibility('refresh_token')}
                                                        title={isTokenVisible('refresh_token') ? 'Hide' : 'Show'}
                                                    >
                                                        {isTokenVisible('refresh_token') ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}
                                                    </IconButton>
                                                    <CopyIconButton
                                                        edge="end"
                                                        value={provider.oauth_detail?.refresh_token ?? ''}
                                                    />
                                                </InputAdornment>
                                            ),
                                        },
                                    }}
                                />
                            )}

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

                        {/* Extra Fields */}
                        {provider.oauth_detail && (
                            <Stack spacing={1.5}>
                                <Alert severity="info" icon={<Info fontSize="small" />}>
                                    <Typography variant="caption" sx={{
                                        display: "block"
                                    }}>
                                        <strong>Extra Fields</strong>
                                    </Typography>
                                </Alert>
                                <TextField
                                    size="small"
                                    fullWidth
                                    multiline
                                    rows={4}
                                    label="OAuth Extra Data"
                                    value={JSON.stringify(provider.oauth_detail, null, 2)}
                                    disabled
                                    slotProps={{
                                        input: {
                                            sx: {
                                                fontFamily: fontMono,
                                                '&.Mui-disabled': {
                                                    color: 'text.primary',
                                                },
                                            },
                                        },
                                    }}
                                />
                            </Stack>
                        )}

                        {/* Submit Error */}
                        {submitError && (
                            <Alert severity="error" onClose={() => setSubmitError(null)}>
                                {submitError}
                            </Alert>
                        )}
                    </Stack>
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2, gap: 1, flexShrink: 0 }}>
                    <ProviderExportButton providerUuid={provider.uuid} onNotification={onNotification}/>
                    <Box sx={{ml: 'auto'}}>
                        <Button onClick={onClose} color="inherit">Cancel</Button>
                        <Button
                            type="submit"
                            variant="contained"
                            size="small"
                            disabled={submitting}
                            sx={{ml: 1}}
                        >
                            {submitting ? 'Saving...' : 'Save Changes'}
                        </Button>
                    </Box>
                </DialogActions>
            </form>
        </Dialog>
    );
};

export default OAuthDetailDialog;
