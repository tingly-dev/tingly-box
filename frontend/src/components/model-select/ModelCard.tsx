import { CheckCircle } from '@mui/icons-material';
import { Box, Card, CardContent, CircularProgress, IconButton, Typography } from '@mui/material';
import React from 'react';

interface ModelCardProps {
    model: string;
    isSelected: boolean;
    onClick: () => void;
    variant?: 'standard' | 'starred';
    gridColumns?: number;
    loading?: boolean;
}

export default function ModelCard({
    model,
    isSelected,
    onClick,
    variant = 'standard',
    gridColumns,
    loading = false,
}: ModelCardProps) {
    const getCardStyles = () => {
        const baseStyles = {
            width: '100%',
            height: 60,
            border: 1,
            borderRadius: 1.5,
            cursor: loading ? 'wait' : 'pointer',
            transition: 'all 0.2s ease-in-out',
            position: 'relative' as const,
            boxShadow: isSelected ? 2 : 0,
            '&:hover': loading ? {} : {
                boxShadow: 2,
            },
        };

        if (variant === 'starred') {
            return {
                ...baseStyles,
                borderColor: isSelected ? 'primary.main' : 'warning.main',
                backgroundColor: isSelected ? 'primary.50' : 'warning.50',
                '&:hover': {
                    backgroundColor: isSelected ? 'primary.100' : 'warning.100',
                },
            };
        }

        return {
            ...baseStyles,
            borderColor: isSelected ? 'primary.main' : 'grey.300',
            backgroundColor: isSelected ? 'primary.50' : 'background.paper',
            '&:hover': {
                backgroundColor: isSelected ? 'primary.100' : 'grey.50',
            },
        };
    };

    return (
        <Card sx={getCardStyles()} onClick={loading ? undefined : onClick}>
            <CardContent sx={{
                py: 1,
                px: 1,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                height: '100%',
                '&:last-child': {
                    pb: 1,
                }
            }}>
                {loading ? (
                    <CircularProgress size={20} />
                ) : (
                    <Typography
                        variant="body2"
                        sx={{
                            fontWeight: 500,
                            fontSize: '0.8rem',
                            lineHeight: 1.2,
                            wordBreak: 'break-word',
                            textAlign: 'center',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            width: '100%',
                        }}
                    >
                        {model}
                    </Typography>
                )}
                {isSelected && !loading && (
                    <CheckCircle
                        color="primary"
                        sx={{
                            position: 'absolute',
                            top: 4,
                            right: 4,
                            fontSize: 16
                        }}
                    />
                )}
            </CardContent>
        </Card>
    );
}