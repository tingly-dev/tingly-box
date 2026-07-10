import { useCallback, useMemo, useState } from 'react';
import {
    Box,
    Button,
    Card,
    CardContent,
    CircularProgress,
    FormControl,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Rule } from '@/components/RoutingGraphTypes';
import UnifiedCard from '@/components/UnifiedCard';
import { getOpenAIClient } from '@/services/openaiClient';

const IMAGE_SCENARIO = 'imagegen';

type Quality = 'auto' | 'high' | 'medium' | 'low' | 'standard';

interface ImageGenPlaygroundCardProps {
    rules: Rule[];
    loadingRules: boolean;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
}

const ImageGenPlaygroundCard: React.FC<ImageGenPlaygroundCardProps> = ({
    rules,
    loadingRules,
    showNotification,
}) => {
    const { t } = useTranslation();
    const models = useMemo(() => {
        const names = rules
            .filter((rule) => rule.active !== false && rule.request_model?.trim())
            .map((rule) => rule.request_model.trim());
        return Array.from(new Set(names));
    }, [rules]);

    const [selectedModel, setSelectedModel] = useState('');
    const model = models.includes(selectedModel) ? selectedModel : (models[0] ?? '');
    const [prompt, setPrompt] = useState('');
    const [size, setSize] = useState('1024x1024');
    const [quality, setQuality] = useState<Quality>('auto');
    const [count, setCount] = useState(1);
    const [results, setResults] = useState<{ url?: string; b64_json?: string }[]>([]);
    const [sending, setSending] = useState(false);

    const handleGenerate = useCallback(async () => {
        if (!prompt.trim() || !model) return;
        setSending(true);
        setResults([]);
        try {
            const client = await getOpenAIClient(IMAGE_SCENARIO);
            const response = await client.images.generate({
                model,
                prompt: prompt.trim(),
                n: count,
                size: size as any,
                quality,
            });
            setResults(response.data ?? []);
        } catch (error: any) {
            const status = error?.status ? `${error.status}: ` : '';
            const message = error?.error?.message || error?.message || 'Request failed';
            showNotification(`${status}${message}`, 'error');
        } finally {
            setSending(false);
        }
    }, [count, model, prompt, quality, showNotification, size]);

    const noModels = models.length === 0;

    return (
        <UnifiedCard
            size="full"
            title={t('playground.imageTitle', { defaultValue: 'Image Playground' })}
        >
            <Stack spacing={2}>
                {noModels && !loadingRules && (
                    <Typography variant="body2" color="text.secondary">
                        {t('playground.noImageModels', {
                            defaultValue: 'Add an image generation model rule below to start generating images.',
                        })}
                    </Typography>
                )}

                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} flexWrap="wrap">
                    <FormControl size="small" sx={{ minWidth: 220 }}>
                        <InputLabel id="image-model-label">
                            {t('playground.model', { defaultValue: 'Model' })}
                        </InputLabel>
                        <Select
                            labelId="image-model-label"
                            label={t('playground.model', { defaultValue: 'Model' })}
                            value={model}
                            onChange={(event) => setSelectedModel(event.target.value)}
                            disabled={noModels}
                        >
                            {models.map((modelName) => (
                                <MenuItem key={modelName} value={modelName}>{modelName}</MenuItem>
                            ))}
                        </Select>
                    </FormControl>
                    <FormControl size="small" sx={{ minWidth: 140 }}>
                        <InputLabel id="image-size-label">
                            {t('playground.size', { defaultValue: 'Size' })}
                        </InputLabel>
                        <Select
                            labelId="image-size-label"
                            label={t('playground.size', { defaultValue: 'Size' })}
                            value={size}
                            onChange={(event) => setSize(event.target.value)}
                        >
                            <MenuItem value="256x256">256x256</MenuItem>
                            <MenuItem value="512x512">512x512</MenuItem>
                            <MenuItem value="1024x1024">1024x1024</MenuItem>
                            <MenuItem value="1024x1792">1024x1792</MenuItem>
                            <MenuItem value="1792x1024">1792x1024</MenuItem>
                        </Select>
                    </FormControl>
                    <FormControl size="small" sx={{ minWidth: 140 }}>
                        <InputLabel id="image-quality-label">
                            {t('playground.quality', { defaultValue: 'Quality' })}
                        </InputLabel>
                        <Select
                            labelId="image-quality-label"
                            label={t('playground.quality', { defaultValue: 'Quality' })}
                            value={quality}
                            onChange={(event) => setQuality(event.target.value as Quality)}
                        >
                            <MenuItem value="auto">auto</MenuItem>
                            <MenuItem value="low">low</MenuItem>
                            <MenuItem value="medium">medium</MenuItem>
                            <MenuItem value="high">high</MenuItem>
                            <MenuItem value="standard">standard</MenuItem>
                        </Select>
                    </FormControl>
                    <TextField
                        size="small"
                        type="number"
                        label={t('playground.count', { defaultValue: 'N' })}
                        value={count}
                        onChange={(event) => {
                            const nextCount = Number(event.target.value);
                            setCount(Number.isFinite(nextCount) && nextCount > 0 ? Math.min(nextCount, 10) : 1);
                        }}
                        sx={{ width: 100 }}
                        slotProps={{ htmlInput: { min: 1, max: 10 } }}
                    />
                </Stack>

                <TextField
                    multiline
                    minRows={4}
                    fullWidth
                    placeholder={t('playground.promptPlaceholder', {
                        defaultValue: 'Describe the image you want to generate…',
                    })}
                    value={prompt}
                    onChange={(event) => setPrompt(event.target.value)}
                    disabled={noModels}
                />

                <Box>
                    <Button
                        variant="contained"
                        onClick={handleGenerate}
                        disabled={sending || noModels || !prompt.trim() || !model}
                        startIcon={sending ? <CircularProgress size={16} /> : undefined}
                    >
                        {sending
                            ? t('playground.generating', { defaultValue: 'Generating…' })
                            : t('playground.generate', { defaultValue: 'Generate' })}
                    </Button>
                </Box>

                {results.length > 0 && (
                    <Box
                        sx={{
                            display: 'grid',
                            gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))',
                            gap: 2,
                        }}
                    >
                        {results.map((image, index) => {
                            const src = image.url || (image.b64_json
                                ? `data:image/png;base64,${image.b64_json}`
                                : '');
                            return (
                                <Card key={`${src.slice(0, 40)}-${index}`} variant="outlined">
                                    <CardContent sx={{ p: 1, '&:last-child': { pb: 1 } }}>
                                        {src ? (
                                            <Box
                                                component="img"
                                                src={src}
                                                alt={t('playground.resultAlt', {
                                                    defaultValue: 'Generated image {{number}}',
                                                    number: index + 1,
                                                })}
                                                sx={{ width: '100%', display: 'block', borderRadius: 1 }}
                                            />
                                        ) : (
                                            <Typography variant="caption" color="text.secondary">
                                                {t('playground.emptyResult', { defaultValue: 'No image returned' })}
                                            </Typography>
                                        )}
                                    </CardContent>
                                </Card>
                            );
                        })}
                    </Box>
                )}
            </Stack>
        </UnifiedCard>
    );
};

export default ImageGenPlaygroundCard;
