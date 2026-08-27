import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
    Box,
    Alert,
    Button,
    ButtonBase,
    Card,
    CardContent,
    CircularProgress,
    Dialog,
    DialogContent,
    DialogTitle,
    FormControl,
    IconButton,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    TextField,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Rule } from '@/components/RoutingGraphTypes';
import UnifiedCard from '@/components/UnifiedCard';
import { AutoAwesome, Brush, Close, ContentCopy, Edit, FileUpload, Photo, ZoomIn } from '@/components/icons';
import { useCopyFeedback } from '@/hooks/useCopyFeedback';
import { getOpenAIClient } from '@/services/openaiClient';

const IMAGE_SCENARIO = 'imagegen';
// Base panel height, plus the mode toggle row present in both modes. Edit
// mode adds the reference-image dropzone on top of that (see
// desktopPanelHeight below) — both panels share one height value so they
// stay visually aligned (see the comment on the grid below).
const PLAYGROUND_PANEL_HEIGHT = 340;
const EDIT_REFERENCE_SECTION_HEIGHT = 108;
// Matches the Codex-native imagegen tool's reference-image cap (see
// .design/imageedit.md) — the common denominator across providers behind
// this scenario.
const MAX_EDIT_REFERENCE_IMAGES = 5;

type Mode = 'generate' | 'edit';
type Quality = 'auto' | 'high' | 'medium' | 'low' | 'standard';

interface ImageResult {
    url?: string;
    b64_json?: string;
}

interface ReferenceImage {
    file: File;
    previewUrl: string;
}

interface GenerationRun {
    id: string;
    mode: Mode;
    prompt: string;
    model: string;
    size: string;
    quality: Quality;
    images: ImageResult[];
    // Data URLs of the reference images an edit run was built from, kept for
    // display alongside the output — the "what did I ask for" half of the
    // history card (edit mode only).
    sourceImages?: string[];
    status?: 'pending' | 'completed';
}

// Reads a File into a base64 data URL, the same representation already used
// for generated images (`data:image/png;base64,...`) so reference thumbnails
// and outputs render through one code path.
const fileToDataUrl = (file: File): Promise<string> => new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
});

interface SelectedImage {
    src: string;
    prompt: string;
    model: string;
    size: string;
    quality: Quality;
    index: number;
    // Distinguishes the run's original reference image(s) from its generated
    // output(s) — same lightbox, different framing ("Original" badge + an
    // edit affordance for source images that otherwise have no interaction).
    kind: 'output' | 'source';
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
    const [mode, setMode] = useState<Mode>('generate');
    const [prompt, setPrompt] = useState('');
    const [size, setSize] = useState('1024x1024');
    const [quality, setQuality] = useState<Quality>('auto');
    const [count, setCount] = useState(1);
    const [referenceImages, setReferenceImages] = useState<ReferenceImage[]>([]);
    const [runs, setRuns] = useState<GenerationRun[]>(() => imageGenSessionRuns);
    const [selectedImage, setSelectedImage] = useState<SelectedImage | null>(null);
    const historyTrackRef = useRef<HTMLDivElement>(null);
    const referenceFileInputRef = useRef<HTMLInputElement>(null);
    const pendingCount = runs.filter((run) => run.status === 'pending').length;
    const { copied: promptCopied, copy: copyPrompt } = useCopyFeedback();

    const updateRuns = useCallback((updater: (currentRuns: GenerationRun[]) => GenerationRun[]) => {
        const nextRuns = updater(imageGenSessionRuns);
        imageGenSessionRuns = nextRuns;
        setRuns(nextRuns);
    }, []);

    useEffect(() => {
        const frame = requestAnimationFrame(() => {
            const track = historyTrackRef.current;
            if (track) track.scrollTo({ left: track.scrollWidth, behavior: 'smooth' });
        });
        return () => cancelAnimationFrame(frame);
    }, [pendingCount, runs.length]);

    // Appends newly picked/dropped files (image/* only) up to the reference
    // cap, converting each to a data URL up front so thumbnails and the
    // eventual run history render through the same representation.
    const handleAddReferenceImages = useCallback(async (files: FileList | File[]) => {
        const incoming = Array.from(files).filter((file) => file.type.startsWith('image/'));
        if (incoming.length === 0) return;
        const accepted = incoming.slice(0, Math.max(0, MAX_EDIT_REFERENCE_IMAGES - referenceImages.length));
        if (accepted.length === 0) return;
        const withPreviews = await Promise.all(accepted.map(async (file) => ({
            file,
            previewUrl: await fileToDataUrl(file),
        })));
        setReferenceImages((current) => [...current, ...withPreviews].slice(0, MAX_EDIT_REFERENCE_IMAGES));
    }, [referenceImages.length]);

