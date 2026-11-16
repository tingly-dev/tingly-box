import { Box, Divider, Typography } from '@mui/material';
import type { Provider } from '@/types/provider';
import ModelCard from './ModelCard';

export interface RecentModelsSectionProps {
    provider: Provider;
    recentModels: string[];
    selectedModel?: string;
    // Overrides selectedModel when several models can be selected at once.
    isModelSelected?: (model: string) => boolean;
    onModelSelect: (model: string) => void;
    columns: number;
}

export function RecentModelsSection({
    provider,
    recentModels,
    selectedModel,
    isModelSelected,
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
                        key={`${provider.uuid}:${model}`}
                        model={model}
                        isSelected={isModelSelected ? isModelSelected(model) : selectedModel === model}
                        onClick={() => onModelSelect(model)}
                        variant="standard"
                        provider={provider}
                    />
                ))}
            </Box>
            <Divider sx={{ mt: 2 }} />
        </Box>
    );
}

export default RecentModelsSection;
