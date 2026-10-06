import type { ReactNode } from 'react';
import { Box } from '@mui/material';
import type { SxProps, Theme } from '@mui/material';
import ProviderIcon from '@/components/ProviderIcon';
import { useProviderIconId, type IconSource } from '@/utils/providerIcon';
import { useProvidersByUuid } from '@/hooks/useProvidersByUuid';

/**
 * The vendor mark for a configured provider, shown beside its name in lists.
 * It says who the provider is; ApiStyleBadge beside it says which protocol it
 * speaks — two separate questions. Unrecognised providers get ProviderIcon's
 * neutral placeholder so names stay aligned down a column.
 */
export default function ProviderLogo({ provider, size = 18, sx }: {
    provider: IconSource | undefined;
    size?: number;
    sx?: SxProps<Theme>;
}) {
    const iconId = useProviderIconId();
    return <ProviderIcon identifier={iconId(provider) ?? ''} size={size} sx={sx} />;
}

/**
 * ProviderLogo for rows that only know a provider's uuid (dashboard tables):
 * the provider is looked up from the shared list; until it loads, or for a
 * provider since deleted, the placeholder shows.
 */
export function ProviderLogoByUuid({ uuid, ...rest }: { uuid: string | undefined } & Omit<Parameters<typeof ProviderLogo>[0], 'provider'>) {
    const providers = useProvidersByUuid();
    return <ProviderLogo provider={uuid ? providers.get(uuid) : undefined} {...rest} />;
}

/**
 * A provider's logo followed by its name — the one place that decides how the
 * pair sits together, so every list aligns the same way. Pass `provider` when
 * the row has the full record, or `uuid` when it only knows the uuid (dashboard
 * rows). `leading` replaces the logo, e.g. a warning icon for a missing
 * provider. The name is the caller's child, so each surface keeps its own
 * typography, tooltip and truncation (give a truncating name `minWidth: 0`).
 */
export function ProviderLabel({ provider, uuid, leading, size = 18, gap = 1, sx, children }: {
    provider?: IconSource;
    uuid?: string;
    leading?: ReactNode;
    size?: number;
    /** Space between logo and name, in theme spacing units. */
    gap?: number;
    sx?: SxProps<Theme>;
    children: ReactNode;
}) {
    const logo = leading ?? (provider
        ? <ProviderLogo provider={provider} size={size} />
        : <ProviderLogoByUuid uuid={uuid} size={size} />);
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap, minWidth: 0, ...sx }}>
            {logo}
            {children}
        </Box>
    );
}