    const handleRemoveReferenceImage = useCallback((index: number) => {
        setReferenceImages((current) => current.filter((_, i) => i !== index));
    }, []);

    // Pasting an image anywhere on the panel is itself the mode signal — the
    // user doesn't have to switch to Edit mode first and then find the
    // dropzone. A paste with no image (e.g. plain text into the prompt field)
    // is left alone. Scoped to this card's own DOM subtree via the React
    // synthetic paste event, not a window-level listener.
    const handlePaste = useCallback((event: React.ClipboardEvent) => {
        const items = event.clipboardData?.items;
        if (!items) return;
        const imageFiles = Array.from(items)
            .filter((item) => item.kind === 'file' && item.type.startsWith('image/'))
            .map((item) => item.getAsFile())
            .filter((file): file is File => file !== null);
        if (imageFiles.length === 0) return;
        event.preventDefault();
        setMode('edit');
        void handleAddReferenceImages(imageFiles);
    }, [handleAddReferenceImages]);

    // Hands a completed output straight back in as the next edit's source —
    // the artifact for the next action, not just a notification that one
    // exists. Reuses the already-rendered src as the preview (it's already a
    // data URL/data-equivalent), so this never re-encodes the image.
    const handleUseAsReference = useCallback(async (src: string) => {
        try {
            const res = await fetch(src);
            const blob = await res.blob();
            const file = new File([blob], `reference-${Date.now()}.png`, { type: blob.type || 'image/png' });
            setMode('edit');
            setReferenceImages([{ file, previewUrl: src }]);
        } catch {
            showNotification(
                t('playground.referenceLoadFailed', { defaultValue: 'Could not use this image as a reference' }),
                'error',
            );
        }
    }, [showNotification, t]);

    const handleSubmit = useCallback(async () => {
        if (!prompt.trim() || !model) return;
        if (mode === 'edit' && referenceImages.length === 0) return;
        const runId = `${Date.now()}-${Math.random().toString(36).slice(2)}`;
        const runMode = mode;
        const generationPrompt = prompt.trim();
        const generationModel = model;
        const generationSize = size;
        const generationQuality = quality;
        const editSources = referenceImages;
        updateRuns((currentRuns) => [...currentRuns, {
            id: runId,
            mode: runMode,
            prompt: generationPrompt,
            model: generationModel,
            size: generationSize,
            quality: generationQuality,
            images: [],
            sourceImages: runMode === 'edit' ? editSources.map((ref) => ref.previewUrl) : undefined,
            status: 'pending',
        }]);
        try {
            const client = await getOpenAIClient(IMAGE_SCENARIO);
            const editFiles = editSources.map((ref) => ref.file);
            const response = runMode === 'edit'
                ? await client.images.edit({
                    image: editFiles.length === 1 ? editFiles[0] : editFiles,
                    model: generationModel,
                    prompt: generationPrompt,
                    n: count,
                    size: generationSize as any,
                    quality: generationQuality as any,
                })
                : await client.images.generate({
                    model: generationModel,
                    prompt: generationPrompt,
                    n: count,
                    size: generationSize as any,
                    quality: generationQuality,
                });
            const images = response.data ?? [];
            updateRuns((currentRuns) => currentRuns.map((run) => (
                run.id === runId ? { ...run, images, status: 'completed' } : run
            )));
        } catch (error: any) {
            updateRuns((currentRuns) => currentRuns.filter((run) => run.id !== runId));
            const status = error?.status ? `${error.status}: ` : '';
            const message = error?.error?.message || error?.message || t('playground.requestFailed', { defaultValue: 'Request failed' });
            showNotification(`${status}${message}`, 'error');
        }
    }, [count, mode, model, prompt, quality, referenceImages, showNotification, size, t, updateRuns]);

    const noModels = models.length === 0;
    const desktopPanelHeight = noModels && !loadingRules
        ? 'auto'
        : PLAYGROUND_PANEL_HEIGHT + (mode === 'edit' ? EDIT_REFERENCE_SECTION_HEIGHT : 0);

