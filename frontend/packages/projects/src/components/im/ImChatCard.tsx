import { Box, Card, CardActionArea, Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import type { ProjectSummary } from '../../client';
import { useProjectsHost } from '../../host/ProjectsHost';
import type { ChannelBinding } from '../../client/channelBindings';
import { imDestination } from './useImBindings';
import { PlatformMark, usePlatformLabel } from './PlatformMark';

/**
 * A bound IM chat as a card: the chat, where it is bound, and what it is
 * talking to. Opening it lands in the bound place (same as on the IM page).
 */
export function ImChatCard({ b, place }: { b: ChannelBinding; place: ProjectSummary | undefined }) {
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const { paths } = useProjectsHost();
    const platformLabel = usePlatformLabel();
    const focus = b.folder || !b.thread_id
        ? t('im.focus.nextSession')
        : b.focus_agent === 'llm'
            ? t('im.focus.coordinator')
            : t('im.focus.thread', { number: b.focus_seq, title: b.focus_title || b.thread_id });
    return (
        <Card variant="outlined" sx={{ borderRadius: radius.card, '&:hover': { borderColor: 'text.disabled' } }}>
            <CardActionArea onClick={() => navigate(imDestination(b, place, paths))} sx={{ p: 2.25, height: '100%', display: 'flex', flexDirection: 'column', alignItems: 'stretch', justifyContent: 'flex-start' }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <PlatformMark platform={b.platform} size={18} colored />
                    <Typography variant="h5" noWrap sx={{ flex: 1 }}>{b.chat_title}</Typography>
                </Box>
                <Typography variant="body2" color="text.secondary" noWrap sx={{ mt: 0.5, minHeight: 40 }}>
                    {t('im.place.at', { place: place?.name ?? t('im.place.elsewhere') })}
                </Typography>
                <Typography variant="caption" color="text.secondary" noWrap sx={{ mt: 0.5, fontSize: fontSizes.sm }}>
                    {platformLabel(b.platform)} · {focus}
                </Typography>
            </CardActionArea>
        </Card>
    );
}
