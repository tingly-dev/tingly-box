import { Box, Chip, Stack, Typography } from '@mui/material';
import type { SxProps, Theme } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Brush, Person } from '@/components/icons';
import type { EntityKind, ImageEntity } from './entityTypes';

// One color per kind, used everywhere an entity shows up (picker, chips,
// library), so "blue = who, orange = how" is learned once. Blue/orange
// rather than hue-only pairs: they also differ in lightness.
export const KIND_COLOR: Record<EntityKind, { fg: string; bg: string; border: string }> = {
    character: { fg: '#2546A8', bg: 'rgba(47, 91, 211, 0.10)', border: 'rgba(47, 91, 211, 0.35)' },
    style: { fg: '#9A4609', bg: 'rgba(194, 86, 14, 0.10)', border: 'rgba(194, 86, 14, 0.35)' },
};

export const KindIcon: React.FC<{ kind: EntityKind; fontSize?: number }> = ({ kind, fontSize = 16 }) => (
    kind === 'character'
        ? <Person sx={{ fontSize, color: KIND_COLOR.character.fg }} />
        : <Brush sx={{ fontSize, color: KIND_COLOR.style.fg }} />
);

export const useKindLabel = () => {
    const { t } = useTranslation();
    return (kind: EntityKind) => (kind === 'character'
        ? t('imageEntity.kind.character', { defaultValue: 'Character' })
        : t('imageEntity.kind.style', { defaultValue: 'Style' }));
};

export const EntityThumb: React.FC<{ src?: string; size?: number; ratio?: number; radius?: number; sx?: SxProps<Theme> }> = ({
    src,
    size = 32,
    ratio = 1,
    radius = 1,
    sx,
}) => (
    <Box
        sx={[
            {
                width: size,
                height: Math.round(size * ratio),
                flexShrink: 0,
                borderRadius: radius,
                overflow: 'hidden',
                bgcolor: 'action.hover',
                '& img': { width: '100%', height: '100%', objectFit: 'cover', display: 'block' },
            },
            ...(Array.isArray(sx) ? sx : [sx]),
        ]}
    >
        {src && <img src={src} alt="" />}
    </Box>
);

// The `@name` token as it appears outside the text field.
export const EntityChip: React.FC<{
    entity: ImageEntity;
    detail?: string;
    onClick?: () => void;
}> = ({ entity, detail, onClick }) => {
    const color = KIND_COLOR[entity.kind];
    return (
        <Chip
            size="small"
            onClick={onClick}
            avatar={<EntityThumb src={entity.refs[0]} size={20} radius={10} sx={{ ml: '4px !important' }} />}
            label={(
                <Stack direction="row" spacing={0.75} sx={{ alignItems: 'baseline' }}>
                    <span>@{entity.name}</span>
                    {detail && (
                        <Typography component="span" sx={{ fontSize: 11, color: 'text.secondary' }}>{detail}</Typography>
                    )}
                </Stack>
            )}
            sx={{
                height: 28,
                color: color.fg,
                bgcolor: color.bg,
                border: '1px solid',
                borderColor: color.border,
                fontWeight: 500,
                '&:hover': { bgcolor: color.bg, filter: 'brightness(0.97)' },
            }}
        />
    );
};
