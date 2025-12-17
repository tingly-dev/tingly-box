import { CheckCircle } from '@mui/icons-material';
import { Box, Card, CardContent, IconButton, Typography } from '@mui/material';
import React from 'react';

interface ModelCardProps {
    model: string;
    isSelected: boolean;
    onClick: () => void;
    variant?: 'standard' | 'starred';
    gridColumns?: number;
}

export default function ModelCard({
    model,
    isSelected,
    onClick,
    variant = 'standard',
    gridColumns
}: ModelCardProps) {
    const getCardStyles = () => {
        const baseStyles = {
            width: '100%',
            height: 60,
            border: 1,
            borderRadius: 1.5,
            cursor: 'pointer',
            transition: 'all 0.2s ease-in-out',
            position: 'relative' as const,
            boxShadow: isSelected ? 2 : 0,
            '&:hover': {
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
        <Card sx={getCardStyles()} onClick={onClick}>
            <CardContent sx={{
                textAlign: 'center',
                py: 0.5,
                px: 0.8,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                height: '100%'
            }}>
                <Typography
                    variant="body2"
                    sx={{
                        fontWeight: 500,
                        fontSize: '0.8rem',
                        lineHeight: 1.3,
                        wordBreak: 'break-word',
                        textAlign: 'center'
                    }}
                >
                    {model}
                </Typography>
                {isSelected && (
                    <CheckCircle
                        color="primary"
                        sx={{
                            position: 'absolute',
                            top: 2,
                            right: 2,
                            fontSize: 14
                        }}
                    />
                )}
            </CardContent>
        </Card>
    );
}