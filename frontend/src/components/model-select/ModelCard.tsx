import { CheckCircle } from '@/components/icons';
import { Box, Card, CardContent, CircularProgress, Tooltip, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';
import type { Theme } from '@mui/material/styles';
import { getModelCardActiveColor, getModelCardStateStyles, modelCardTransition } from './cardStyles';

interface ModelCardProps {
    model: string;
    isSelected: boolean;
    onClick: () => void;
    variant?: 'standard' | 'starred';
    gridColumns?: number;
    loading?: boolean;
    showNewBadge?: boolean;
    description?: string; // Model description from API
}

export default function ModelCard({
    model,
    isSelected,
    onClick,
    variant = 'standard',
    gridColumns,
    loading = false,
    showNewBadge = false,
    description,
}: ModelCardProps) {
    const getCardStyles = () => {
        const baseStyles = {
            width: '100%',
            height: 60,
            border: 1,
            borderRadius: 1,
            cursor: loading ? 'wait' : 'pointer',
            transition: modelCardTransition,
            position: 'relative' as const,
            outline: 'none',
            overflow: 'visible',
        };

        if (variant === 'starred') {
            return (theme: Theme) => ({
                ...baseStyles,
                ...(isSelected
                    ? getModelCardStateStyles(theme, true)
                    : {
                        borderColor: theme.palette.warning.main,
                        backgroundColor: alpha(theme.palette.warning.main, theme.palette.mode === 'dark' ? 0.14 : 0.08),
                        boxShadow: 'none',
                        transform: 'translateY(0)',
                    }),
                ...(loading ? {} : {
                    '&:hover': {
                        ...(isSelected
                            ? getModelCardStateStyles(theme, true)
                            : getModelCardStateStyles(theme, false)['&:hover']),
                        transform: 'translateY(-1px)',
                    },
                }),
            });
        }

        return (theme: Theme) => ({
            ...baseStyles,
            ...(loading
                ? {
                    borderColor: theme.palette.divider,
                    backgroundColor: theme.palette.background.paper,
                    boxShadow: 'none',
                    transform: 'translateY(0)',
                }
                : getModelCardStateStyles(theme, isSelected)),
        });
    };

    return (
        <Card sx={getCardStyles()} onClick={loading ? undefined : onClick}>
            <CardContent sx={{
                py: 1,
                px: 1,
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                height: '100%',
                gap: 0.5,
                '&:last-child': {
                    pb: 1,
                }
            }}>
                {loading ? (
                    <CircularProgress size={20} />
                ) : (
                    <>
                        <Tooltip title={model} arrow disableInteractive>
                            <Typography
                                variant="body2"
                                sx={{
                                    fontWeight: 500,
                                    fontSize: '0.8rem',
                                    lineHeight: 1.2,
                                    display: '-webkit-box',
                                    WebkitLineClamp: 3,
                                    WebkitBoxOrient: 'vertical',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    wordBreak: 'break-word',
                                    textAlign: 'center',
                                    width: '100%',
                                }}
                            >
                                {model}
                            </Typography>
                        </Tooltip>
                        {description && (
                            <Typography
                                variant="caption"
                                sx={{
                                    fontSize: '0.65rem',
                                    lineHeight: 1.1,
                                    color: 'text.secondary',
                                    textAlign: 'center',
                                    display: '-webkit-box',
                                    WebkitLineClamp: 2,
                                    WebkitBoxOrient: 'vertical',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    width: '100%',
                                }}
                            >
                                {description}
                            </Typography>
                        )}
                    </>
                )}
                {isSelected && !loading && (
                    <CheckCircle
                        sx={{
                            position: 'absolute',
                            top: 4,
                            right: 4,
                            fontSize: 16,
                            color: (theme) => getModelCardActiveColor(theme),
                        }}
                    />
                )}
                {showNewBadge && !loading && (
                    <Box
                        sx={{
                            position: 'absolute',
                            top: 4,
                            left: 4,
                            bgcolor: 'success.main',
                            color: 'white',
                            fontSize: '0.6rem',
                            px: 0.5,
                            py: 0.2,
                            borderRadius: 1,
                            fontWeight: 'bold',
                        }}
                    >
                        NEW
                    </Box>
                )}
            </CardContent>
        </Card>
    );
}
