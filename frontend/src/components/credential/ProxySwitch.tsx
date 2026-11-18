import { CircularProgress, IconButton, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Route, RouteOff } from '@/components/icons';
import { proxyState, redactProxyUrl } from './proxyState';

interface Props {
    provider: { uuid: string; name: string; proxy_url?: string | null; proxy_disabled?: boolean | null };
    pending?: boolean;
    onToggle?: (uuid: string) => void;
    /** With no proxy stored there is nothing to switch; the control leads to the editor instead. */
    onConfigure?: (uuid: string) => void;
}

/**
 * Per-credential proxy control. State is carried by the icon shape and the
 * tooltip as well as colour: route = in use, route-off = stored but off.
 * With no proxy stored there is nothing to show, so the control stays hidden
 * until its row (`.proxy-row`) is hovered or focused and then leads to the
 * editor; touch screens have no hover, so it stays reachable there. Touch
 * target is 40px on phones.
 */
export function ProxySwitch({ provider, pending = false, onToggle, onConfigure }: Props) {
    const { t } = useTranslation();
    const state = proxyState(provider);
    const url = provider.proxy_url ? redactProxyUrl(provider.proxy_url) : '';
    const size = { width: { xs: 40, md: 28 }, height: { xs: 40, md: 28 } };

    if (state === 'none') {
        if (!onConfigure) return null;
        const label = t('credentialPage.proxyNoneAction', { name: provider.name });
        return (
            <Tooltip title={t('credentialPage.proxyNoneHint')} arrow>
                <IconButton
                    size="small"
                    aria-label={label}
                    onClick={() => onConfigure(provider.uuid)}
                    sx={{
                        ...size,
                        color: 'text.disabled',
                        opacity: 0,
                        transition: 'opacity 120ms',
                        '.proxy-row:hover &, .proxy-row:focus-within &, &:focus-visible': { opacity: 1 },
                        '@media (hover: none)': { opacity: 1 },
                    }}
                >
                    <Route fontSize="small" />
                </IconButton>
            </Tooltip>
        );
    }

    const on = state === 'on';
    const label = t('credentialPage.proxySwitchLabel', { name: provider.name });
    const hint = t(on ? 'credentialPage.proxyOnHint' : 'credentialPage.proxyOffHint', { url });
    return (
        <Tooltip title={hint} arrow>
            <span>
                <IconButton
                    size="small"
                    role="switch"
                    aria-checked={on}
                    aria-label={label}
                    aria-busy={pending || undefined}
                    disabled={pending || !onToggle}
                    onClick={() => onToggle?.(provider.uuid)}
                    sx={{ ...size, color: on ? 'success.main' : 'text.secondary' }}
                >
                    {pending ? <CircularProgress size={16} color="inherit" /> : on ? <Route fontSize="small" /> : <RouteOff fontSize="small" />}
                </IconButton>
            </span>
        </Tooltip>
    );
}
