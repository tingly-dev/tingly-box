import { useCallback, useMemo, useState } from 'react';
import {
    Box,
    Alert,
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
import { AutoAwesome, Photo } from '@/components/icons';
import { getOpenAIClient } from '@/services/openaiClient';

const IMAGE_SCENARIO = 'imagegen';

type Quality = 'auto' | 'high' | 'medium' | 'low' | 'standard';

interface ImageResult {
    url?: string;
    b64_json?: string;
}

interface GenerationRun {
    id: string;
    prompt: string;
    model: string;
    size: string;
    quality: Quality;
    images: ImageResult[];
}

// Keep playground output while navigating between pages in the current app session.
// This deliberately stays in memory: base64 images can quickly exceed sessionStorage quotas.
let imageGenSessionRuns: GenerationRun[] = [];

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
    const [runs, setRuns] = useState<GenerationRun[]>(() => imageGenSessionRuns);
    const [sending, setSending] = useState(false);

    const handleGenerate = useCallback(async () => {
        if (!prompt.trim() || !model) return;
        const generationPrompt = prompt.trim();
        const generationModel = model;
        const generationSize = size;
        const generationQuality = quality;
        setSending(true);
        try {
            const client = await getOpenAIClient(IMAGE_SCENARIO);
            const response = await client.images.generate({
                model: generationModel,
                prompt: generationPrompt,
                n: count,
                size: generationSize as any,
                quality: generationQuality,
            });
            const images = response.data ?? [];
            setRuns((currentRuns) => {
                const nextRuns = [{
                    id: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
                    prompt: generationPrompt,
                    model: generationModel,
                    size: generationSize,
                    quality: generationQuality,
                    images,
                }, ...currentRuns];
                imageGenSessionRuns = nextRuns;
                return nextRuns;
            });
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
            <Box
                sx={{
                    display: 'grid',
                    gridTemplateColumns: { xs: '1fr', md: 'minmax(360px, 0.9fr) minmax(420px, 1.1fr)' },
                    gap: 3,
                    alignItems: 'stretch',
                }}
            >
                <Stack spacing={2}>
                    {noModels && !loadingRules && (
                        <Alert severity="info" variant="outlined">
                            {t('playground.noImageModels', {
                                defaultValue: 'Add an image generation model rule below to start generating images.',
                            })}
                        </Alert>
                    )}

                    <FormControl size="small" fullWidth>
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

                    <TextField
                        multiline
                        minRows={7}
                        fullWidth
                        label={t('playground.prompt', { defaultValue: 'Prompt' })}
                        placeholder={t('playground.promptPlaceholder', {
                            defaultValue: 'Describe the image you want to generate…',
                        })}
                        value={prompt}
                        onChange={(event) => setPrompt(event.target.value)}
                        disabled={noModels}
                    />

                    <Box
                        sx={{
                            display: 'grid',
                            gridTemplateColumns: 'minmax(0, 1fr) minmax(0, 1fr) 88px',
                            gap: 1.5,
                        }}
                    >
                        <FormControl size="small">
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
                        <FormControl size="small">
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
                            slotProps={{ htmlInput: { min: 1, max: 10 } }}
                        />
                    </Box>

                    <Button
                        variant="contained"
                        size="large"
                        fullWidth
                        onClick={handleGenerate}
                        disabled={sending || noModels || !prompt.trim() || !model}
                        startIcon={sending ? <CircularProgress size={18} /> : <AutoAwesome />}
                    >
                        {sending
                            ? t('playground.generating', { defaultValue: 'Generating…' })
                            : t('playground.generate', { defaultValue: 'Generate' })}
                    </Button>
                </Stack>

                <Box
                    sx={{
                        minHeight: { xs: 320, md: 420 },
                        height: { xs: 'auto', md: 420 },
                        maxHeight: { xs: 560, md: 420 },
                        border: '1px solid',
                        borderColor: 'divider',
                        borderRadius: 2,
                        bgcolor: 'action.hover',
                        p: 2,
                        display: 'flex',
                        alignItems: runs.length === 0 && !sending ? 'center' : 'stretch',
                        justifyContent: runs.length === 0 && !sending ? 'center' : 'flex-start',
                        overflowY: runs.length === 0 && !sending ? 'hidden' : 'auto',
                        overflowX: 'hidden',
                    }}
                >
                    {runs.length === 0 && !sending ? (
                        <Stack alignItems="center" spacing={1} sx={{ color: 'text.secondary', textAlign: 'center' }}>
                            <Photo sx={{ fontSize: 44, opacity: 0.45 }} />
                            <Typography variant="subtitle2" color="text.secondary">
                                {t('playground.previewEmpty', { defaultValue: 'Your generated images will appear here' })}
                            </Typography>
                            <Typography variant="caption" color="text.disabled">
                                {t('playground.previewHint', { defaultValue: 'Each generation will be kept for this session.' })}
                            </Typography>
                        </Stack>
                    ) : (
                        <Stack spacing={1.5} sx={{ width: '100%' }}>
                            <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between' }}>
                                <Typography variant="subtitle2">
                                    {t('playground.sessionOutputs', { defaultValue: 'Session outputs' })}
                                </Typography>
                                <Typography variant="caption" color="text.secondary">
                                    {t('playground.runCount', {
                                        defaultValue: runs.length === 1 ? '1 generation' : '{{count}} generations',
                                        count: runs.length,
                                    })}
                                </Typography>
                            </Box>

                            {sending && (
                                <Card variant="outlined" sx={{ borderStyle: 'dashed', bgcolor: 'background.paper' }}>
                                    <CardContent sx={{ p: 1.5, '&:last-child': { pb: 1.5 } }}>
                                        <Stack direction="row" spacing={1.5} alignItems="center">
                                            <CircularProgress size={20} />
                                            <Typography variant="body2" color="text.secondary">
                                                {t('playground.generatingNew', { defaultValue: 'Generating new images…' })}
                                            </Typography>
                                        </Stack>
                                    </CardContent>
                                </Card>
                            )}

                            {runs.map((run) => (
                                <Card key={run.id} variant="outlined" sx={{ bgcolor: 'background.paper' }}>
                                    <CardContent sx={{ p: 1.5, '&:last-child': { pb: 1.5 } }}>
                                        <Stack spacing={1.25}>
                                            <Box>
                                                <Typography
                                                    variant="body2"
                                                    sx={{
                                                        fontWeight: 500,
                                                        display: '-webkit-box',
                                                        WebkitLineClamp: 2,
                                                        WebkitBoxOrient: 'vertical',
                                                        overflow: 'hidden',
                                                    }}
                                                >
                                                    {run.prompt}
                                                </Typography>
                                                <Typography variant="caption" color="text.secondary">
                                                    {run.model} · {run.size} · {run.quality}
                                                </Typography>
                                            </Box>
                                            <Box
                                                sx={{
                                                    display: 'grid',
                                                    gridTemplateColumns: run.images.length === 1
                                                        ? 'minmax(0, 1fr)'
                                                        : 'repeat(2, minmax(0, 1fr))',
                                                    gap: 1,
                                                }}
                                            >
                                                {run.images.map((image, index) => {
                                                    const src = image.url || (image.b64_json
                                                        ? `data:image/png;base64,${image.b64_json}`
                                                        : '');
                                                    return src ? (
                                                        <Box
                                                            key={`${run.id}-${index}`}
                                                            component="img"
                                                            src={src}
                                                            alt={t('playground.resultAlt', {
                                                                defaultValue: 'Generated image {{number}}',
                                                                number: index + 1,
                                                            })}
                                                            sx={{
                                                                width: '100%',
                                                                maxHeight: 360,
                                                                objectFit: 'contain',
                                                                display: 'block',
                                                                borderRadius: 1,
                                                                bgcolor: 'action.hover',
                                                            }}
                                                        />
                                                    ) : (
                                                        <Typography key={`${run.id}-${index}`} variant="caption" color="text.secondary">
                                                            {t('playground.emptyResult', { defaultValue: 'No image returned' })}
                                                        </Typography>
                                                    );
                                                })}
                                            </Box>
                                        </Stack>
                                    </CardContent>
                                </Card>
                            ))}
                        </Stack>
                    )}
                </Box>
            </Box>
        </UnifiedCard>
    );
};

export default ImageGenPlaygroundCard;
