import { CheckCircle } from '@mui/icons-material';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import { Box, Card, CircularProgress, IconButton, Tooltip, Typography } from '@mui/material';
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
    loading?: boolean;
}

export default function CustomModelCard({
    model,
    provider,
    isSelected,
    onEdit,
    onDelete,
    onSelect,
    variant,
    loading = false,
}: CustomModelCardProps) {
    const handleCardClick = () => {
        if (!loading) {
            onSelect();
        }
    };

    const handleEditClick = (e: React.MouseEvent) => {
        e.stopPropagation();
        onEdit();
    };

    const handleDeleteClick = (e: React.MouseEvent) => {
        e.stopPropagation();
        onDelete();
    };

    return (
        <Card
            sx={{
                width: '100%',
                height: 60,
                border: 1,
                borderColor: variant === 'selected' ? 'primary.main' : 'grey.300',
                borderRadius: 1.5,
                backgroundColor: 'background.paper',
                cursor: loading ? 'wait' : 'pointer',
                transition: 'all 0.2s ease-in-out',
                position: 'relative',
                boxShadow: isSelected ? 2 : 0,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                overflow: 'hidden',
                '&:hover': loading ? {} : {
                    backgroundColor: 'grey.50',
                    boxShadow: 2,
                    '& .control-bar': {
                        opacity: 1,
                    },
                },
            }}
            onClick={handleCardClick}
        >
            {/* Main content area */}
            <Box sx={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                px: 2,
                width: '100%',
                height: '100%',
                zIndex: 1,
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
                        }}
                    >
                        {model}
                    </Typography>
                )}
            </Box>

            {/* Selected indicator */}
            {isSelected && !loading && (
                <CheckCircle
                    color="primary"
                    sx={{
                        position: 'absolute',
                        top: 4,
                        right: 4,
                        fontSize: 16,
                        zIndex: 2,
                    }}
                />
            )}

            {/* Triangle badge in bottom-left corner */}
            {!loading && (
                <Tooltip title="Custom model" arrow>
                    <Box
                        sx={{
                            position: 'absolute',
                            bottom: 0,
                            left: 0,
                            width: 20,
                            height: 20,
                            backgroundColor: 'primary.main',
                            clipPath: 'polygon(0 100%, 100% 100%, 0 0)',
                            cursor: 'help',
                        }}
                    />
                </Tooltip>
            )}

            {/* Control bar - visible on hover */}
            <Box
                className="control-bar"
                sx={{
                    position: 'absolute',
                    bottom: 0,
                    right: 0,
                    height: 20,
                    backgroundColor: 'grey.50',
                    borderTop: 1,
                    borderTopLeftRadius: 4,
                    borderColor: 'grey.200',
                    display: 'flex',
                    alignItems: 'center',
                    px: 0.5,
                    opacity: loading ? 0 : 0,
                    transition: 'opacity 0.2s',
                    zIndex: 10,
                }}
                onClick={(e) => {
                    e.stopPropagation();
                    e.preventDefault();
                }}
                onMouseDown={(e) => {
                    e.stopPropagation();
                    e.preventDefault();
                }}
            >
                <IconButton
                    size="small"
                    onClick={handleEditClick}
                    sx={{
                        p: 0.3,
                        color: 'text.secondary',
                        '&:hover': {
                            backgroundColor: 'rgba(0, 0, 0, 0.04)',
                            color: 'primary.main',
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
                        color: 'text.secondary',
                        '&:hover': {
                            backgroundColor: 'rgba(211, 47, 47, 0.08)',
                            color: 'error.main',
                        }
                    }}
                    title="Delete custom model"
                >
                    <DeleteIcon sx={{ fontSize: 14 }} />
                </IconButton>
            </Box>
        </Card>
    );
}