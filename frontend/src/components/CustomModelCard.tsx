import { CheckCircle } from '@mui/icons-material';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { Box, Card, IconButton, Typography } from '@mui/material';
import React from 'react';
import type { Provider } from '../types/provider';

interface CustomModelCardProps {
    model: string;
    provider: Provider;
    isSelected: boolean;
    onEdit: () => void;
    onDelete: () => void;
    onSelect: () => void;
    variant: 'localStorage' | 'backend' | 'selected';
}

export default function CustomModelCard({
    model,
    provider,
    isSelected,
    onEdit,
    onDelete,
    onSelect,
    variant
}: CustomModelCardProps) {
    const handleCardClick = () => {
        onSelect();
    };

    const handleEditClick = (e: React.MouseEvent) => {
        e.stopPropagation();
        onEdit();
    };

    const handleDeleteClick = (e: React.MouseEvent) => {
        e.stopPropagation();
        onDelete();
    };

    const showEditButton = variant !== 'backend';
    const showLabel = variant === 'localStorage';

    return (
        <Card
            sx={{
                width: '100%',
                height: 60,
                border: 1,
                borderColor: variant === 'selected' ? 'primary.main' : 'grey.300',
                borderRadius: 1.5,
                backgroundColor: 'background.paper',
                cursor: 'pointer',
                transition: 'all 0.2s ease-in-out',
                position: 'relative',
                boxShadow: isSelected ? 2 : 0,
                display: 'flex',
                flexDirection: 'column',
                '&:hover': {
                    backgroundColor: 'grey.50',
                    boxShadow: 2,
                },
            }}
            onClick={handleCardClick}
        >
            {/* Main content area */}
            <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', px: 1, py: 0.5 }}>
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
            </Box>
            {/* Control bar */}
            <Box
                sx={{
                    height: 20,
                    backgroundColor: 'primary.100',
                    borderTop: 1,
                    borderColor: variant === 'selected' ? 'primary.main' : 'primary.main',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: showLabel ? 'space-between' : 'flex-end',
                    px: 0.5,
                    borderRadius: '0 0 12px 12px',
                }}
                onClick={(e) => e.stopPropagation()}
            >
                {showLabel && (
                    <Typography
                        variant="caption"
                        sx={{
                            fontSize: '0.65rem',
                            fontWeight: 600,
                            color: 'primary.main',
                            textTransform: 'uppercase',
                            letterSpacing: 0.5,
                            pl: 0.5,
                        }}
                    >
                        custom
                    </Typography>
                )}
                <Box>
                    <IconButton
                        size="small"
                        onClick={handleEditClick}
                        sx={{
                            p: 0.3,
                            '&:hover': {
                                backgroundColor: 'primary.100',
                            }
                        }}
                        title="Edit custom model"
                    >
                        <EditIcon sx={{ fontSize: 14 }} />
                    </IconButton>
                    <IconButton
                        size="small"
                        onClick={handleDeleteClick}
                        sx={{
                            p: 0.3,
                            '&:hover': {
                                backgroundColor: 'rgba(211, 47, 47, 0.04)',
                            }
                        }}
                        title="Delete custom model"
                    >
                        <DeleteIcon sx={{ fontSize: 14 }} />
                    </IconButton>
                </Box>
            </Box>
        </Card>
    );
}