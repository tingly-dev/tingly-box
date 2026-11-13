import { alpha, type Theme } from '@mui/material/styles';
import { routeGraphColors, routeGraphShadows } from '@/theme/routeGraph';

const routeActive = routeGraphColors.active;
const routeActiveBg = routeGraphColors.activeBg;

export const getModelCardActiveColor = (theme: Theme) =>
    theme.palette.mode === 'dark' ? routeGraphColors.modelCardActiveDark : routeActive;

export const modelCardTransition =
    'border-color 0.16s ease, background-color 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease';

export const getModelCardStateStyles = (theme: Theme, isSelected: boolean) => {
    const isDark = theme.palette.mode === 'dark';
    const activeColor = getModelCardActiveColor(theme);
    const activeBg = isDark ? alpha(routeActive, 0.18) : routeActiveBg;

    if (isSelected) {
        return {
            borderColor: activeColor,
            backgroundColor: activeBg,
            boxShadow: isDark
                ? [
                    routeGraphShadows.cardSelectedDark,
                    `0 0 0 3px ${alpha(activeColor, 0.20)}`,
                ].join(', ')
                : [
                    `0 0 0 3px ${alpha(routeActive, 0.16)}`,
                    routeGraphShadows.cardSelected,
                ].join(', '),
            transform: 'translateY(-1px)',
            '&:hover': {
                borderColor: activeColor,
                backgroundColor: activeBg,
            },
        };
    }

    return {
        borderColor: theme.palette.divider,
        backgroundColor: theme.palette.background.paper,
        boxShadow: 'none',
        transform: 'translateY(0)',
        '&:hover': {
            borderColor: activeColor,
            backgroundColor: activeBg,
            boxShadow: isDark
                ? [
                    routeGraphShadows.cardHoverDark,
                    `0 0 0 3px ${alpha(activeColor, 0.14)}`,
                ].join(', ')
                : [
                    `0 0 0 3px ${alpha(routeActive, 0.10)}`,
                    routeGraphShadows.cardHover,
                ].join(', '),
            transform: 'translateY(-1px)',
        },
    };
};
