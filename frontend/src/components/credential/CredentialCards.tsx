import { ApiStyleBadge } from '@/components/ApiStyleBadge';
import { ProviderLabel } from '@/components/ProviderLogo';
import { Edit, ListAlt, MoreVert, Route } from '@/components/icons';
import { QuotaCell } from '@/components/credential/QuotaCell';
import type { RedeemOutcome } from '@/hooks/useProviderQuota';
import type { Provider } from '@/types/provider';
import type { ProviderQuota } from '@/types/quota';
import { Box, Button, Divider, IconButton, Paper, Stack, Switch, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { MouseEvent } from 'react';

interface Props {
    providers: Provider[];
    onEdit?: (uuid: string) => void;
    onToggle?: (uuid: string) => void;
    onModels: (uuid: string) => void;
    onMore: (event: MouseEvent<HTMLElement>, uuid: string) => void;
    providerQuotas?: Record<string, ProviderQuota>;
    refreshingQuotas?: Set<string>;
    onQuotaRefresh?: (uuid: string) => void;
    onQuotaRedeem?: (uuid: string, rewardId: string) => Promise<RedeemOutcome>;
}

export function CredentialCards({ providers, onEdit, onToggle, onModels, onMore, providerQuotas, refreshingQuotas, onQuotaRefresh, onQuotaRedeem }: Props) {
    const { t } = useTranslation();
    return (
        <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 300px), 1fr))', gap: 2 }}>
            {providers.map(provider => {
                const oauth = provider.auth_type === 'oauth';
                const expiry = provider.oauth_detail?.expires_at;
                const expired = expiry && new Date(expiry).getTime() < Date.now();
                return (
                    <Paper key={provider.uuid} variant="outlined" sx={{ p: 2, borderRadius: 2, minWidth: 0, display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                        <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 1, minWidth: 0 }}>
                            <Box sx={{ minWidth: 0, overflow: 'hidden' }}><ProviderLabel provider={provider}><Typography variant="subtitle2" noWrap title={provider.name}>{provider.name}</Typography></ProviderLabel></Box>
                            <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', flexShrink: 0 }}>
                                {provider.proxy_url && <Tooltip title={provider.proxy_url} arrow><Box component="span" aria-label={`Proxy: ${provider.proxy_url}`} sx={{ display: 'flex', color: 'text.secondary' }}><Route fontSize="small" /></Box></Tooltip>}
                                <Switch checked={provider.enabled} onChange={() => onToggle?.(provider.uuid)} size="small" color="success" slotProps={{ input: { 'aria-label': `${t('credentialPage.status')}: ${provider.name}` } }} />
                            </Stack>
                        </Stack>
                        <Stack direction="row" spacing={1} sx={{ alignItems: 'center', minHeight: 24 }}>
                            {provider.api_base_openai && provider.api_base_anthropic ? <><ApiStyleBadge apiStyle="openai" minimal minimalSize="medium"/><ApiStyleBadge apiStyle="anthropic" minimal minimalSize="medium"/></> : <ApiStyleBadge apiStyle={provider.api_style} minimal minimalSize="medium"/>}
                            <Typography variant="caption" color="text.secondary">{oauth ? (provider.oauth_detail?.issuer || 'OAuth') : 'API Key'}</Typography>
                        </Stack>
                        <Box sx={{ flex: 1, minWidth: 0 }}>
                            <Typography variant="caption" color="text.secondary">{oauth ? t('credentialPage.expiresAt') : t('credentialPage.baseUrl')}</Typography>
                            <Typography variant="body2" noWrap title={oauth ? (expiry || t('credentialPage.never')) : provider.api_base} color={expired ? 'error.main' : 'text.primary'}>{oauth ? (expiry ? (expired ? t('credentialPage.expired') : new Date(expiry).toLocaleString()) : t('credentialPage.never')) : (provider.api_base || '—')}</Typography>
                        </Box>
                        {onQuotaRefresh && <Box><QuotaCell quota={providerQuotas?.[provider.uuid]} refreshing={refreshingQuotas?.has(provider.uuid) ?? false} onRefresh={() => onQuotaRefresh(provider.uuid)} onRedeem={onQuotaRedeem && (id => onQuotaRedeem(provider.uuid, id))}/></Box>}
                        <Divider />
                        <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between' }}>
                            <Button size="small" startIcon={<ListAlt />} onClick={() => onModels(provider.uuid)}>Models</Button>
                            <Box>{onEdit && <Tooltip title="Edit"><IconButton aria-label={`Edit ${provider.name}`} size="small" onClick={() => onEdit(provider.uuid)}><Edit fontSize="small" /></IconButton></Tooltip>}<IconButton aria-label={`More actions for ${provider.name}`} size="small" onClick={e => onMore(e, provider.uuid)}><MoreVert fontSize="small" /></IconButton></Box>
                        </Stack>
                    </Paper>
                );
            })}
        </Box>
    );
}
