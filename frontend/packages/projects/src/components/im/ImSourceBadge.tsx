import { Box, Typography } from '@mui/material';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { useTranslation } from 'react-i18next';
import { PlatformMark, usePlatformLabel } from './PlatformMark';

/** "From Telegram · @yz": the small line under a project message that an IM chat wrote. */
export function ImSourceBadge({ platform, chat }: { platform: string; chat: string }) {
    const { t } = useTranslation('projects');
    const label = usePlatformLabel()(platform);
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.5, color: 'text.secondary', minWidth: 0, maxWidth: '100%' }}>
            {platform && <PlatformMark platform={platform} size={12} />}
            <Typography noWrap sx={{ fontSize: fontSizes.sm, lineHeight: 1.4 }}>{platform ? (chat ? t('im.source', { platform: label, chat }) : t('im.sourcePlatform', { platform: label })) : t('im.sourceBot')}</Typography>
        </Box>
    );
}