    return (
        <>
            <UnifiedCard
                size="full"
                title={t('playground.imageTitle', { defaultValue: 'Image Playground' })}
            >
                <Box
                    onPaste={handlePaste}
                    sx={{
                        display: 'grid',
                        gridTemplateColumns: { xs: '1fr', lg: 'minmax(360px, 0.9fr) minmax(420px, 1.1fr)' },
                        gap: 3,
                        // Both desktop panels consume the same height token below.
                        // Do not introduce panel-specific desktop heights: generated
                        // image content otherwise makes the two sides drift apart.
                        alignItems: 'stretch',
                    }}
                >
                    <Stack
                        data-testid="imagegen-controls-panel"
                        spacing={2}
                        sx={{
                            minWidth: 0,
                            height: { xs: 'auto', lg: desktopPanelHeight },
                        }}
                    >
                        {noModels && !loadingRules && (
                            <Alert severity="info" variant="outlined">
                                {t('playground.noImageModels', {
                                    defaultValue: 'Add an image generation model rule below to start generating images.',
                                })}
                            </Alert>
                        )}

                        <ToggleButtonGroup
                            value={mode}
                            exclusive
                            size="small"
                            onChange={(_, next: Mode | null) => { if (next) setMode(next); }}
                            disabled={noModels}
                            fullWidth
                        >
                            <ToggleButton value="generate">
                                <AutoAwesome fontSize="small" sx={{ mr: 0.75 }} />
                                {t('playground.modeGenerate', { defaultValue: 'Generate' })}
                            </ToggleButton>
                            <ToggleButton value="edit">
                                <Brush fontSize="small" sx={{ mr: 0.75 }} />
                                {t('playground.modeEdit', { defaultValue: 'Edit' })}
                            </ToggleButton>
                        </ToggleButtonGroup>

                        {mode === 'edit' && (
                            <Box>
                                <Typography variant="caption" sx={{ display: 'block', mb: 0.5, color: 'text.secondary' }}>
                                    {t('playground.referenceImages', { defaultValue: 'Reference images' })}
                                </Typography>
                                <Box
                                    onClick={() => referenceFileInputRef.current?.click()}
                                    onDragOver={(event) => event.preventDefault()}
                                    onDrop={(event) => {
                                        event.preventDefault();
                                        if (event.dataTransfer.files?.length) {
                                            void handleAddReferenceImages(event.dataTransfer.files);
                                        }
                                    }}
                                    sx={{
                                        display: 'flex',
                                        flexWrap: 'wrap',
                                        gap: 1,
                                        p: 1,
                                        border: '1px dashed',
                                        borderColor: 'divider',
                                        borderRadius: 1.5,
                                        bgcolor: 'action.hover',
                                        minHeight: 64,
                                        alignItems: 'center',
                                        cursor: referenceImages.length < MAX_EDIT_REFERENCE_IMAGES ? 'pointer' : 'default',
                                    }}
                                >
                                    {referenceImages.length === 0 ? (
                                        <Stack direction="row" spacing={1} sx={{ width: '100%', alignItems: 'center', justifyContent: 'center', color: 'text.secondary', py: 0.5 }}>
                                            <FileUpload sx={{ fontSize: 20 }} />
                                            <Typography variant="body2">
                                                {t('playground.dropReferenceImage', { defaultValue: 'Drop images here, click to browse, or paste' })}
                                            </Typography>
                                        </Stack>
                                    ) : (
                                        <>
                                            {referenceImages.map((ref, index) => (
                                                <Box
                                                    key={index}
                                                    sx={{ position: 'relative', width: 56, height: 56, borderRadius: 1, overflow: 'hidden', flexShrink: 0 }}
                                                >
                                                    <Box
                                                        component="img"
                                                        src={ref.previewUrl}
                                                        alt={t('playground.referenceThumbAlt', { defaultValue: 'Reference image {{number}}', number: index + 1 })}
                                                        sx={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
                                                    />
                                                    <IconButton
                                                        size="small"
                                                        onClick={(event) => { event.stopPropagation(); handleRemoveReferenceImage(index); }}
                                                        aria-label={t('playground.removeReferenceImage', { defaultValue: 'Remove reference image {{number}}', number: index + 1 })}
                                                        sx={{
                                                            position: 'absolute',
                                                            top: -6,
                                                            right: -6,
                                                            width: 20,
                                                            height: 20,
                                                            bgcolor: 'rgba(15, 23, 42, 0.7)',
                                                            color: 'common.white',
                                                            '&:hover': { bgcolor: 'rgba(15, 23, 42, 0.9)' },
                                                        }}
                                                    >
                                                        <Close sx={{ fontSize: 14 }} />
                                                    </IconButton>
                                                </Box>
                                            ))}
                                            {referenceImages.length < MAX_EDIT_REFERENCE_IMAGES && (
                                                <Stack
                                                    aria-label={t('playground.addReferenceImage', { defaultValue: 'Add image' })}
                                                    sx={{
                                                        width: 56,
                                                        height: 56,
                                                        alignItems: 'center',
                                                        justifyContent: 'center',
                                                        borderRadius: 1,
                                                        color: 'text.secondary',
                                                        border: '1px solid',
                                                        borderColor: 'divider',
                                                    }}
                                                >
                                                    <FileUpload sx={{ fontSize: 20 }} />
                                                </Stack>
                                            )}
                                        </>
                                    )}
                                </Box>
                                <Typography variant="caption" sx={{ display: 'block', mt: 0.5, color: 'text.disabled' }}>
                                    {t('playground.referenceHint', {
                                        defaultValue: 'Up to {{max}} images · PNG, JPEG, or WebP',
                                        max: MAX_EDIT_REFERENCE_IMAGES,
                                    })}
                                </Typography>
                                <input
                                    ref={referenceFileInputRef}
                                    type="file"
                                    accept="image/png,image/jpeg,image/webp"
                                    multiple
                                    hidden
                                    onChange={(event) => {
                                        if (event.target.files?.length) void handleAddReferenceImages(event.target.files);
                                        event.target.value = '';
                                    }}
                                />
                            </Box>
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
                            rows={5}
                            fullWidth
                            label={t('playground.prompt', { defaultValue: 'Prompt' })}
                            placeholder={mode === 'edit'
                                ? t('playground.editPromptPlaceholder', { defaultValue: 'Describe the change you want to make…' })
                                : t('playground.promptPlaceholder', { defaultValue: 'Describe the image you want to generate…' })}
                            value={prompt}
                            onChange={(event) => setPrompt(event.target.value)}
                            disabled={noModels}
                            sx={{
                                minHeight: 0,
                                '& .MuiInputBase-root': {
                                    minHeight: 0,
                                    alignItems: 'flex-start',
                                    overflow: 'hidden',
                                },
                                '& .MuiInputBase-inputMultiline': {
                                    maxHeight: '100%',
                                    boxSizing: 'border-box',
                                    overflowY: 'auto !important',
                                    overscrollBehavior: 'contain',
                                    resize: 'none',
                                    scrollbarWidth: 'thin',
                                },
                            }}
                        />

                        <Box
                            sx={{
                                display: 'grid',
                                gridTemplateColumns: {
                                    xs: 'repeat(2, minmax(0, 1fr))',
                                    sm: 'minmax(0, 1fr) minmax(0, 1fr) 88px',
                                },
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
                                sx={{ gridColumn: { xs: '1 / -1', sm: 'auto' } }}
                            />
                        </Box>

                        <Button
                            variant="contained"
                            size="large"
                            fullWidth
                            onClick={handleSubmit}
                            disabled={noModels || !prompt.trim() || !model || (mode === 'edit' && referenceImages.length === 0)}
                            startIcon={pendingCount > 0
                                ? <CircularProgress size={18} color="inherit" />
                                : (mode === 'edit' ? <Brush /> : <AutoAwesome />)}
                            sx={{
                                '&.Mui-disabled': {
                                    color: 'common.white',
                                },
                            }}
                        >
                            {mode === 'edit'
                                ? (pendingCount > 0
                                    ? t('playground.editAnother', { defaultValue: 'Edit another · {{count}} running', count: pendingCount })
                                    : t('playground.edit', { defaultValue: 'Edit Image' }))
                                : (pendingCount > 0
                                    ? t('playground.generateAnother', { defaultValue: 'Generate another · {{count}} running', count: pendingCount })
                                    : t('playground.generate', { defaultValue: 'Generate' }))}
                        </Button>
                    </Stack>

                    <Box
                        data-testid="imagegen-preview-panel"
                        sx={{
                            minWidth: 0,
                            minHeight: 0,
                            height: { xs: 320, lg: desktopPanelHeight },
                            border: '1px solid',
                            borderColor: 'divider',
                            borderRadius: 2,
                            bgcolor: 'action.hover',
                            p: 2,
                            display: 'flex',
                            alignItems: runs.length === 0 ? 'center' : 'stretch',
                            justifyContent: runs.length === 0 ? 'center' : 'flex-start',
                            overflow: 'hidden',
                        }}
                    >
                        {runs.length === 0 ? (
                            <Stack
                                spacing={1}
                                sx={{
                                    alignItems: "center",
                                    color: 'text.secondary',
                                    textAlign: 'center'
                                }}>
                                <Photo sx={{ fontSize: 44, opacity: 0.45 }} />
                                <Typography variant="subtitle2" sx={{
                                    color: "text.secondary"
                                }}>
                                    {t('playground.previewEmpty', { defaultValue: 'Your generated images will appear here' })}
                                </Typography>
                                <Typography variant="caption" sx={{
                                    color: "text.disabled"
                                }}>
                                    {t('playground.previewHint', { defaultValue: 'Each generation will be kept for this session.' })}
                                </Typography>
                            </Stack>
                        ) : (
                            <Stack spacing={1.5} sx={{ width: '100%', minWidth: 0, height: '100%' }}>
                                <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 1 }}>
                                    <Typography variant="subtitle2" sx={{ minWidth: 0 }}>
                                        {t('playground.sessionOutputs', { defaultValue: 'Session outputs' })}
                                    </Typography>
                                    <Typography
                                        variant="caption"
                                        sx={{
                                            color: "text.secondary",
                                            flexShrink: 0,
                                            whiteSpace: 'nowrap',
                                        }}
                                    >
                                        {runs.length === 1
                                            ? t('playground.runCountOne', { defaultValue: '1 generation' })
                                            : t('playground.runCount', {
                                                defaultValue: '{{count}} generations',
                                                count: runs.length,
                                            })}
                                    </Typography>
                                </Box>

                                <Box
                                    ref={historyTrackRef}
                                    data-testid="imagegen-history-track"
                                    sx={{
                                        display: 'flex',
                                        gap: 1.5,
                                        flex: 1,
                                        minHeight: 0,
                                        overflowX: 'auto',
                                        overflowY: 'hidden',
                                        pb: 0.5,
                                        scrollSnapType: 'x proximity',
                                        overflowAnchor: 'none',
                                        scrollbarWidth: 'thin',
                                        '&::-webkit-scrollbar': { height: 6 },
                                        '&::-webkit-scrollbar-thumb': { bgcolor: 'action.selected', borderRadius: 3 },
                                    }}
                                >
                                    {runs.map((run) => (
                                        <Card
                                            key={run.id}
                                            data-testid="imagegen-generation-run"
                                            data-generation-status={run.status ?? 'completed'}
                                            variant="outlined"
                                            sx={{
                                                flex: { xs: '0 0 min(82vw, 320px)', md: '0 0 clamp(280px, 46%, 360px)' },
                                                height: '100%',
                                                bgcolor: 'background.paper',
                                                borderStyle: run.status === 'pending' ? 'dashed' : 'solid',
                                                scrollSnapAlign: 'start',
                                            }}
                                        >
                                        <CardContent sx={{ p: 1.5, height: '100%', '&:last-child': { pb: 1.5 } }}>
                                            {run.status === 'pending' ? (
                                                <Stack
                                                    spacing={1.25}
                                                    aria-live="polite"
                                                    sx={{
                                                        alignItems: 'center',
                                                        justifyContent: 'center',
                                                        height: '100%',
                                                        minWidth: 0,
                                                        textAlign: 'center',
                                                    }}
                                                >
                                                    <CircularProgress size={24} />
                                                    <Typography variant="body2" sx={{ fontWeight: 500 }}>
                                                        {run.mode === 'edit'
                                                            ? t('playground.editingNew', { defaultValue: 'Editing images…' })
                                                            : t('playground.generatingNew', { defaultValue: 'Generating new images…' })}
                                                    </Typography>
                                                    <Typography
                                                        variant="caption"
                                                        sx={{
                                                            width: '100%',
                                                            color: 'text.secondary',
                                                            display: '-webkit-box',
                                                            WebkitLineClamp: 2,
                                                            WebkitBoxOrient: 'vertical',
                                                            overflow: 'hidden',
                                                        }}
                                                    >
                                                        {run.prompt}
                                                    </Typography>
                                                    <Typography
                                                        variant="caption"
                                                        sx={{
                                                            width: '100%',
                                                            color: 'text.disabled',
                                                            overflow: 'hidden',
                                                            textOverflow: 'ellipsis',
                                                            whiteSpace: 'nowrap',
                                                        }}
                                                    >
                                                        {run.model} · {run.size} · {run.quality}
                                                    </Typography>
                                                </Stack>
                                            ) : (
                                            <Stack spacing={1.25} sx={{ height: '100%' }}>
                                                <Box sx={{ minWidth: 0 }}>
                                                    <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 0.75 }}>
                                                        {run.mode === 'edit' && (
                                                            <Typography
                                                                component="span"
                                                                variant="caption"
                                                                sx={{
                                                                    flexShrink: 0,
                                                                    px: 0.75,
                                                                    borderRadius: 1,
                                                                    bgcolor: 'action.selected',
                                                                    color: 'text.secondary',
                                                                    fontWeight: 600,
                                                                }}
                                                            >
                                                                {t('playground.editBadge', { defaultValue: 'Edited' })}
                                                            </Typography>
                                                        )}
                                                        <Typography
                                                            variant="body2"
                                                            sx={{
                                                                fontWeight: 500,
                                                                minWidth: 0,
                                                                display: '-webkit-box',
                                                                WebkitLineClamp: 2,
                                                                WebkitBoxOrient: 'vertical',
                                                                overflow: 'hidden',
                                                            }}
                                                        >
                                                            {run.prompt}
                                                        </Typography>
                                                    </Box>
                                                    <Typography
                                                        variant="caption"
                                                        sx={{
                                                            display: 'block',
                                                            color: "text.secondary",
                                                            overflow: 'hidden',
                                                            textOverflow: 'ellipsis',
                                                            whiteSpace: 'nowrap',
                                                        }}
                                                    >
                                                        {run.model} · {run.size} · {run.quality}
                                                    </Typography>
                                                    {run.mode === 'edit' && run.sourceImages && run.sourceImages.length > 0 && (
                                                        <Stack direction="row" spacing={0.5} sx={{ mt: 0.75, overflowX: 'auto' }}>
                                                            {run.sourceImages.map((src, i) => (
                                                                <ButtonBase
                                                                    key={i}
                                                                    onClick={() => setSelectedImage({
                                                                        src,
                                                                        prompt: run.prompt,
                                                                        model: run.model,
                                                                        size: run.size,
                                                                        quality: run.quality,
                                                                        index: i,
                                                                        kind: 'source',
                                                                    })}
                                                                    aria-label={t('playground.viewSourceImage', {
                                                                        defaultValue: 'View original image {{number}}',
                                                                        number: i + 1,
                                                                    })}
                                                                    sx={{
                                                                        display: 'block',
                                                                        width: 28,
                                                                        height: 28,
                                                                        borderRadius: 0.5,
                                                                        overflow: 'hidden',
                                                                        flexShrink: 0,
                                                                        border: '1px solid',
                                                                        borderColor: 'divider',
                                                                    }}
                                                                >
                                                                    <Box
                                                                        component="img"
                                                                        src={src}
                                                                        alt={t('playground.referenceThumbAlt', { defaultValue: 'Reference image {{number}}', number: i + 1 })}
                                                                        sx={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
                                                                    />
                                                                </ButtonBase>
                                                            ))}
                                                        </Stack>
                                                    )}
                                                </Box>
                                                <Box
                                                    sx={{
                                                        display: 'grid',
                                                        gridTemplateColumns: run.images.length === 1
                                                            ? 'minmax(0, 1fr)'
                                                            : 'repeat(2, minmax(0, 1fr))',
                                                        flex: 1,
                                                        minHeight: 0,
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
                                                                sx={{ position: 'relative', width: '100%', height: '100%', borderRadius: 1, overflow: 'hidden', bgcolor: 'action.hover' }}
                                                            >
                                                                <ButtonBase
                                                                    onClick={() => setSelectedImage({
                                                                        src,
                                                                        prompt: run.prompt,
                                                                        model: run.model,
                                                                        size: run.size,
                                                                        quality: run.quality,
                                                                        index,
                                                                        kind: 'output',
                                                                    })}
                                                                    aria-label={t('playground.openResult', {
                                                                        defaultValue: 'Open generated image {{number}}',
                                                                        number: index + 1,
                                                                    })}
                                                                    sx={{
                                                                        width: '100%',
                                                                        height: '100%',
                                                                        display: 'block',
                                                                        '&:hover .image-preview-overlay, &:focus-visible .image-preview-overlay': {
                                                                            opacity: 1,
                                                                        },
                                                                    }}
                                                                >
                                                                    <Box
                                                                        component="img"
                                                                        src={src}
                                                                        alt={t('playground.resultAlt', {
                                                                            defaultValue: 'Generated image {{number}}',
                                                                            number: index + 1,
                                                                        })}
                                                                        sx={{
                                                                            width: '100%',
                                                                            height: '100%',
                                                                            maxHeight: '100%',
                                                                            objectFit: 'contain',
                                                                            display: 'block',
                                                                        }}
                                                                    />
                                                                    <Box
                                                                        className="image-preview-overlay"
                                                                        sx={{
                                                                            position: 'absolute',
                                                                            inset: 0,
                                                                            display: 'flex',
                                                                            alignItems: 'center',
                                                                            justifyContent: 'center',
                                                                            color: 'common.white',
                                                                            bgcolor: 'rgba(15, 23, 42, 0.38)',
                                                                            opacity: 0,
                                                                            transition: 'opacity 0.16s ease-out',
                                                                        }}
                                                                    >
                                                                        <ZoomIn sx={{ fontSize: 30 }} />
                                                                    </Box>
                                                                    <Box
                                                                        sx={{
                                                                            position: 'absolute',
                                                                            top: 8,
                                                                            right: 8,
                                                                            width: 30,
                                                                            height: 30,
                                                                            display: 'flex',
                                                                            alignItems: 'center',
                                                                            justifyContent: 'center',
                                                                            borderRadius: '50%',
                                                                            color: 'common.white',
                                                                            bgcolor: 'rgba(15, 23, 42, 0.58)',
                                                                            backdropFilter: 'blur(4px)',
                                                                        }}
                                                                    >
                                                                        <ZoomIn fontSize="small" />
                                                                    </Box>
                                                                </ButtonBase>
                                                                <IconButton
                                                                    size="small"
                                                                    onClick={(event) => { event.stopPropagation(); void handleUseAsReference(src); }}
                                                                    aria-label={t('playground.useAsReference', { defaultValue: 'Edit this image' })}
                                                                    sx={{
                                                                        position: 'absolute',
                                                                        bottom: 8,
                                                                        right: 8,
                                                                        width: 30,
                                                                        height: 30,
                                                                        color: 'common.white',
                                                                        bgcolor: 'rgba(15, 23, 42, 0.58)',
                                                                        backdropFilter: 'blur(4px)',
                                                                        '&:hover': { bgcolor: 'rgba(15, 23, 42, 0.78)' },
                                                                    }}
                                                                >
                                                                    <Edit fontSize="small" />
                                                                </IconButton>
                                                            </Box>
                                                        ) : (
                                                            <Typography key={`${run.id}-${index}`} variant="caption" sx={{
                                                                color: "text.secondary"
                                                            }}>
                                                                {t('playground.emptyResult', { defaultValue: 'No image returned' })}
                                                            </Typography>
                                                        );
                                                    })}
                                                </Box>
                                            </Stack>
                                            )}
                                        </CardContent>
                                        </Card>
                                    ))}
                                </Box>
                            </Stack>
                        )}
                    </Box>
                </Box>
            </UnifiedCard>
            <Dialog
                open={selectedImage !== null}
                onClose={() => setSelectedImage(null)}
                maxWidth={false}
                fullWidth
                slotProps={{
                    paper: {
                        sx: {
                            width: { xs: 'calc(100vw - 16px)', sm: 'calc(100vw - 48px)' },
                            height: { xs: 'calc(100dvh - 16px)', sm: 'calc(100dvh - 48px)' },
                            maxWidth: 'none',
                            maxHeight: 'none',
                            m: { xs: 1, sm: 3 },
                            borderRadius: 3,
                            bgcolor: 'grey.900',
                            color: 'common.white',
                            overflow: 'hidden',
                        },
                    }
                }}
            >
                <DialogTitle
                    sx={{
                        py: 1.5,
                        px: 2,
                        display: 'flex',
                        alignItems: 'center',
                        gap: 2,
                        bgcolor: 'rgba(15, 23, 42, 0.96)',
                        color: 'common.white',
                    }}
                >
                    <Box sx={{ minWidth: 0, flex: 1 }}>
                        <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 0.75 }}>
                            {selectedImage?.kind === 'source' && (
                                <Typography
                                    component="span"
                                    variant="caption"
                                    sx={{
                                        flexShrink: 0,
                                        px: 0.75,
                                        borderRadius: 1,
                                        bgcolor: 'rgba(255, 255, 255, 0.14)',
                                        color: 'grey.200',
                                        fontWeight: 600,
                                    }}
                                >
                                    {t('playground.originalBadge', { defaultValue: 'Original' })}
                                </Typography>
                            )}
                            <Typography
                                component="span"
                                variant="subtitle1"
                                sx={{ fontWeight: 600, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                            >
                                {selectedImage?.prompt}
                            </Typography>
                        </Box>
                        <Typography
                            component="span"
                            variant="caption"
                            sx={{
                                display: 'block',
                                color: 'grey.400',
                                overflow: 'hidden',
                                textOverflow: 'ellipsis',
                                whiteSpace: 'nowrap',
                            }}
                        >
                            {selectedImage?.model} · {selectedImage?.size} · {selectedImage?.quality}
                        </Typography>
                    </Box>
                    <Stack direction="row" spacing={0.75} sx={{ flexShrink: 0 }}>
                        <Tooltip
                            title={promptCopied
                                ? t('playground.promptCopied', { defaultValue: 'Copied' })
                                : t('playground.copyPrompt', { defaultValue: 'Copy prompt' })}
                            open={promptCopied || undefined}
                            disableHoverListener={promptCopied}
                        >
                            <IconButton
                                onClick={() => { if (selectedImage) copyPrompt(selectedImage.prompt); }}
                                aria-label={t('playground.copyPrompt', { defaultValue: 'Copy prompt' })}
                                sx={{
                                    color: 'common.white',
                                    bgcolor: 'rgba(255, 255, 255, 0.08)',
                                    '&:hover': { bgcolor: 'rgba(255, 255, 255, 0.16)' },
                                }}
                            >
                                <ContentCopy fontSize="small" />
                            </IconButton>
                        </Tooltip>
                        <Tooltip title={t('playground.useAsReference', { defaultValue: 'Edit this image' })}>
                            <IconButton
                                onClick={() => {
                                    if (!selectedImage) return;
                                    void handleUseAsReference(selectedImage.src);
                                    setSelectedImage(null);
                                }}
                                aria-label={t('playground.useAsReference', { defaultValue: 'Edit this image' })}
                                sx={{
                                    color: 'common.white',
                                    bgcolor: 'rgba(255, 255, 255, 0.08)',
                                    '&:hover': { bgcolor: 'rgba(255, 255, 255, 0.16)' },
                                }}
                            >
                                <Edit fontSize="small" />
                            </IconButton>
                        </Tooltip>
                        <IconButton
                            onClick={() => setSelectedImage(null)}
                            aria-label={t('playground.closePreview', { defaultValue: 'Close image preview' })}
                            sx={{
                                color: 'common.white',
                                bgcolor: 'rgba(255, 255, 255, 0.08)',
                                '&:hover': { bgcolor: 'rgba(255, 255, 255, 0.16)' },
                            }}
                        >
                            <Close />
                        </IconButton>
                    </Stack>
                </DialogTitle>
                <DialogContent
                    sx={{
                        p: 2,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        bgcolor: 'common.black',
                        overflow: 'hidden',
                    }}
                >
                    {selectedImage && (
                        <Box
                            component="img"
                            src={selectedImage.src}
                            alt={selectedImage.kind === 'source'
                                ? t('playground.referenceThumbAlt', { defaultValue: 'Reference image {{number}}', number: selectedImage.index + 1 })
                                : t('playground.resultAlt', { defaultValue: 'Generated image {{number}}', number: selectedImage.index + 1 })}
                            sx={{
                                display: 'block',
                                maxWidth: '100%',
                                maxHeight: '100%',
                                objectFit: 'contain',
                            }}
                        />
                    )}
                </DialogContent>
            </Dialog>
        </>
    );
};

export default ImageGenPlaygroundCard;
