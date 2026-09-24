import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControl,
    IconButton,
    InputAdornment,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import type { Rule } from '@/components/RoutingGraphTypes';
import UnifiedCard from '@/components/UnifiedCard';
import ConfirmDialog from '@/components/ConfirmDialog';
import { CopyIconButton } from '@/components/CopyIconButton';
import { AutoAwesome, Close, Description, OpenInFull } from '@/components/icons';
import { useCopyFeedback } from '@/hooks/useCopyFeedback';
import { fontMono } from '@/theme/fonts';
import { api } from '@/services/api';
import { downloadImage, slugify } from '@/utils/download';
import { isPromptFile, partitionDroppedFiles, readPromptFile } from '@/utils/promptFile';
import ImageSliceDialog from './ImageSliceDialog';
import ImageGenGalleryDialog from './ImageGenGalleryDialog';
import ImageGenLightbox from './ImageGenLightbox';
import ImageGenResultsPanel from './ImageGenResultsPanel';
import { MAX_EDIT_REFERENCE_IMAGES, ReferenceImagesRow } from './ImageGenReferenceImages';
import { useImageGenRefs } from './useImageGenRefs';
import { useImageGenRuns } from './useImageGenRuns';
import { useImageGenLightbox } from './useImageGenLightbox';
import { downloadStem, formatBytes, runImage } from './imageGenSession';
import MaskEditorDialog from './MaskEditorDialog';
import SketchCanvasDialog from './SketchCanvasDialog';
import type {
    GenerationRun,
    ImportedImage,
    Quality,
    SelectedImage,
} from './ImageGenPlayground.types';

