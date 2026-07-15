import {ArrowBack, Close, Visibility, VisibilityOff} from '@/components/icons';
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Divider,
    IconButton,
    InputAdornment,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import React, {useEffect, useMemo, useState} from 'react';
import {api} from '../../services/api';
import {getServiceProvider} from '@/services/serviceProviders';
import ProviderIcon from '@/components/ProviderIcon';
import {getCloudFields, buildCloudApiBase, type CloudField} from './cloudCredentialSchema';

interface CloudProviderDialogProps {
    open: boolean;
    /** Cloud template id chosen in the picker (e.g. "aws-bedrock"). */
    presetId: string | null;
    onClose: () => void;
    onSuccess: () => void;
    /** Re-open the picker (bottom-left Back button), mirrors ProviderFormDialog. */
    onBack?: () => void;
    onNotification?: (message: string, severity: 'success' | 'error') => void;
}

/**
 * Dedicated add-flow dialog for cloud-credential providers (AWS Bedrock, GCP
 * Vertex, Azure OpenAI). These authenticate with multi-field credentials rather
 * than a single bearer token, so they get their own form instead of being
 * crammed into the protocol-slot ProviderFormDialog — the same separation OAuth
 * providers already have with OAuthDialog.
 *
 * The card identity (name, icon, api_style, models) is data-driven from the
 * backend provider template; only the credential field schema is code, keyed by
 * the template's auth_type (see cloudCredentialSchema).
 */
