import {ExpandMore, Visibility, VisibilityOff} from '@/components/icons';
import {
    Accordion, AccordionDetails, AccordionSummary, Alert, Box, Chip, IconButton,
    InputAdornment, Stack, TextField, Typography,
} from '@mui/material';
import {useEffect, useRef, useState, type FormEvent} from 'react';
import {useTranslation} from 'react-i18next';
import type {Provider} from '@/types/provider';
import ProviderExportButton from '@/components/ProviderExportButton';
import {CopyIconButton} from '@/components/CopyIconButton';
import {api} from '@/services/api';
import ProviderFormLayout, {ProviderIdentityFields, ProviderFormSection} from './provider-form-dialog/ProviderFormLayout';
import ProtocolSlot from './provider-form-dialog/ProtocolSlot';
import ProxyUrlField from './provider-form-dialog/ProxyUrlField';
import {isHttpUrl} from './provider-form-dialog/validation';
import {fontMono} from '@/theme/fonts';

interface OAuthEditFormData {
    name: string;
    apiBase: string;
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

export default function OAuthDetailDialog({open, provider, onClose, onSubmit, onNotification}: OAuthDetailDialogProps) {
    const {t, i18n} = useTranslation();
    const formRef = useRef<HTMLFormElement>(null);
    const [formData, setFormData] = useState<OAuthEditFormData>({name: '', apiBase: '', enabled: true});
    const [submitting, setSubmitting] = useState(false);
    const [submitError, setSubmitError] = useState<string | null>(null);
    const [validationAttempted, setValidationAttempted] = useState(false);
    const [visibleTokens, setVisibleTokens] = useState<Record<string, boolean>>({});
    const [detailsOpen, setDetailsOpen] = useState(false);
    const [globalProxyUrl, setGlobalProxyUrl] = useState('');

    useEffect(() => {
        api.getConfig().then(result => setGlobalProxyUrl(result?.data?.http_transport?.global_proxy_url ?? '')).catch(() => {
            // Quick proxy lookup is optional.
        });
    }, []);
    useEffect(() => {
        if (!open || !provider) return;
        setFormData({
            name: provider.name, apiBase: provider.api_base,
            apiBaseOpenAI: provider.api_base_openai || '', apiBaseAnthropic: provider.api_base_anthropic || '',
            enabled: provider.enabled, proxyUrl: provider.proxy_url || '',
        });
        setVisibleTokens({});
        setSubmitError(null);
        setValidationAttempted(false);
        setDetailsOpen(false);
        // Snapshot at the start of each edit session; background refreshes must not erase the draft.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open, provider?.uuid]);

    if (!provider) return null;
    const apiStyle = provider.api_style || 'openai';
    const isDual = !!(provider.api_base_openai && provider.api_base_anthropic);
    const autoName = (() => {
        try {return new URL(isDual ? formData.apiBaseOpenAI || '' : formData.apiBase).hostname || provider.name;}
        catch {return provider.name;}
    })();
    const detail = provider.oauth_detail;
    const expiry = detail?.expires_at ? new Date(detail.expires_at) : null;
    const validExpiry = expiry && !Number.isNaN(expiry.getTime());
    const expired = validExpiry && expiry.getTime() < Date.now();
    const extraFields = detail?.extra_fields;

    const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (submitting) return;
        setValidationAttempted(true);
        setSubmitError(null);
        const urls = isDual ? [formData.apiBaseOpenAI || '', formData.apiBaseAnthropic || ''] : [formData.apiBase];
        if (!urls.every(isHttpUrl)) {
            setTimeout(() => formRef.current?.querySelector<HTMLInputElement>('[aria-invalid="true"]')?.focus(), 0);
            return;
        }
        setSubmitting(true);
        try {
            await onSubmit({
                ...formData, name: formData.name.trim() || autoName,
                apiBase: isDual ? (apiStyle === 'anthropic' ? formData.apiBaseAnthropic! : formData.apiBaseOpenAI!).trim() : formData.apiBase.trim(),
                apiBaseOpenAI: formData.apiBaseOpenAI?.trim(), apiBaseAnthropic: formData.apiBaseAnthropic?.trim(),
                proxyUrl: formData.proxyUrl?.trim(),
            });
            onClose();
        } catch (error) {
            setSubmitError(error instanceof Error ? error.message : t('providerDialog.editUx.saveFailed'));
        } finally {setSubmitting(false);}
    };
    const tokenField = (key: 'access_token' | 'refresh_token', label: string) => {
        const token = detail?.[key] || '';
        const visible = !!visibleTokens[key];
        return <TextField size="small" fullWidth label={label} type={visible ? 'text' : 'password'} value={token}
            slotProps={{htmlInput: {autoComplete: 'off'}, input: {readOnly: true, sx: {fontFamily: fontMono}, endAdornment: token && (
                <InputAdornment position="end">
                    <IconButton size="small" aria-label={t(visible ? 'providerDialog.editUx.hideKey' : 'providerDialog.editUx.showKey')}
                        onClick={() => setVisibleTokens(previous => ({...previous, [key]: !previous[key]}))}>
                        {visible ? <VisibilityOff fontSize="small"/> : <Visibility fontSize="small"/>}
                    </IconButton><CopyIconButton edge="end" value={token}/>
                </InputAdornment>
            )}}}/>;
    };
    const protocolSlot = (kind: 'openai' | 'anthropic' | 'google') => {
        const enabled = isDual || apiStyle === kind;
        const field = isDual ? kind === 'openai' ? 'apiBaseOpenAI' : 'apiBaseAnthropic' : 'apiBase';
        const url = formData[field] || '';
        return <ProtocolSlot key={kind} kind={kind} slot={{enabled, url}}
            selectionMode="fixed"
            onToggle={() => {}}
            onUrlChange={value => setFormData(previous => ({...previous, [field]: value}))} onUrlBlur={() => {}}
            urlError={validationAttempted && enabled && !isHttpUrl(url)}/>;
    };

    return <ProviderFormLayout open={open} description={t('providerDialog.editUx.description')}
        busy={submitting} saving={submitting} onClose={onClose} onSubmit={handleSubmit} formRef={formRef}
        exportAction={<ProviderExportButton providerUuid={provider.uuid} onNotification={onNotification}/>}>
        <ProviderIdentityFields name={formData.name} autoName={autoName} onNameChange={name => setFormData(previous => ({...previous, name}))}
            enabled={formData.enabled} onEnabledChange={enabled => setFormData(previous => ({...previous, enabled}))}/>
        <ProviderFormSection title={t('providerDialog.editUx.authentication')}>
            <Accordion expanded={detailsOpen} onChange={(_, expanded) => setDetailsOpen(expanded)} disableGutters elevation={0}
                sx={{border: 1, borderColor: 'divider', borderRadius: '8px !important', '&::before': {display: 'none'}}}>
                <AccordionSummary expandIcon={<ExpandMore fontSize="small"/>}>
                    <Box sx={{flex: 1, minWidth: 0}}>
                        <Typography variant="body2" sx={{fontWeight: 600}}>OAuth{detail?.issuer ? ` · ${detail.issuer}` : ''}</Typography>
                        <Typography variant="caption" color="text.secondary">{t('providerDialog.editUx.oauthHint')}</Typography>
                    </Box>
                    {expired && <Chip label={t('providerDialog.editUx.expired')} color="error" size="small" sx={{mr: 1}}/>}
                </AccordionSummary>
                <AccordionDetails><Stack spacing={2}>
                    <TextField size="small" fullWidth label={t('providerDialog.editUx.providerType')} value={detail?.issuer || '—'} slotProps={{input: {readOnly: true}}}/>
                    <TextField size="small" fullWidth label={t('providerDialog.editUx.userId')} value={detail?.user_id || '—'} slotProps={{input: {readOnly: true}}}/>
                    {tokenField('access_token', t('providerDialog.editUx.accessToken'))}
                    {detail?.refresh_token && tokenField('refresh_token', t('providerDialog.editUx.refreshToken'))}
                    <TextField size="small" fullWidth label={t('providerDialog.editUx.expiresAt')} value={validExpiry ? expiry.toLocaleString(i18n.language) : '—'}
                        error={!!expired} slotProps={{input: {readOnly: true}}}/>
                    {extraFields && Object.keys(extraFields).length > 0 && <TextField size="small" fullWidth multiline minRows={3}
                        label={t('providerDialog.editUx.extraData')} value={JSON.stringify(extraFields, null, 2)} slotProps={{input: {readOnly: true, sx: {fontFamily: fontMono}}}}/>}
                </Stack></AccordionDetails>
            </Accordion>
        </ProviderFormSection>
        <ProviderFormSection title={t('providerDialog.protocol.label')}>
            <Typography variant="caption" color="text.secondary">{isDual ? t('credentialPage.oauthProtocolDual') : t('credentialPage.oauthProtocolSingle', {
                style: apiStyle === 'google' ? 'Google' : apiStyle === 'anthropic' ? 'Anthropic' : 'OpenAI',
            })}</Typography>
            <Typography variant="caption" color="text.secondary">{t('providerDialog.editUx.oauthStyleLocked')}</Typography>
            <Stack spacing={2}>
                {isDual ? <>{protocolSlot('openai')}{protocolSlot('anthropic')}</> : protocolSlot(apiStyle === 'google' ? 'google' : apiStyle === 'anthropic' ? 'anthropic' : 'openai')}
            </Stack>
        </ProviderFormSection>
        <ProviderFormSection title={t('providerDialog.editUx.proxySettings')}>
            <ProxyUrlField mode="edit" proxyUrl={formData.proxyUrl || ''} onProxyUrlChange={proxyUrl => setFormData(previous => ({...previous, proxyUrl}))}
                globalProxyUrl={globalProxyUrl} useGlobalProxy={false} onUseGlobalProxyChange={() => {}}/>
        </ProviderFormSection>
        {submitError && <Alert severity="error" onClose={() => setSubmitError(null)}>{submitError}</Alert>}
    </ProviderFormLayout>;
}