// Base panel height with the reference-image row in its compact (empty)
// state. Once references are added the row grows into a thumbnail strip
// (see desktopPanelHeight below) — both panels share one height value so
// they stay visually aligned (see the comment on the grid below).
const PLAYGROUND_PANEL_HEIGHT = 348;
const REFERENCE_STRIP_EXTRA_HEIGHT = 48;
// The results strip is "what's happening right now", not a scrollback buffer —
// dragging through dozens of past generations to find one belongs in the
// overview (searchable, grid, newest first), not here. Capping the strip to
// its most recent items and handing off anything older to a single "open the
// overview" tile keeps the strip a status readout instead of a second archive.
const HISTORY_STRIP_VISIBLE = 6;

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
    const navigate = useNavigate();
    const models = useMemo(() => {
        const names = rules
            .filter((rule) => rule.active !== false && rule.request_model?.trim())
            .map((rule) => rule.request_model.trim());
        return Array.from(new Set(names));
    }, [rules]);

    const [selectedModel, setSelectedModel] = useState('');
    const model = models.includes(selectedModel) ? selectedModel : (models[0] ?? '');
    const [prompt, setPrompt] = useState('');
    // The prompt in a dialog-sized editor: the panel's field is one column of a
    // fixed-height panel, which is the wrong place to read or rework a long one.
    const [promptEditorOpen, setPromptEditorOpen] = useState(false);
    const [size, setSize] = useState('1024x1024');
    const [quality, setQuality] = useState<Quality>('auto');
    const [count, setCount] = useState(1);
    // Where generated images land on disk — read-only, shown so the user can
    // navigate there themselves; this page never opens it for them.
    const [outputDir, setOutputDir] = useState('');
    useEffect(() => {
        let cancelled = false;
        void api.getImageGenInfo().then((result) => {
            if (!cancelled && result?.success) setOutputDir(result.output_dir ?? '');
        });
        return () => { cancelled = true; };
    }, []);

    const {
        runs,
        imported,
        pendingCount,
        historyTrackRef,
        inFlightRef,
        updateRuns,
        updateImports,
        runGeneration,
        handleCancelRun,
        handleRetry,
        runSourcesToReferences,
        handleImportImages,
        removeRun,
        removeImport,
    } = useImageGenRuns(showNotification);
    const {
        referenceImages,
        setReferenceImages,
        draggingReference,
        setDraggingReference,
        dragOverReference,
        setDragOverReference,
        handleAddReferenceImages,
        handleRemoveReferenceImage,
        handleReorderReference,
        handleReferenceKeyDown,
        handleUseAsReference,
        sketchTarget,
        setSketchTarget,
        handleOpenSketch,
        handleSketchSubmit,
        sketchInitial,
        hasSketchReference,
        maskTarget,
        setMaskTarget,
        handleMaskSubmit,
        handleRemoveMask,
        maskInitial,
        maskedReference,
        hasMaskedReference,
    } = useImageGenRefs({ showNotification, size });
    // A brought-in image is a first-class image on this panel, not just a
    // request parameter: it opens in the same lightbox as a result, with the
    // same download and slicing tools. Its header names the file and its real
    // pixel size, since there is no prompt or model behind it.
    const referenceSelection = useCallback((index: number): SelectedImage | null => {
        const ref = referenceImages[index];
        if (!ref) return null;
        const dimensions = ref.width && ref.height ? `${ref.width}×${ref.height} px` : '';
        const kilobytes = `${Math.max(1, Math.round(ref.file.size / 1024))} KB`;
        return {
            src: ref.previewUrl,
            prompt: '',
            model: '',
            size: '',
            quality: 'auto',
            index,
            kind: 'reference',
            label: ref.source === 'sketch'
                ? t('playground.sketch.title', { defaultValue: 'Sketch' })
                : ref.file.name,
            caption: [dimensions, kilobytes].filter(Boolean).join(' · '),
            ...(ref.mask ? { maskSrc: ref.mask.previewUrl } : {}),
        };
    }, [referenceImages, t]);
    const lightboxReferences = useMemo(() => ({
        srcs: referenceImages.map((ref) => ref.previewUrl),
        select: referenceSelection,
    }), [referenceImages, referenceSelection]);
    const {
        selectedImage,
        setSelectedImage,
        lightboxRun,
        lightboxFilm,
        showLightboxFrame,
        handleLightboxKeyDown,
    } = useImageGenLightbox(runs, lightboxReferences);

    // Reusing a run refills this panel; on a narrow layout it sits above the
    // results strip and off-screen, so the refilled form is scrolled back
    // into view instead of leaving the user to wonder where the request went.
    const controlsPanelRef = useRef<HTMLDivElement>(null);
    const promptFileInputRef = useRef<HTMLInputElement>(null);
    const { copied: promptCopied, copy: copyPrompt } = useCopyFeedback();

    const handleOpenImport = useCallback((item: ImportedImage) => {
        setSelectedImage({
            src: item.src,
            prompt: '',
            model: '',
            size: '',
            quality: 'auto',
            index: 0,
            kind: 'import',
            label: item.name,
            caption: [
                item.width && item.height ? `${item.width}×${item.height} px` : '',
                formatBytes(item.bytes),
            ].filter(Boolean).join(' · '),
        });
    }, [setSelectedImage]);

    // Deleting is one-way — the image leaves the session for good — so every
    // path into it (the strip's own button, the gallery tile's button) goes
    // through the same confirm below rather than firing immediately.
    const [pendingRemoval, setPendingRemoval] = useState<{ kind: 'run' | 'import'; id: string } | null>(null);
    // The overview: the strip answers "what is happening now", this answers
    // "where is the one I made twenty images ago" without dragging a scrollbar.
    const [galleryOpen, setGalleryOpen] = useState(false);
    const [sliceTarget, setSliceTarget] = useState<SelectedImage | null>(null);

    const handleRemoveImport = useCallback((id: string) => {
        setPendingRemoval({ kind: 'import', id });
    }, []);

    // The click-shaped twin of Ctrl+V, aimed at the results panel.
    const handleImportFromClipboard = useCallback(async () => {
        const nothingPasted = () => showNotification(
            t('playground.pasteHint', { defaultValue: 'Copy an image first, then press Ctrl+V / ⌘V here' }),
            'info',
        );
        try {
            const items = await navigator.clipboard.read();
            const files: File[] = [];
            for (const item of items) {
                const type = item.types.find((candidate) => candidate.startsWith('image/'));
                if (!type) continue;
                const blob = await item.getType(type);
                files.push(new File([blob], `pasted-${Date.now()}.${type.split('/')[1] ?? 'png'}`, { type }));
            }
            if (files.length === 0) {
                nothingPasted();
                return;
            }
            await handleImportImages(files);
        } catch {
            nothingPasted();
        }
    }, [handleImportImages, showNotification, t]);

    // A text file — .txt, .md, a JSON or YAML template — is a prompt kept
    // outside the browser, and it opens as the prompt wherever on the panel
    // it lands: the prompt field, the reference row, the results panel, the
    // clipboard. It replaces what is in the field: the file *is* the prompt,
    // and appending to whatever was typed would hand back a mess to clean.
    const handleOpenPromptFile = useCallback(async (file: File) => {
        const result = await readPromptFile(file);
        if (result.ok) {
            setPrompt(result.text);
            showNotification(t('playground.promptLoaded', { defaultValue: 'Prompt loaded from {{name}}', name: result.name }), 'success');
            return;
        }
        const messages = {
            'too-large': t('playground.promptFileTooLarge', { defaultValue: '{{name}} is too large to be a prompt', name: result.name }),
            empty: t('playground.promptFileEmpty', { defaultValue: '{{name}} has no text in it', name: result.name }),
            unreadable: t('playground.promptFileUnreadable', { defaultValue: 'Could not read {{name}}', name: result.name }),
        } as const;
        showNotification(messages[result.reason], 'warning');
    }, [showNotification, t]);

    // Routes a drop by what was dropped: a text file becomes the prompt, the
    // images go wherever this drop target sends images.
    const handleDroppedFiles = useCallback((files: FileList | File[], onImages: (images: File[]) => void) => {
        const { prompt: promptFile, images } = partitionDroppedFiles(files);
        if (promptFile) void handleOpenPromptFile(promptFile);
        if (images.length > 0) onImages(images);
    }, [handleOpenPromptFile]);

    // Pasting an image anywhere on the panel adds it as a reference — the
    // user doesn't have to find the dropzone first; a pasted text *file*
    // opens as the prompt. A paste with no file (plain text into the prompt
    // field) is left alone. Scoped to this card's own DOM subtree via the
    // React synthetic paste event, not a window-level listener.
    const handlePaste = useCallback((event: React.ClipboardEvent) => {
        const items = event.clipboardData?.items;
        if (!items) return;
        const files = Array.from(items)
            .filter((item) => item.kind === 'file')
            .map((item) => item.getAsFile())
            .filter((file): file is File => file !== null);
        const promptFile = files.find(isPromptFile);
        const imageFiles = files.filter((file) => file.type.startsWith('image/'));
        if (!promptFile && imageFiles.length === 0) return;
        event.preventDefault();
        if (promptFile) void handleOpenPromptFile(promptFile);
        if (imageFiles.length > 0) void handleAddReferenceImages(imageFiles);
    }, [handleAddReferenceImages, handleOpenPromptFile]);

    // The Paste button is the click-shaped twin of Ctrl+V: it asks the
    // clipboard directly (Chromium-family browsers grant this after a prompt)
    // and, where the browser won't hand the clipboard to a click, says so and
    // points at the shortcut that always works — never a silent no-op.
    const handlePasteFromClipboard = useCallback(async () => {
        const nothingPasted = () => showNotification(
            t('playground.pasteHint', { defaultValue: 'Copy an image first, then press Ctrl+V / ⌘V here' }),
            'info',
        );
        try {
            const items = await navigator.clipboard.read();
            const files: File[] = [];
            for (const item of items) {
                const type = item.types.find((candidate) => candidate.startsWith('image/'));
                if (!type) continue;
                const blob = await item.getType(type);
                files.push(new File([blob], `pasted-${Date.now()}.${type.split('/')[1] ?? 'png'}`, { type }));
            }
            if (files.length === 0) {
                nothingPasted();
                return;
            }
            await handleAddReferenceImages(files);
        } catch {
            nothingPasted();
        }
    }, [handleAddReferenceImages, showNotification, t]);

    const handleOpenReference = useCallback((index: number) => {
        const image = referenceSelection(index);
        if (image) setSelectedImage(image);
    }, [referenceSelection, setSelectedImage]);

    // Hands the finished pixels over, not a notification that they exist.
    const handleDownload = useCallback(async (image: SelectedImage) => {
        try {
            await downloadImage(image.src, downloadStem(image, slugify));
        } catch {
            showNotification(
                t('playground.downloadFailed', { defaultValue: 'Could not download this image' }),
                'error',
            );
        }
    }, [showNotification, t]);

    const canSubmit = Boolean(prompt.trim()) && Boolean(model);

    const handleSubmit = useCallback(async () => {
        if (!canSubmit) return;
        await runGeneration({
            prompt: prompt.trim(),
            model,
            size,
            quality,
            count,
            sources: referenceImages,
        });
    }, [canSubmit, count, model, prompt, quality, referenceImages, runGeneration, size]);

    // ⌘/Ctrl+Enter from the prompt — in the panel or in the larger editor —
    // is the keyboard's Generate button.
    const handlePromptKeyDown = useCallback((event: React.KeyboardEvent) => {
        if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
            event.preventDefault();
            void handleSubmit();
        }
    }, [handleSubmit]);

    // Re-entry, not just retry: puts a run's entire request back into the
    // panel — prompt, model, size, quality, count and the images it was built
    // from — so the next attempt starts from what was asked and can be edited
    // first. "Done" is a state, not a lock (.design/ux-principles.md §10).
    const handleReuseRun = useCallback(async (run: GenerationRun) => {
        try {
            const sources = await runSourcesToReferences(run);
            setPrompt(run.prompt);
            setSelectedModel(run.model);
            setSize(run.size);
            setQuality(run.quality);
            setCount(run.count ?? 1);
            setReferenceImages(sources.slice(0, MAX_EDIT_REFERENCE_IMAGES));
            controlsPanelRef.current?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
            // A model that no longer has a rule can't be silently swapped for
            // another one: the panel says so rather than generating with a
            // model the user did not ask for.
            if (run.model && !models.includes(run.model)) {
                showNotification(
                    t('playground.reuse.modelMissing', {
                        defaultValue: '{{model}} has no rule any more — pick a model before generating',
                        model: run.model,
                    }),
                    'warning',
                );
                return;
            }
            showNotification(
                t('playground.reuse.loaded', { defaultValue: 'Request loaded into the panel — edit it and generate again' }),
                'success',
            );
        } catch {
            showNotification(t('playground.reuse.failed', { defaultValue: 'Could not load this request' }), 'error');
        }
    }, [models, runSourcesToReferences, setReferenceImages, showNotification, t]);

    // Opens one of a run's source images in the same lightbox its outputs use.
    const handleOpenRunSource = useCallback((run: GenerationRun, index: number) => {
        const src = run.sourceImages?.[index];
        if (src) setSelectedImage(runImage(run, 'source', index, src));
    }, [setSelectedImage]);

    // Empties the session in one move — the counterpart of a session that now
    // survives a reload: without this, a playground with forty images can only
    // be emptied one tile at a time. Only the in-memory/IndexedDB session goes;
    // the files the gateway wrote to the output folder are not this panel's to
    // delete, and the confirm says so.
    const handleClearSession = useCallback(() => {
        inFlightRef.current.forEach((controller) => controller.abort());
        updateRuns(() => []);
        updateImports(() => []);
        setSelectedImage(null);
        setGalleryOpen(false);
    }, [inFlightRef, setSelectedImage, updateImports, updateRuns]);

    const handleRemoveRun = useCallback((id: string) => {
        setPendingRemoval({ kind: 'run', id });
    }, []);

    const pendingImport = pendingRemoval?.kind === 'import'
        ? imported.find((item) => item.id === pendingRemoval.id)
        : undefined;

    const handleConfirmRemoval = useCallback(() => {
        if (!pendingRemoval) return;
        if (pendingRemoval.kind === 'run') removeRun(pendingRemoval.id);
        else removeImport(pendingRemoval.id);
        setPendingRemoval(null);
    }, [pendingRemoval, removeImport, removeRun]);

    const reuseLabel = t('playground.reuse.action', { defaultValue: 'Edit this request' });
    const copyPromptLabel = t('playground.copyPrompt', { defaultValue: 'Copy prompt' });
    const promptCopiedLabel = t('playground.promptCopied', { defaultValue: 'Copied' });

    // One timeline for the results panel: images brought in to work on and
    // images the model produced, in the order they arrived. Two lists would
    // make "where did my image go" a question the user has to ask.
    const timeline = useMemo(() => ([
        ...imported.map((item) => ({ kind: 'import' as const, at: item.createdAt, item })),
        ...runs.map((run) => ({ kind: 'run' as const, at: run.createdAt ?? 0, run })),
    ].sort((a, b) => a.at - b.at)), [imported, runs]);

    // The strip only ever shows the tail (oldest-to-newest, so the newest —
    // what the user just did — is the one nearest the "Generate" button it
    // came from). Everything older collapses into the "view all" tile at the
    // far end instead of staying draggable here.
    const visibleTimeline = useMemo(
        () => (timeline.length > HISTORY_STRIP_VISIBLE ? timeline.slice(-HISTORY_STRIP_VISIBLE) : timeline),
        [timeline],
    );
    const olderTimelineCount = timeline.length - visibleTimeline.length;

    const noModels = models.length === 0;
    const desktopPanelHeight = noModels && !loadingRules
        ? 'auto'
        : PLAYGROUND_PANEL_HEIGHT + (referenceImages.length > 0 ? REFERENCE_STRIP_EXTRA_HEIGHT : 0);

    return (
        <>
            <UnifiedCard
                size="full"
                titleHeadingLevel={1}
                title={t('playground.imageTitle', { defaultValue: 'Image Playground' })}
                subtitle={outputDir ? (
                    <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', flexWrap: 'wrap' }}>
                        <Box component="span">
                            {t('playground.outputDirLabel', { defaultValue: 'Generated images are saved to' })}:
                        </Box>
                        <Box component="span" sx={{ fontFamily: fontMono, wordBreak: 'break-all' }}>
                            {outputDir}
                        </Box>
                        <CopyIconButton
                            value={outputDir}
                            label={t('common.copy', { defaultValue: 'Copy' })}
                            copiedLabel={t('common.copied', { defaultValue: 'Copied!' })}
                            iconSize={14}
                            sx={{ p: 0.25 }}
                        />
                    </Stack>
                ) : undefined}
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
                        ref={controlsPanelRef}
                        data-testid="imagegen-controls-panel"
                        spacing={2}
                        sx={{
                            minWidth: 0,
                            height: { xs: 'auto', lg: desktopPanelHeight },
                        }}
                    >
                        {noModels && !loadingRules && (
                            <Alert
                                severity="info"
                                variant="outlined"
                                action={(
                                    <Button color="inherit" size="small" onClick={() => navigate('/image/api')}>
                                        {t('playground.addImageModel', { defaultValue: 'Add a model' })}
                                    </Button>
                                )}
                            >
                                {t('playground.noImageModels', {
                                    defaultValue: 'Add an image model rule on the Image API page to start generating images.',
                                })}
                            </Alert>
                        )}

                        {/* Reference images are optional input, not a mode. Empty, the
                            row is a one-line invitation; with images it grows into a
                            thumbnail strip. Either way the request below adapts. */}
                        <ReferenceImagesRow
                            referenceImages={referenceImages}
                            onDropFiles={handleDroppedFiles}
                            onAddReferenceImages={(files) => { void handleAddReferenceImages(files); }}
                            onOpenPromptFile={(file) => { void handleOpenPromptFile(file); }}
                            promptFileInputRef={promptFileInputRef}
                            onOpenReference={handleOpenReference}
                            onEditSketch={handleOpenSketch}
                            onEditMask={setMaskTarget}
                            onRemoveReference={handleRemoveReferenceImage}
                            onReorder={handleReorderReference}
                            onMoveByKey={handleReferenceKeyDown}
                            draggingReference={draggingReference}
                            dragOverReference={dragOverReference}
                            onDragStart={setDraggingReference}
                            onDragEnd={() => { setDraggingReference(null); setDragOverReference(null); }}
                            onDragEnter={setDragOverReference}
                            onDragLeave={(index) => setDragOverReference((current) => (current === index ? null : current))}
                        />

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

                        {/* The field takes whatever height the column has left and
                            scrolls inside its own outline — a fixed row count in a
                            height-constrained column is how text ends up painted
                            past the border. */}
                        <TextField
                            multiline
                            minRows={3}
                            fullWidth
                            label={t('playground.prompt', { defaultValue: 'Prompt' })}
                            placeholder={hasMaskedReference
                                ? t('playground.mask.promptPlaceholder', { defaultValue: 'Describe what should appear in the painted area…' })
                                : hasSketchReference
                                ? t('playground.sketch.promptPlaceholder', { defaultValue: 'Describe what this sketch should become…' })
                                : referenceImages.length > 0
                                    ? t('playground.referencePromptPlaceholder', { defaultValue: 'Describe what to make from these images…' })
                                    : t('playground.promptPlaceholder', { defaultValue: 'Describe the image you want to generate…' })}
                            value={prompt}
                            onChange={(event) => setPrompt(event.target.value)}
                            onKeyDown={handlePromptKeyDown}
                            onDragOver={(event) => event.preventDefault()}
                            onDrop={(event) => {
                                event.preventDefault();
                                if (event.dataTransfer.files?.length) {
                                    handleDroppedFiles(event.dataTransfer.files, (images) => { void handleAddReferenceImages(images); });
                                }
                            }}
                            disabled={noModels}
                            slotProps={{
                                input: {
                                    endAdornment: (
                                        <InputAdornment position="end" sx={{ alignSelf: 'flex-start', mt: 0.5, mr: -0.5, gap: 0.25 }}>
                                            {/* A prompt is text the user goes on to reuse elsewhere —
                                                it should never have to be selected by hand. */}
                                            {prompt.trim() && (
                                                <CopyIconButton
                                                    value={prompt}
                                                    label={copyPromptLabel}
                                                    copiedLabel={promptCopiedLabel}
                                                    iconSize={16}
                                                />
                                            )}
                                            <Tooltip title={t('playground.openPromptFile', { defaultValue: 'Open a text file as the prompt' })}>
                                                <IconButton
                                                    size="small"
                                                    onClick={() => promptFileInputRef.current?.click()}
                                                    aria-label={t('playground.openPromptFile', { defaultValue: 'Open a text file as the prompt' })}
                                                >
                                                    <Description sx={{ fontSize: 16 }} />
                                                </IconButton>
                                            </Tooltip>
                                            <Tooltip title={t('playground.expandPrompt', { defaultValue: 'Open the prompt in a larger editor' })}>
                                                <IconButton
                                                    size="small"
                                                    edge="end"
                                                    onClick={() => setPromptEditorOpen(true)}
                                                    aria-label={t('playground.expandPrompt', { defaultValue: 'Open the prompt in a larger editor' })}
                                                >
                                                    <OpenInFull sx={{ fontSize: 16 }} />
                                                </IconButton>
                                            </Tooltip>
                                        </InputAdornment>
                                    ),
                                },
                            }}
                            sx={{
                                flex: { lg: 1 },
                                minHeight: 0,
                                display: 'flex',
                                '& .MuiInputBase-root': {
                                    flex: 1,
                                    minHeight: 0,
                                    alignItems: 'flex-start',
                                },
                                // The textarea autosizes to its content with an inline
                                // height; inside a fixed-height column it has to be the
                                // flex item that shrinks and scrolls instead, or it is
                                // painted straight past the outline.
                                '& textarea.MuiInputBase-input': {
                                    flex: 1,
                                    alignSelf: 'stretch',
                                    height: { lg: 'auto !important' },
                                    minHeight: 0,
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

                        <Tooltip title={t('playground.submitShortcut', { defaultValue: '⌘/Ctrl + Enter to generate' })} placement="top">
                        <span>
                        <Button
                            variant="contained"
                            size="large"
                            fullWidth
                            onClick={() => { void handleSubmit(); }}
                            disabled={noModels || !canSubmit}
                            startIcon={pendingCount > 0
                                ? <CircularProgress size={18} color="inherit" />
                                : <AutoAwesome />}
                            sx={{
                                '&.Mui-disabled': {
                                    color: 'common.white',
                                },
                            }}
                        >
                            {pendingCount > 0
                                ? t('playground.generateAnother', { defaultValue: 'Generate another · {{count}} running', count: pendingCount })
                                : referenceImages.length > 0
                                    ? t('playground.generateFromReferences', {
                                        defaultValue_one: 'Generate from {{count}} image',
                                        defaultValue_other: 'Generate from {{count}} images',
                                        count: referenceImages.length,
                                    })
                                    : t('playground.generate', { defaultValue: 'Generate' })}
                        </Button>
                        </span>
                        </Tooltip>
                    </Stack>

                    {/* The results panel takes images too: dropping one here
                        says "I want to work on this", which is a different
                        intent from the reference row's "generate from this" —
                        so each lands where its outcome shows up. */}
                    <ImageGenResultsPanel
                        timeline={timeline}
                        visibleTimeline={visibleTimeline}
                        runsCount={runs.length}
                        importedCount={imported.length}
                        olderTimelineCount={olderTimelineCount}
                        desktopPanelHeight={desktopPanelHeight}
                        historyTrackRef={historyTrackRef}
                        onDropFiles={handleDroppedFiles}
                        onImportFiles={(files) => { void handleImportImages(files); }}
                        onImportFromClipboard={() => { void handleImportFromClipboard(); }}
                        onOpenGallery={() => setGalleryOpen(true)}
                        onOpenImport={handleOpenImport}
                        onUseAsReference={(src) => { void handleUseAsReference(src); }}
                        onRemoveImport={handleRemoveImport}
                        onCancelRun={handleCancelRun}
                        onRetryRun={(run) => { void handleRetry(run); }}
                        onReuseRun={(run) => { void handleReuseRun(run); }}
                        onOpenRunSource={handleOpenRunSource}
                        onOpenOutput={setSelectedImage}
                        onRemoveRun={handleRemoveRun}
                    />
                </Box>
            </UnifiedCard>
            <ImageGenLightbox
                selectedImage={selectedImage}
                onClose={() => setSelectedImage(null)}
                onKeyDown={handleLightboxKeyDown}
                lightboxRun={lightboxRun}
                lightboxFilm={lightboxFilm}
                onShowFrame={showLightboxFrame}
                promptCopied={promptCopied}
                onCopyPrompt={copyPrompt}
                reuseLabel={reuseLabel}
                onReuseRun={(run) => {
                    void handleReuseRun(run);
                    setSelectedImage(null);
                }}
                onSlice={(image) => setSliceTarget(image)}
                onDownload={(image) => { void handleDownload(image); }}
                referenceImages={referenceImages}
                onEditSketch={(index) => {
                    handleOpenSketch(index);
                    setSelectedImage(null);
                }}
                onUseAsReference={(src) => {
                    void handleUseAsReference(src);
                    setSelectedImage(null);
                }}
            />
            <ImageGenGalleryDialog
                open={galleryOpen}
                runs={runs}
                imported={imported}
                onClose={() => setGalleryOpen(false)}
                onOpenOutput={(run, imageIndex, src) => setSelectedImage(runImage(run, 'output', imageIndex, src))}
                onOpenSource={handleOpenRunSource}
                onOpenImport={handleOpenImport}
                onUseAsReference={(src) => { void handleUseAsReference(src); }}
                // Loading a request refills the panel behind the dialog, so the
                // dialog gets out of the way — otherwise the user is looking at
                // a grid while the thing they asked for happened elsewhere.
                onReuseRun={(run) => { setGalleryOpen(false); void handleReuseRun(run); }}
                onRetryRun={(run) => { void handleRetry(run); }}
                onCancelRun={handleCancelRun}
                onRemoveRun={handleRemoveRun}
                onRemoveImport={handleRemoveImport}
                onClearAll={handleClearSession}
            />
            <ConfirmDialog
                open={pendingRemoval !== null}
                title={pendingRemoval?.kind === 'import'
                    ? t('playground.removeImportedTitle', {
                        defaultValue: 'Remove {{name}}?',
                        name: pendingImport?.name ?? '',
                    })
                    : t('playground.removeRunTitle', { defaultValue: 'Remove this generation?' })}
                description={pendingRemoval?.kind === 'import'
                    ? t('playground.removeImportedBody', { defaultValue: 'Removes it from the playground.' })
                    : t('playground.removeRunBody', {
                        defaultValue: 'Removes it from the playground. Images already written to the output folder stay on disk.',
                    })}
                confirmLabel={t('common.delete', { defaultValue: 'Delete' })}
                cancelLabel={t('common.cancel', { defaultValue: 'Cancel' })}
                confirmColor="error"
                onClose={() => setPendingRemoval(null)}
                onConfirm={handleConfirmRemoval}
            />
            <ImageSliceDialog
                open={sliceTarget !== null}
                src={sliceTarget?.src ?? null}
                prompt={sliceTarget?.prompt || sliceTarget?.label || ''}
                onClose={() => setSliceTarget(null)}
                showNotification={showNotification}
            />
            <Dialog
                open={promptEditorOpen}
                onClose={() => setPromptEditorOpen(false)}
                maxWidth="md"
                fullWidth
            >
                <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1, pr: 1 }}>
                    <Typography variant="h6" component="span" sx={{ flex: 1, fontSize: '1.05rem' }}>
                        {t('playground.promptEditorTitle', { defaultValue: 'Prompt' })}
                    </Typography>
                    <CopyIconButton
                        value={prompt}
                        label={copyPromptLabel}
                        copiedLabel={promptCopiedLabel}
                        size="medium"
                    />
                    <Tooltip title={t('playground.openPromptFile', { defaultValue: 'Open a text file as the prompt' })}>
                        <IconButton
                            onClick={() => promptFileInputRef.current?.click()}
                            aria-label={t('playground.openPromptFile', { defaultValue: 'Open a text file as the prompt' })}
                        >
                            <Description fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <IconButton
                        onClick={() => setPromptEditorOpen(false)}
                        aria-label={t('playground.promptEditorDone', { defaultValue: 'Done' })}
                    >
                        <Close />
                    </IconButton>
                </DialogTitle>
                <DialogContent dividers>
                    {/* Same state as the panel's field — this is a bigger window
                        onto the prompt, not a second prompt. */}
                    <TextField
                        autoFocus
                        multiline
                        minRows={12}
                        maxRows={28}
                        fullWidth
                        value={prompt}
                        onChange={(event) => setPrompt(event.target.value)}
                        onKeyDown={handlePromptKeyDown}
                        placeholder={t('playground.promptPlaceholder', { defaultValue: 'Describe the image you want to generate…' })}
                        helperText={t('playground.submitShortcut', { defaultValue: '⌘/Ctrl + Enter to generate' })}
                    />
                </DialogContent>
                <DialogActions sx={{ px: 3, py: 2 }}>
                    <Button variant="contained" onClick={() => setPromptEditorOpen(false)}>
                        {t('playground.promptEditorDone', { defaultValue: 'Done' })}
                    </Button>
                </DialogActions>
            </Dialog>
            <MaskEditorDialog
                open={maskTarget !== null}
                imageUrl={maskedReference?.previewUrl ?? null}
                imageName={maskedReference?.file.name}
                initial={maskInitial}
                onClose={() => setMaskTarget(null)}
                onSubmit={handleMaskSubmit}
                onRemove={maskedReference?.mask
                    ? () => { if (maskTarget !== null) handleRemoveMask(maskTarget); setMaskTarget(null); }
                    : undefined}
                showNotification={showNotification}
            />
            <SketchCanvasDialog
                open={sketchTarget !== null}
                size={size}
                initial={sketchInitial}
                onClose={() => setSketchTarget(null)}
                onSubmit={handleSketchSubmit}
                showNotification={showNotification}
            />
        </>
    );
};

export default ImageGenPlaygroundCard;
