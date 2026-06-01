import { Box, Chip, Divider, Typography, styled } from '@mui/material';
import {
    NODE_LAYER_STYLES,
    getRouteGraphBorderColor,
    graphNodeBaseHoverStyles,
    graphNodeHoverStyles,
} from './styles';
import type { Theme } from '@mui/material/styles';

type AtType = 'tb' | 'cc';

const AT_CONFIG: Record<AtType, { label: string; chipColor: 'primary' | 'secondary' }> = {
    tb: { label: '@tb', chipColor: 'primary' },
    cc: { label: '@cc', chipColor: 'secondary' },
};

const StyledAtNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'clickable',
})<{ clickable: boolean }>(({ clickable, theme }: { clickable: boolean; theme: Theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 8,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: getRouteGraphBorderColor(theme),
    backgroundColor: theme.palette.background.paper,
    textAlign: 'center',
    width: 120,
    height: 72,
    boxShadow: 'none',
    transition: 'border-color 0.16s ease, background-color 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
    cursor: clickable ? 'pointer' : 'default',
    ...graphNodeBaseHoverStyles,
    ...(clickable && {
        '&:hover': graphNodeHoverStyles(theme),
    }),
}));

interface AtNodeProps {
    type: AtType;
    onClick?: () => void;
}

const AtNode: React.FC<AtNodeProps> = ({ type, onClick }) => {
    const config = AT_CONFIG[type];
    const clickable = !!onClick;

    return (
        <StyledAtNode clickable={clickable} onClick={onClick}>
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <Typography variant="body2" sx={{ ...NODE_LAYER_STYLES.typography, color: 'text.secondary' }}>
                    Route
                </Typography>
            </Box>

            <Divider sx={NODE_LAYER_STYLES.divider} />

            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <Chip
                    label={config.label}
                    size="small"
                    color={config.chipColor}
                    sx={{ height: 22, fontSize: '0.75rem', fontWeight: 700, fontFamily: 'monospace' }}
                />
            </Box>
        </StyledAtNode>
    );
};

export default AtNode;
