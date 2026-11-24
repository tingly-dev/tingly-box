import { Box, Typography } from '@mui/material';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { useTranslation } from 'react-i18next';
import { ImChatLabel } from './ImChatLabel';
import { PlatformMark, usePlatformLabel } from './PlatformMark';

/** "From Telegram · 群 · Release Crew · 1002…": the small line under a project message that an IM chat wrote. */
export function ImSourceBadge({ platform, chatId }: { platform: string; chatId: string }) {
    const { t } = useTranslation('projects');
    const label = usePlatformLabel()(platform);
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.5, color: 'text.secondary', minWidth: 0, maxWidth: '100%' }}>
            {platform && chatId ? (
                <>
                    <Typography noWrap sx={{ fontSize: fontSizes.sm, lineHeight: 1.4, flexShrink: 0 }}>{t('im.sourceFrom')}</Typography>
                    <ImChatLabel platform={platform} chatId={chatId} platformAs="text" />
                </>
            ) : (
                <>
                    {platform && <PlatformMark platform={platform} size={12} />}
                    <Typography noWrap sx={{ fontSize: fontSizes.sm, lineHeight: 1.4 }}>{platform ? t('im.sourcePlatform', { platform: label }) : t('im.sourceBot')}</Typography>
                </>
            )}
        </Box>
    );
}
