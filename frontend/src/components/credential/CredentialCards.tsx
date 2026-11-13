import { ApiStyleBadge } from '@/components/ApiStyleBadge';
import ProviderLogo from '@/components/ProviderLogo';
import { Edit, ListAlt, MoreVert, Route } from '@/components/icons';
import { QuotaCell } from '@/components/credential/QuotaCell';
import type { RedeemOutcome } from '@/hooks/useProviderQuota';
import type { Provider } from '@/types/provider';
import type { ProviderQuota } from '@/types/quota';
import { Box, Button, IconButton, Paper, Stack, Switch, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { MouseEvent } from 'react';
import { radius } from '@tingly/ui/theme/radius';

interface Props {
    providers: Provider[];
    layout: "list" | "card";
    onEdit?: (uuid: string) => void;
    onToggle?: (uuid: string) => void;
    onModels: (uuid: string) => void;
    onMore: (event: MouseEvent<HTMLElement>, uuid: string) => void;
    providerQuotas?: Record<string, ProviderQuota>;
    refreshingQuotas?: Set<string>;
    onQuotaRefresh?: (uuid: string) => void;
    onQuotaRedeem?: (uuid: string, rewardId: string) => Promise<RedeemOutcome>;
}

export function CredentialCards({ layout, providers, onEdit, onToggle, onModels, onMore, providerQuotas, refreshingQuotas, onQuotaRefresh, onQuotaRedeem }: Props) {
    const { t } = useTranslation();
    return (
        <Box sx={{ display: 'grid', gridTemplateColumns: layout === 'card' ? 'repeat(auto-fit, minmax(min(100%, 580px), 1fr))' : 'minmax(0, 1fr)', gap: 1.5 }}> 
            {providers.map(provider => {
                const oauth = provider.auth_type === 'oauth';
                const expiry = provider.oauth_detail?.expires_at;
                const expired = expiry && new Date(expiry).getTime() < Date.now();
                const dual = Boolean(provider.api_base_openai && provider.api_base_anthropic);
                const endpoints = dual
                    ? [{ label: 'OpenAI', url: provider.api_base_openai }, { label: 'Anthropic', url: provider.api_base_anthropic }]
                    : provider.api_base ? [{ label: t('credentialPage.baseUrl'), url: provider.api_base }] : [];
                return (
                    <Paper key={provider.uuid} variant="outlined" sx={{ px: { xs: 2, md: 2.5 }, py: 1, borderRadius: radius.card, minWidth: 0 }}>
                        <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'minmax(0, 1fr) auto auto', md: layout === 'card' ? 'minmax(155px, 1.6fr) minmax(85px, 0.8fr) minmax(85px, 1fr) 20px auto auto' : 'minmax(230px, 1.5fr) minmax(130px, 0.8fr) minmax(130px, 1fr) 24px auto auto' }, gridTemplateAreas: { xs: '"identity identity status" "endpoint endpoint endpoint" "quota quota proxy" "actions actions actions"', md: '"identity endpoint quota proxy actions status"' }, alignItems: 'center', columnGap: { xs: 2, md: layout === 'card' ? 0.75 : 3 }, rowGap: 1 }}>
                            <Box sx={{ gridArea: 'identity', minWidth: 0, display: 'flex', alignItems: 'center', gap: layout === 'card' ? 1 : 2 }}>
                                <ProviderLogo provider={provider} size={42} sx={{ flexShrink: 0 }} />
                                <Box sx={{ minWidth: 0 }}><Typography variant="subtitle1" noWrap title={provider.name} sx={{ fontWeight: 600 }}>{provider.name}</Typography>
                                <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', mt: 0.5, minWidth: 0 }}>
                                    {dual ? <><ApiStyleBadge apiStyle="openai" minimal minimalSize="medium"/><ApiStyleBadge apiStyle="anthropic" minimal minimalSize="medium"/></> : <ApiStyleBadge apiStyle={provider.api_style} minimal minimalSize="medium"/>}
                                    <Typography variant="caption" color="text.secondary" noWrap>{oauth ? (provider.oauth_detail?.issuer || 'OAuth') : 'API Key'}</Typography>
                                </Stack></Box>
                            </Box>
                            <Box sx={{ gridArea: 'endpoint', minWidth: 0, borderLeft: { md: '1px solid' }, borderLeftColor: { md: 'divider' }, pl: { md: layout === 'card' ? 1 : 3 } }}>
                                {oauth && <Box sx={{ mb: endpoints.length ? 0.25 : 0 }}><Typography variant="caption" color="text.secondary">{t('credentialPage.expiresAt')}</Typography><Typography variant="body2" noWrap color={expired ? 'error.main' : 'text.primary'} title={expiry ? new Date(expiry).toLocaleString() : t('credentialPage.never')}>{expiry ? `${expired ? `${t('credentialPage.expired')} · ` : ''}${new Date(expiry).toLocaleDateString()}` : t('credentialPage.never')}</Typography></Box>}
                                {endpoints.length > 0 && <Tooltip arrow placement="top" title={<Stack spacing={1}>{endpoints.map(({ label, url }) => <Box key={label}><Typography variant="caption" sx={{ display: 'block', fontWeight: 600, color: 'text.secondary' }}>{label}</Typography><Typography variant="caption" sx={{ display: 'block', overflowWrap: 'anywhere', color: 'text.primary' }}>{url}</Typography></Box>)}</Stack>} slotProps={{ tooltip: { sx: { maxWidth: 420, p: 1.5 } } }}><Box component="button" type="button" aria-label={`${provider.name}: ${t(dual ? 'credentialPage.twoEndpoints' : 'credentialPage.endpoint')}`} sx={{ border: 0, p: 0, bgcolor: 'transparent', color: 'primary.main', font: 'inherit', fontSize: '0.75rem', textAlign: 'left', cursor: 'help', textDecoration: 'underline', textDecorationStyle: 'dotted', textUnderlineOffset: 3 }}>{t(dual ? 'credentialPage.twoEndpoints' : 'credentialPage.endpoint')}</Box></Tooltip>}
                            </Box>
                            {onQuotaRefresh && <Box sx={{ gridArea: 'quota', minWidth: 0, borderLeft: { md: '1px solid' }, borderLeftColor: { md: 'divider' }, pl: { md: layout === 'card' ? 1 : 3 } }}><QuotaCell quota={providerQuotas?.[provider.uuid]} refreshing={refreshingQuotas?.has(provider.uuid) ?? false} onRefresh={() => onQuotaRefresh(provider.uuid)} onRedeem={onQuotaRedeem && (id => onQuotaRedeem(provider.uuid, id))}/></Box>}
                            <Box sx={{ gridArea: 'proxy', display: 'flex', alignItems: 'center', justifyContent: 'center', width: 24 }}>
                                {provider.proxy_url && <Tooltip title={provider.proxy_url} arrow><Box component="span" aria-label={`Proxy: ${provider.proxy_url}`} sx={{ display: 'flex', color: 'text.secondary' }}><Route fontSize="small" /></Box></Tooltip>}
                            </Box>
                            <Stack direction="row" spacing={0.25} sx={{ alignItems: 'center', gridArea: 'actions' }}>
                                {layout === 'card' ? <Tooltip title="Models"><IconButton size="small" aria-label={`Models for ${provider.name}`} color="inherit" onClick={() => onModels(provider.uuid)}><ListAlt fontSize="small" /></IconButton></Tooltip> : <Button size="small" color="inherit" startIcon={<ListAlt />} onClick={() => onModels(provider.uuid)}>Models</Button>}
                                {onEdit && <Tooltip title="Edit"><IconButton aria-label={`Edit ${provider.name}`} size="small" onClick={() => onEdit(provider.uuid)}><Edit fontSize="small" /></IconButton></Tooltip>}
                                <IconButton aria-label={`More actions for ${provider.name}`} size="small" onClick={e => onMore(e, provider.uuid)}><MoreVert fontSize="small" /></IconButton>
                            </Stack>
                            <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', justifyContent: 'flex-end', gridArea: 'status', borderLeft: { md: '1px solid' }, borderLeftColor: { md: 'divider' }, pl: { md: layout === 'card' ? 0.5 : 2 } }}>
                                <Switch checked={provider.enabled} onChange={() => onToggle?.(provider.uuid)} size="small" color="success" slotProps={{ input: { 'aria-label': `${t('credentialPage.status')}: ${provider.name}` } }} />
                            </Stack>
                        </Box>
                    </Paper>
                );
            })}
        </Box>
    );
}
