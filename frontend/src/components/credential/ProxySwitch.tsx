import { CircularProgress, IconButton, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Route, RouteOff } from '@/components/icons';

export interface ProxyControl {
    pending: ReadonlySet<string>;
    /** Turning the proxy off clears its URL. */
    onDisable: (uuid: string) => void;
    /** Turning it on asks for (a prefilled) URL first. */
    onEnable: (uuid: string) => void;
}

interface Props {
    provider: { uuid: string; name: string; proxy_url?: string | null };
    proxy?: ProxyControl;
}

/**
 * One-click proxy control for a credential row. A stored proxy URL means "on":
 * the route icon turns it off (clears the URL). With none stored the control
 * stays hidden until its row (`.proxy-row`) is hovered or focused, and then
 * offers to turn one on; touch screens have no hover, so it stays visible there.
 */
export function ProxySwitch({ provider, proxy }: Props) {
    const { t } = useTranslation();
    if (!proxy) return null;
    const on = !!provider.proxy_url;
    const pending = proxy.pending.has(provider.uuid);
    const size = { width: { xs: 40, md: 28 }, height: { xs: 40, md: 28 } };
    const hidden = on ? {} : {
        opacity: 0,
        transition: 'opacity 120ms',
        '.proxy-row:hover &, .proxy-row:focus-within &, &:focus-visible': { opacity: 1 },
        '@media (hover: none)': { opacity: 1 },
    };

    return (
        <Tooltip title={on ? t('credentialPage.proxyOnHint', { url: provider.proxy_url }) : t('credentialPage.proxyOffHint')} arrow>
            <span>
                <IconButton
                    size="small"
                    role="switch"
                    aria-checked={on}
                    aria-label={t('credentialPage.proxySwitchLabel', { name: provider.name })}
                    aria-busy={pending || undefined}
                    disabled={pending}
                    onClick={() => (on ? proxy.onDisable(provider.uuid) : proxy.onEnable(provider.uuid))}
                    sx={{ ...size, color: on ? 'success.main' : 'text.secondary', ...hidden }}
                >
                    {pending ? <CircularProgress size={16} color="inherit" /> : on ? <Route fontSize="small" /> : <RouteOff fontSize="small" />}
                </IconButton>
            </span>
        </Tooltip>
    );
}
