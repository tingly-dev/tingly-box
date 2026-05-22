import { ChangeEvent, useCallback, useEffect, useMemo, useState } from 'react';
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
import { api } from '@/services/api';
import { getOpenAIClient } from '@/services/openaiClient';
import { useFunctionPanelData } from '@/hooks/useFunctionPanelData';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import CardGrid from '@/components/CardGrid';

const IMAGE_SCENARIO = 'imagegen';

type Quality = 'auto' | 'high' | 'medium' | 'low' | 'standard';


const fileToDataURL = (file: File): Promise<string> =>
    new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '');
        reader.onerror = () => reject(new Error(`Failed to read file: ${file.name}`));
        reader.readAsDataURL(file);
    });

const extractModelsFromRules = (rules: any[] | undefined | null): string[] => {
    if (!Array.isArray(rules)) return [];
    const seen = new Set<string>();
    rules.forEach((r) => {
        if (r?.disabled) return;
        const name = r?.request_model;
        if (typeof name === 'string' && name.trim()) {
            seen.add(name.trim());
        }
    });
    return Array.from(seen);
};

const PlaygroundPage: React.FC = () => {
    const { t } = useTranslation();
    const { notification, showNotification } = useFunctionPanelData();

    const [models, setModels] = useState<string[]>([]);
    const [model, setModel] = useState<string>('');
    const [prompt, setPrompt] = useState<string>('');
    const [imageRefs, setImageRefs] = useState<string>('');
    const [uploadRefs, setUploadRefs] = useState<string[]>([]);
    const [size, setSize] = useState<string>('1024x1024');
    const [quality, setQuality] = useState<Quality>('auto');
    const [count, setCount] = useState<number>(1);
    const [results, setResults] = useState<{ url?: string; b64_json?: string }[]>([]);
    const [sending, setSending] = useState(false);
    const [loadingModels, setLoadingModels] = useState(false);

    useEffect(() => {
        let cancelled = false;
        (async () => {
            setLoadingModels(true);
            const resp = await api.getRules(IMAGE_SCENARIO);
            const rules = Array.isArray(resp?.data) ? resp.data : (Array.isArray(resp) ? resp : []);
            const list = extractModelsFromRules(rules);
            if (!cancelled) {
                setModels(list);
                setModel((current) => current || list[0] || '');
                setLoadingModels(false);
            }
        })();
        return () => { cancelled = true; };
    }, []);


    const handleRefUpload = useCallback(async (e: ChangeEvent<HTMLInputElement>) => {
        const files = Array.from(e.target.files ?? []);
        if (files.length === 0) return;

        try {
            const encoded = await Promise.all(files.map((f) => fileToDataURL(f)));
            const valid = encoded.map((v) => v.trim()).filter(Boolean);
            setUploadRefs(valid);
        } catch (err: any) {
            showNotification(err?.message || 'Failed to load reference image', 'error');
        } finally {
            e.target.value = '';
        }
    }, [showNotification]);

    const handleGenerate = useCallback(async () => {
        if (!prompt.trim() || !model) return;
        setSending(true);
        setResults([]);
        try {
            const client = await getOpenAIClient(IMAGE_SCENARIO);
            const refs = imageRefs.split('\n').map((v) => v.trim()).filter(Boolean);
            const mergedRefs = [...refs, ...uploadRefs];
            const resp = await client.images.generate({
                model,
                prompt: prompt.trim(),
                n: count,
                size: size as any,
                quality,
                ...(mergedRefs.length > 0 ? ({ extra_body: { input_image_refs: mergedRefs } } as any) : {}),
            } as any);
            setResults(resp.data ?? []);
        } catch (err: any) {
            const status = err?.status ? `${err.status}: ` : '';
            const msg = err?.error?.message || err?.message || 'Request failed';
            showNotification(`${status}${msg}`, 'error');
        } finally {
            setSending(false);
        }
    }, [prompt, imageRefs, uploadRefs, model, count, size, quality, showNotification]);

    const noModels = useMemo(() => models.length === 0, [models]);

    return (
        <PageLayout loading={false} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    size="full"
                    title={t('playground.imageTitle', { defaultValue: 'Image Generation Playground' })}
                >
                    <Stack spacing={2}>
                        {noModels && !loadingModels && (
                            <Typography variant="body2" color="text.secondary">
                                {t('playground.noImageModels', {
                                    defaultValue: 'No image generation rules configured. Add one on the Image Gen page first.',
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
                                    onChange={(e) => setModel(e.target.value)}
                                    disabled={noModels}
                                >
                                    {models.map((m) => (
                                        <MenuItem key={m} value={m}>{m}</MenuItem>
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
                                    onChange={(e) => setSize(e.target.value)}
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
                                    onChange={(e) => setQuality(e.target.value as Quality)}
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
                                onChange={(e) => {
                                    const n = Number(e.target.value);
                                    setCount(Number.isFinite(n) && n > 0 ? Math.min(n, 10) : 1);
                                }}
                                sx={{ width: 100 }}
                                inputProps={{ min: 1, max: 10 }}
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
                            onChange={(e) => setPrompt(e.target.value)}
                            disabled={noModels}
                        />


                        <TextField
                            multiline
                            minRows={2}
                            fullWidth
                            placeholder={t('playground.refsPlaceholder', {
                                defaultValue: 'Optional reference image URLs, one per line…',
                            })}
                            value={imageRefs}
                            onChange={(e) => setImageRefs(e.target.value)}
                            disabled={noModels}
                        />


                        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ sm: 'center' }}>
                            <Button variant="outlined" component="label" disabled={noModels || sending}>
                                Upload reference image(s)
                                <input hidden type="file" accept="image/*" multiple onChange={handleRefUpload} />
                            </Button>
                            {uploadRefs.length > 0 && (
                                <Typography variant="body2" color="text.secondary">
                                    {uploadRefs.length} uploaded reference image(s)
                                </Typography>
                            )}
                            {uploadRefs.length > 0 && (
                                <Button size="small" onClick={() => setUploadRefs([])}>
                                    Clear uploads
                                </Button>
                            )}
                        </Stack>

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
                                {results.map((img, idx) => {
                                    const src = img.url
                                        ? img.url
                                        : img.b64_json
                                            ? `data:image/png;base64,${img.b64_json}`
                                            : '';
                                    return (
                                        <Card key={idx} variant="outlined">
                                            <CardContent sx={{ p: 1, '&:last-child': { pb: 1 } }}>
                                                {src ? (
                                                    <Box
                                                        component="img"
                                                        src={src}
                                                        alt={`result-${idx}`}
                                                        sx={{ width: '100%', display: 'block', borderRadius: 1 }}
                                                    />
                                                ) : (
                                                    <Typography variant="caption" color="text.secondary">
                                                        empty
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
            </CardGrid>
        </PageLayout>
    );
};

export default PlaygroundPage;
