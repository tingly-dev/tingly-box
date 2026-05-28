import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
    Tab,
    Tabs,
    TextField,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { tablerMui } from '@/components/icons';
import { IconPhoto, IconPhotoEdit, IconUpload, IconX } from '@tabler/icons-react';
import { api } from '@/services/api';
import { getOpenAIClient } from '@/services/openaiClient';
import { getApiBaseUrl } from '@/utils/protocol';
import { useFunctionPanelData } from '@/hooks/useFunctionPanelData';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import CardGrid from '@/components/CardGrid';

const PhotoIcon = tablerMui(IconPhoto);
const PhotoEditIcon = tablerMui(IconPhotoEdit);
const UploadIcon = tablerMui(IconUpload);
const CloseIcon = tablerMui(IconX);

const IMAGE_SCENARIO = 'imagegen';

type Mode = 'generate' | 'edit';
type Quality = 'auto' | 'high' | 'medium' | 'low' | 'standard';
type InputFidelity = 'auto' | 'high' | 'low';

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

interface ImageFile {
    file: File;
    preview: string; // data URL
}

const ImageUploadBox: React.FC<{
    value: ImageFile | null;
    onChange: (img: ImageFile | null) => void;
    label: string;
    optional?: boolean;
    accept?: string;
}> = ({ value, onChange, label, optional, accept = 'image/png,image/jpeg,image/webp' }) => {
    const inputRef = useRef<HTMLInputElement>(null);

    const handleFiles = (files: FileList | null) => {
        if (!files || files.length === 0) return;
        const file = files[0];
        const reader = new FileReader();
        reader.onload = (e) => {
            onChange({ file, preview: e.target?.result as string });
        };
        reader.readAsDataURL(file);
    };

    const handleDrop = (e: React.DragEvent) => {
        e.preventDefault();
        handleFiles(e.dataTransfer.files);
    };

    return (
        <Box>
            <Typography variant="caption" color="text.secondary" sx={{ mb: 0.5, display: 'block' }}>
                {label}{optional && <span style={{ opacity: 0.6 }}> (optional)</span>}
            </Typography>
            <Box
                onClick={() => !value && inputRef.current?.click()}
                onDrop={handleDrop}
                onDragOver={(e) => e.preventDefault()}
                sx={{
                    border: '1px dashed',
                    borderColor: 'divider',
                    borderRadius: 1,
                    p: value ? 0.5 : 2,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: value ? 'flex-start' : 'center',
                    flexDirection: value ? 'row' : 'column',
                    gap: 1,
                    cursor: value ? 'default' : 'pointer',
                    minHeight: 80,
                    bgcolor: 'action.hover',
                    '&:hover': value ? {} : { bgcolor: 'action.selected' },
                    transition: 'background-color 0.15s',
                    position: 'relative',
                    overflow: 'hidden',
                }}
            >
                {value ? (
                    <>
                        <Box
                            component="img"
                            src={value.preview}
                            sx={{ height: 64, width: 64, objectFit: 'cover', borderRadius: 0.5, flexShrink: 0 }}
                        />
                        <Box sx={{ flex: 1, minWidth: 0 }}>
                            <Typography variant="body2" noWrap title={value.file.name}>
                                {value.file.name}
                            </Typography>
                            <Typography variant="caption" color="text.secondary">
                                {(value.file.size / 1024).toFixed(0)} KB
                            </Typography>
                        </Box>
                        <Box sx={{ display: 'flex', gap: 0.5 }}>
                            <Button
                                size="small"
                                variant="outlined"
                                onClick={() => inputRef.current?.click()}
                                sx={{ minWidth: 0, px: 1 }}
                            >
                                <UploadIcon fontSize="small" />
                            </Button>
                            <Button
                                size="small"
                                variant="outlined"
                                color="error"
                                onClick={() => onChange(null)}
                                sx={{ minWidth: 0, px: 1 }}
                            >
                                <CloseIcon fontSize="small" />
                            </Button>
                        </Box>
                    </>
                ) : (
                    <>
                        <UploadIcon fontSize="medium" color="disabled" />
                        <Typography variant="body2" color="text.secondary" align="center">
                            Click or drop image here
                        </Typography>
                        <Typography variant="caption" color="text.disabled">
                            PNG · JPEG · WebP
                        </Typography>
                    </>
                )}
                <input
                    ref={inputRef}
                    type="file"
                    accept={accept}
                    style={{ display: 'none' }}
                    onChange={(e) => handleFiles(e.target.files)}
                    // reset value so re-selecting same file triggers onChange
                    onClick={(e) => { (e.target as HTMLInputElement).value = ''; }}
                />
            </Box>
        </Box>
    );
};

