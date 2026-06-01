import { Box, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';
import { getRouteGraphActiveColor, graphNodeHoverStyles, NODE_LAYER_STYLES } from './styles';

type AtType = 'tb' | 'cc';

const AT_LABELS: Record<AtType, string> = {
    tb: '@tb',
    cc: '@cc',
};

interface AtNodeProps {
    type: AtType;
    onClick?: () => void;
}

const AtNode: React.FC<AtNodeProps> = ({ type, onClick }) => {
    const clickable = !!onClick;

    return (
        <Box
            onClick={onClick}
            sx={(theme) => ({
                width: 72,
                height: 36,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                borderRadius: `${theme.shape.borderRadius}px`,
                border: '1px solid',
                borderColor: alpha(
                    getRouteGraphActiveColor(theme),
                    theme.palette.mode === 'dark' ? 0.45 : 0.38,
                ),
                transition: 'border-color 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
                cursor: clickable ? 'pointer' : 'default',
                ...(clickable && {
                    '&:hover': graphNodeHoverStyles(theme),
                }),
            })}
        >
            <Typography
                sx={{
                    ...NODE_LAYER_STYLES.typography,
                    fontFamily: 'monospace',
                    color: 'text.secondary',
                }}
            >
                {AT_LABELS[type]}
            </Typography>
        </Box>
    );
};

export default AtNode;
