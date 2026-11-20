import { Add } from '@/components/icons';
import { ButtonBase, Stack, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';
import { getModelCardActiveColor, modelCardTransition, PHONE_ROW_MIN_HEIGHT } from './cardStyles';
import { usePhoneShell } from '@tingly/ui/hooks/usePhoneShell';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

interface AddCustomModelCardProps {
    onClick: () => void;
}

// First cell of the model grid: add a model ID the provider's list doesn't
// carry. Same footprint as a ModelCard, drawn dashed so it reads as an action.
export default function AddCustomModelCard({ onClick }: AddCustomModelCardProps) {
    const phone = usePhoneShell();
    return (
        <ButtonBase
            onClick={onClick}
            focusRipple
            sx={(theme) => {
                const activeColor = getModelCardActiveColor(theme);
                return {
                    width: '100%',
                    height: phone ? PHONE_ROW_MIN_HEIGHT : 60,
                    border: '1px dashed',
                    borderColor: theme.palette.divider,
                    borderRadius: radius.row,
                    color: 'text.secondary',
                    transition: modelCardTransition,
                    '&:hover, &.Mui-focusVisible': {
                        borderColor: activeColor,
                        color: activeColor,
                        backgroundColor: alpha(activeColor, theme.palette.mode === 'dark' ? 0.14 : 0.06),
                    },
                };
            }}
        >
            <Stack sx={{ alignItems: 'center', gap: 0.25 }}>
                <Stack direction="row" sx={{ alignItems: 'center', gap: 0.5 }}>
                    <Add sx={{ fontSize: 16 }} />
                    <Typography variant="body2" sx={{ fontWeight: 500, fontSize: fontSizes.md, lineHeight: 1.2 }}>
                        Custom Model
                    </Typography>
                </Stack>
                <Typography variant="caption" sx={{ fontSize: fontSizes.micro, lineHeight: 1.1, color: 'text.secondary' }}>
                    Enter any model ID
                </Typography>
            </Stack>
        </ButtonBase>
    );
}
