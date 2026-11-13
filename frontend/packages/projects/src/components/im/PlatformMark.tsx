import type { ReactNode } from 'react';
import { Box } from '@mui/material';
import { useProjectsHost } from '../../host/ProjectsHost';

const capitalise = (s: string) => (s ? s[0].toUpperCase() + s.slice(1) : s);

/** A platform's display name: the host's ("Telegram"), else the id capitalised. */
export function usePlatformLabel(): (platform: string) => string {
    const { im } = useProjectsHost();
    return (platform) => im?.platformLabel?.(platform) ?? capitalise(platform);
}

/** A platform's logo at `size` px, or a quiet initial when the host has none. */
export function PlatformMark({ platform, size = 18, colored = false }: { platform: string; size?: number; colored?: boolean }): ReactNode {
    const { im } = useProjectsHost();
    const icon = im?.platformIcon?.(platform, size, colored);
    if (icon) return <Box component="span" aria-hidden="true" sx={{ display: 'inline-flex', flexShrink: 0, width: size, height: size }}>{icon}</Box>;
    return (
        <Box
            component="span"
            aria-hidden="true"
            sx={{
                width: size, height: size, borderRadius: '50%', flexShrink: 0, bgcolor: 'action.selected', color: 'text.secondary',
                display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
                fontSize: Math.max(9, Math.round(size * 0.5)), fontWeight: 600, lineHeight: 1,
            }}
        >
            {capitalise(platform).charAt(0)}
        </Box>
    );
}
