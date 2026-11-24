import { Box, ToggleButton, ToggleButtonGroup } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { ControlTooltip } from './ControlTooltip';
import { fontSizes } from '@tingly/ui/theme/fonts';
import type { DetailSurface } from '../hooks/useMessageDetail';
import type { MessageDetail } from '../transcript/messageDetail';

export function MessageDetailControl({ detail, onChange, surface }: {
    detail: MessageDetail;
    onChange: (detail: MessageDetail) => void;
    surface: DetailSurface;
}) {
    const { t } = useTranslation('projects');
    return (
        <Box sx={{ display: 'flex', justifyContent: 'flex-end', pb: 0.5 }}>
            <ControlTooltip title={t(surface === 'conversation' ? 'messageDetail.managerHint' : 'messageDetail.threadHint')}>
                <ToggleButtonGroup exclusive size="small" value={detail} aria-label={t('messageDetail.label')}
                    onChange={(_, next: MessageDetail | null) => next && onChange(next)}
                    sx={{ '& .MuiToggleButton-root': { py: 0.125, px: 1, fontSize: fontSizes.sm, textTransform: 'none' } }}>
                    <ToggleButton value="detailed">{t('messageDetail.detailed')}</ToggleButton>
                    <ToggleButton value="concise">{t('messageDetail.concise')}</ToggleButton>
                </ToggleButtonGroup>
            </ControlTooltip>
        </Box>
    );
}