const PlaygroundPage: React.FC = () => {
    const { t } = useTranslation();
    const { notification, showNotification } = useFunctionPanelData();

    const [mode, setMode] = useState<Mode>('generate');
    const [models, setModels] = useState<string[]>([]);
    const [model, setModel] = useState<string>('');
    const [prompt, setPrompt] = useState<string>('');
    const [size, setSize] = useState<string>('1024x1024');
    const [quality, setQuality] = useState<Quality>('auto');
    const [inputFidelity, setInputFidelity] = useState<InputFidelity>('auto');
    const [count, setCount] = useState<number>(1);
    const [results, setResults] = useState<{ url?: string; b64_json?: string }[]>([]);
    const [sending, setSending] = useState(false);
    const [loadingModels, setLoadingModels] = useState(false);

    // Edit mode
    const [sourceImage, setSourceImage] = useState<ImageFile | null>(null);
    const [maskImage, setMaskImage] = useState<ImageFile | null>(null);

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

    const handleGenerate = useCallback(async () => {
        if (!prompt.trim() || !model) return;
        setSending(true);
        setResults([]);
        try {
            const client = await getOpenAIClient(IMAGE_SCENARIO);
            const resp = await client.images.generate({
                model,
                prompt: prompt.trim(),
                n: count,
                size: size as any,
                quality,
            });
            setResults(resp.data ?? []);
        } catch (err: any) {
            const status = err?.status ? `${err.status}: ` : '';
            const msg = err?.error?.message || err?.message || 'Request failed';
            showNotification(`${status}${msg}`, 'error');
        } finally {
            setSending(false);
        }
    }, [prompt, model, count, size, quality, showNotification]);

    const handleEdit = useCallback(async () => {
        if (!prompt.trim() || !model || !sourceImage) return;
        setSending(true);
        setResults([]);
        try {
            const base = await getApiBaseUrl();
            const tokenResult = await api.getToken();
            const apiKey = tokenResult?.token ?? '';

            const form = new FormData();
            form.append('model', model);
            form.append('prompt', prompt.trim());
            form.append('size', size);
            form.append('quality', quality);
            if (count > 1) form.append('n', String(count));
            if (inputFidelity !== 'auto') form.append('input_fidelity', inputFidelity);
            form.append('image', sourceImage.file, sourceImage.file.name);
            if (maskImage) {
                form.append('mask', maskImage.file, maskImage.file.name);
            }

            const res = await fetch(`${base}/tingly/${IMAGE_SCENARIO}/v1/images/edits`, {
                method: 'POST',
                headers: { Authorization: `Bearer ${apiKey}` },
                body: form,
            });

            if (!res.ok) {
                const errJson = await res.json().catch(() => ({}));
                const msg = errJson?.error?.message || `HTTP ${res.status}`;
                throw new Error(msg);
            }

            const data = await res.json();
            setResults(data?.data ?? []);
        } catch (err: any) {
            showNotification(err?.message || 'Edit request failed', 'error');
        } finally {
            setSending(false);
        }
    }, [prompt, model, count, size, quality, inputFidelity, sourceImage, maskImage, showNotification]);

    const canSubmit = useMemo(() => {
        if (!model || !prompt.trim() || sending) return false;
        if (mode === 'edit' && !sourceImage) return false;
        return true;
    }, [model, prompt, sending, mode, sourceImage]);

    const noModels = useMemo(() => models.length === 0, [models]);

    const handleSubmit = mode === 'generate' ? handleGenerate : handleEdit;

    return (
        <PageLayout loading={false} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    size="full"
                    title={t('playground.imageTitle', { defaultValue: 'Image Playground' })}
                >
                    <Stack spacing={2}>
                        {noModels && !loadingModels && (
                            <Typography variant="body2" color="text.secondary">
                                {t('playground.noImageModels', {
                                    defaultValue: 'No image generation rules configured. Add one on the Image Gen page first.',
                                })}
                            </Typography>
                        )}

                        <Tabs
                            value={mode}
                            onChange={(_, v) => { setMode(v); setResults([]); }}
                            sx={{ borderBottom: 1, borderColor: 'divider' }}
                        >
                            <Tab
                                value="generate"
                                label={t('playground.modeGenerate', { defaultValue: 'Generate' })}
                                icon={<PhotoIcon fontSize="small" />}
                                iconPosition="start"
                            />
                            <Tab
                                value="edit"
                                label={t('playground.modeEdit', { defaultValue: 'Edit' })}
                                icon={<PhotoEditIcon fontSize="small" />}
                                iconPosition="start"
                            />
                        </Tabs>

                        {/* Common controls row */}
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
                                    <MenuItem value="256x256">256×256</MenuItem>
                                    <MenuItem value="512x512">512×512</MenuItem>
                                    <MenuItem value="1024x1024">1024×1024</MenuItem>
                                    <MenuItem value="1024x1536">1024×1536</MenuItem>
                                    <MenuItem value="1536x1024">1536×1024</MenuItem>
                                    <MenuItem value="1024x1792">1024×1792</MenuItem>
                                    <MenuItem value="1792x1024">1792×1024</MenuItem>
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
                            {mode === 'edit' && (
                                <FormControl size="small" sx={{ minWidth: 160 }}>
                                    <InputLabel id="image-fidelity-label">
                                        {t('playground.inputFidelity', { defaultValue: 'Input Fidelity' })}
                                    </InputLabel>
                                    <Select
                                        labelId="image-fidelity-label"
                                        label={t('playground.inputFidelity', { defaultValue: 'Input Fidelity' })}
                                        value={inputFidelity}
                                        onChange={(e) => setInputFidelity(e.target.value as InputFidelity)}
                                    >
                                        <MenuItem value="auto">auto</MenuItem>
                                        <MenuItem value="low">low</MenuItem>
                                        <MenuItem value="high">high</MenuItem>
                                    </Select>
                                </FormControl>
                            )}
                            {mode === 'generate' && (
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
                            )}
                        </Stack>

                        {/* Edit mode: image uploads */}
                        {mode === 'edit' && (
                            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
                                <Box sx={{ flex: 1 }}>
                                    <ImageUploadBox
                                        value={sourceImage}
                                        onChange={setSourceImage}
                                        label={t('playground.sourceImage', { defaultValue: 'Source Image' })}
                                    />
                                </Box>
                                <Box sx={{ flex: 1 }}>
                                    <ImageUploadBox
                                        value={maskImage}
                                        onChange={setMaskImage}
                                        label={t('playground.maskImage', { defaultValue: 'Mask (inpaint region)' })}
                                        optional
                                        accept="image/png"
                                    />
                                </Box>
                            </Stack>
                        )}

                        <TextField
                            multiline
                            minRows={4}
                            fullWidth
                            placeholder={mode === 'generate'
                                ? t('playground.promptPlaceholder', { defaultValue: 'Describe the image you want to generate…' })
                                : t('playground.editPromptPlaceholder', { defaultValue: 'Describe the edits you want to make…' })
                            }
                            value={prompt}
                            onChange={(e) => setPrompt(e.target.value)}
                            disabled={noModels}
                        />

                        <Box>
                            <Button
                                variant="contained"
                                onClick={handleSubmit}
                                disabled={!canSubmit || noModels}
                                startIcon={sending ? <CircularProgress size={16} /> : undefined}
                            >
                                {sending
                                    ? t('playground.generating', { defaultValue: 'Working…' })
                                    : mode === 'generate'
                                        ? t('playground.generate', { defaultValue: 'Generate' })
                                        : t('playground.editAction', { defaultValue: 'Edit Image' })}
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
