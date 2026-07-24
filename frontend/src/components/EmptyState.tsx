import { Add } from '@/components/icons';
import { Box, Button, Stack, Typography } from '@mui/material';
import { type ReactNode } from 'react';

export interface EmptyStateAction {
    label: ReactNode;
    onClick: () => void;
    icon?: ReactNode;
    variant?: 'contained' | 'outlined' | 'text';
}

interface EmptyStateProps {
    title: ReactNode;
    description?: ReactNode;
    icon?: ReactNode;
    primaryAction?: EmptyStateAction;
    secondaryAction?: EmptyStateAction;
    compact?: boolean;
    titleHeadingLevel?: 2 | 3 | 4 | 5 | 6;
}

const EmptyState = ({
    title,
    description,
    icon,
    primaryAction,
    secondaryAction,
    compact = false,
    titleHeadingLevel = 3,
}: EmptyStateProps) => {
    const actions = [
        secondaryAction && { ...secondaryAction, defaultVariant: 'outlined' as const },
        primaryAction && { ...primaryAction, defaultVariant: 'contained' as const },
    ].filter(Boolean) as Array<EmptyStateAction & { defaultVariant: 'contained' | 'outlined' }>;

    return (
        <Box
            sx={{
                width: '100%',
                px: 2,
                py: compact ? 4 : 6,
                textAlign: 'center',
            }}
        >
            {icon && (
                <Box
                    aria-hidden="true"
                    sx={{
                        width: 56,
                        height: 56,
                        mx: 'auto',
                        mb: 2,
                        borderRadius: 2,
                        bgcolor: 'action.hover',
                        color: 'primary.main',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        '& > svg': {
                            fontSize: 28,
                        },
                    }}
                >
                    {icon}
                </Box>
            )}
            <Typography
                component={`h${titleHeadingLevel}`}
                variant={compact ? 'h6' : 'h5'}
                sx={{ fontWeight: 600 }}
            >
                {title}
            </Typography>
            {description && (
                <Typography
                    variant="body1"
                    sx={{
                        color: 'text.secondary',
                        mt: 1,
                        maxWidth: 560,
                        mx: 'auto',
                    }}
                >
                    {description}
                </Typography>
            )}
            {actions.length > 0 && (
                <Stack
                    direction="row"
                    spacing={1.5}
                    sx={{
                        justifyContent: 'center',
                        flexWrap: 'wrap',
                        mt: 3,
                    }}
                >
                    {actions.map((action, index) => (
                        <Button
                            key={index}
                            variant={action.variant ?? action.defaultVariant}
                            startIcon={action.icon === undefined ? <Add /> : action.icon}
                            onClick={action.onClick}
                        >
                            {action.label}
                        </Button>
                    ))}
                </Stack>
            )}
        </Box>
    );
};

export default EmptyState;
