import { alpha, type Theme } from '@mui/material/styles';

const routeActive = '#4F6F9F';
const routeActiveBg = '#F7F9FC';

export const getModelCardActiveColor = (theme: Theme) =>
    theme.palette.mode === 'dark' ? '#8EA7CF' : routeActive;

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
                    '0 14px 30px rgba(0, 0, 0, 0.34)',
                    `0 0 0 3px ${alpha(activeColor, 0.20)}`,
                ].join(', ')
                : [
                    `0 0 0 3px ${alpha(routeActive, 0.16)}`,
                    '0 8px 24px rgba(31, 41, 55, 0.10)',
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
                    '0 12px 24px rgba(0, 0, 0, 0.30)',
                    `0 0 0 3px ${alpha(activeColor, 0.14)}`,
                ].join(', ')
                : [
                    `0 0 0 3px ${alpha(routeActive, 0.10)}`,
                    '0 8px 24px rgba(31, 41, 55, 0.08)',
                ].join(', '),
            transform: 'translateY(-1px)',
        },
    };
};