const CloudProviderDialog: React.FC<CloudProviderDialogProps> = ({
    open, presetId, onClose, onSuccess, onBack, onNotification,
}) => {
    const template = presetId ? getServiceProvider(presetId) : null;
    const authType = template?.auth_type || '';
    const fields = useMemo(() => getCloudFields(authType), [authType]);

    const [name, setName] = useState('');
    const [values, setValues] = useState<Record<string, string>>({});
    const [reveal, setReveal] = useState<Record<string, boolean>>({});
    const [proxyUrl, setProxyUrl] = useState('');
    const [submitting, setSubmitting] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [advancedOpen, setAdvancedOpen] = useState(false);

    const displayName = template?.alias || template?.name || '';

    // Reset form whenever a new template is opened.
    useEffect(() => {
        if (open && template) {
            setName(template.alias || template.name || '');
            setValues({});
            setReveal({});
            setProxyUrl('');
            setError(null);
            setAdvancedOpen(false);
        }
    }, [open, presetId]); // eslint-disable-line react-hooks/exhaustive-deps

    const hasAdvanced = useMemo(() => fields.some((f) => f.advanced), [fields]);

    if (!template || !authType) return null;

    const setValue = (key: string, v: string) => {
        setValues((prev) => ({...prev, [key]: v}));
        setError(null);
    };

    const missingRequired = fields
        .filter((f) => f.required && !(values[f.key] || '').trim())
        .map((f) => f.label);
    const canSubmit = name.trim().length > 0 && missingRequired.length === 0 && !submitting;

    const handleSubmit = async () => {
        if (!name.trim()) {
            setError('Provider name is required');
            return;
        }
        if (missingRequired.length > 0) {
            setError(`Missing required field(s): ${missingRequired.join(', ')}`);
            return;
        }

        // Only send non-empty credential fields.
        const credential: Record<string, string> = {};
        fields.forEach((f) => {
            const v = (values[f.key] || '').trim();
            if (v) credential[f.key] = v;
        });

        setSubmitting(true);
        setError(null);
        try {
            const result = await api.addProvider({
                name: name.trim(),
                api_base: buildCloudApiBase(authType, values),
                api_style: template.api_style,
                auth_type: authType,
                credential,
                proxy_url: proxyUrl.trim() || undefined,
                enabled: true,
            });
            if (result?.success) {
                onNotification?.('Provider connected successfully!', 'success');
                onSuccess();
                onClose();
            } else {
                const msg = result?.error || 'Failed to connect provider';
                setError(msg);
                onNotification?.(`Failed to connect provider: ${msg}`, 'error');
            }
        } catch (e: any) {
            setError(e?.message || 'Failed to connect provider');
        } finally {
            setSubmitting(false);
        }
    };

    const renderField = (f: CloudField) => {
        const isSecret = f.type === 'password';
        const shown = reveal[f.key];
        return (
            <TextField
                key={f.key}
                size="small"
                fullWidth
                required={f.required}
                label={f.label}
                placeholder={f.placeholder}
                helperText={f.helper}
                value={values[f.key] || ''}
                onChange={(e) => setValue(f.key, e.target.value)}
                multiline={f.type === 'multiline'}
                minRows={f.type === 'multiline' ? 4 : undefined}
                type={isSecret && !shown ? 'password' : 'text'}
                InputProps={isSecret ? {
                    endAdornment: (
                        <InputAdornment position="end">
                            <IconButton
                                size="small"
                                onClick={() => setReveal((prev) => ({...prev, [f.key]: !prev[f.key]}))}
                                edge="end"
                            >
                                {shown ? <VisibilityOff fontSize="small"/> : <Visibility fontSize="small"/>}
                            </IconButton>
                        </InputAdornment>
                    ),
                } : undefined}
            />
        );
    };

    const primaryFields = fields.filter((f) => !f.advanced);
    const advancedFields = fields.filter((f) => f.advanced);

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth
            PaperProps={{sx: {maxHeight: '88vh', display: 'flex', flexDirection: 'column'}}}>
            <DialogTitle sx={{flexShrink: 0}}>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 1.25}}>
                    <Box sx={{width: 28, height: 28, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0}}>
                        <ProviderIcon identifier={template.icon || template.id} size={24}/>
                    </Box>
                    <Box sx={{flex: 1, minWidth: 0}}>
                        <Typography variant="h6" sx={{lineHeight: 1.2}}>{displayName}</Typography>
                        {template.description && (
                            <Typography variant="caption" color="text.secondary">{template.description}</Typography>
                        )}
                    </Box>
                    <IconButton aria-label="close" onClick={onClose} size="small"><Close/></IconButton>
                </Box>
            </DialogTitle>
            <DialogContent dividers sx={{pt: 2, overflowY: 'auto', flex: 1}}>
                <Stack spacing={2.5}>
                    {error && <Alert severity="error" onClose={() => setError(null)}>{error}</Alert>}

                    <TextField
                        size="small" fullWidth required
                        label="Name"
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                    />

                    {primaryFields.map(renderField)}

                    {hasAdvanced && (
                        <Box>
                            <Button size="small" variant="text" onClick={() => setAdvancedOpen((v) => !v)} sx={{px: 0}}>
                                {advancedOpen ? 'Hide advanced' : 'Advanced (optional)'}
                            </Button>
                            {advancedOpen && (
                                <Stack spacing={2.5} sx={{mt: 1.5}}>
                                    <Divider/>
                                    {advancedFields.map(renderField)}
                                    <TextField
                                        size="small" fullWidth
                                        label="Proxy URL"
                                        placeholder="http://localhost:7890"
                                        value={proxyUrl}
                                        onChange={(e) => setProxyUrl(e.target.value)}
                                    />
                                </Stack>
                            )}
                        </Box>
                    )}
                </Stack>
            </DialogContent>
            <DialogActions sx={{px: 3, pb: 2}}>
                {onBack && (
                    <Button
                        type="button" variant="text" size="small"
                        startIcon={<ArrowBack fontSize="small"/>}
                        onClick={() => { onClose(); onBack(); }}
                    >
                        Back
                    </Button>
                )}
                <Box sx={{ml: 'auto'}}>
                    <Button
                        variant="contained" size="small"
                        disabled={!canSubmit}
                        onClick={handleSubmit}
                    >
                        {submitting ? <CircularProgress size={20} thickness={4}/> : 'Connect'}
                    </Button>
                </Box>
            </DialogActions>
        </Dialog>
    );
};

export default CloudProviderDialog;
