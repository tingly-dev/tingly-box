import CloseIcon from '@mui/icons-material/Close';
import { Box, Divider, IconButton, Typography } from '@mui/material';
import React from 'react';
import ModelCard from './ModelCard';

export interface NewModelsSectionProps {
    providerUuid: string;
    newModels: string[];
    selectedModel?: string;
    onModelSelect: (model: string) => void;
    onDismiss: () => void;
    columns: number;
}

export function NewModelsSection({
    providerUuid,
    newModels,
    selectedModel,
    onModelSelect,
    onDismiss,
    columns,
}: NewModelsSectionProps) {
    if (newModels.length === 0) {
        return null;
    }

    return (
        <Box>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                    New Models
                </Typography>
                <IconButton
                    size="small"
                    onClick={onDismiss}
                    sx={{ p: 0.5 }}
                    title="Dismiss new models"
                >
                    <CloseIcon fontSize="small" />
                </IconButton>
            </Box>
            <Box sx={{ display: 'grid', gridTemplateColumns: `repeat(${columns}, 1fr)`, gap: 0.8 }}>
                {newModels.map((model) => (
                    <ModelCard
                        key={model}
                        model={model}
                        isSelected={selectedModel === model}
                        onClick={() => onModelSelect(model)}
                        variant="standard"
                        showNewBadge
                    />
                ))}
            </Box>
            <Divider sx={{ mt: 2 }} />
        </Box>
    );
}

export default NewModelsSection;
