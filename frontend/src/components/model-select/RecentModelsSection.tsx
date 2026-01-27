import { Box, Divider, Typography } from '@mui/material';
import React from 'react';
import ModelCard from './ModelCard';

export interface RecentModelsSectionProps {
    providerUuid: string;
    recentModels: string[];
    selectedModel?: string;
    onModelSelect: (model: string) => void;
    columns: number;
}

export function RecentModelsSection({
    providerUuid,
    recentModels,
    selectedModel,
    onModelSelect,
    columns,
}: RecentModelsSectionProps) {
    if (recentModels.length === 0) {
        return null;
    }

    return (
        <Box>
            <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 600 }}>
                Recent
            </Typography>
            <Box sx={{ display: 'grid', gridTemplateColumns: `repeat(${columns}, 1fr)`, gap: 0.8 }}>
                {recentModels.map((model) => (
                    <ModelCard
                        key={model}
                        model={model}
                        isSelected={selectedModel === model}
                        onClick={() => onModelSelect(model)}
                        variant="standard"
                    />
                ))}
            </Box>
            <Divider sx={{ mt: 2 }} />
        </Box>
    );
}

export default RecentModelsSection;
