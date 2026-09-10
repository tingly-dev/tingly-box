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
import type { Rule } from '@/components/RoutingGraphTypes';
import UnifiedCard from '@/components/UnifiedCard';
import { CopyIconButton } from '@/components/CopyIconButton';
import { AutoAwesome, Close, ContentCopy, ContentPaste, Create, Description, Download, Edit, ErrorOutline, FileUpload, GridView, OpenInFull, Photo, Refresh, ZoomIn } from '@/components/icons';
import { useCopyFeedback } from '@/hooks/useCopyFeedback';
import { fontMono } from '@/theme/fonts';
import { parseImageSize } from '@/utils/sketchCanvas';
import { api } from '@/services/api';
import { getOpenAIClient } from '@/services/modelApi';
import { downloadImage, fetchBlob, slugify } from '@/utils/download';
import { loadPlaygroundSession, savePlaygroundSession } from '@/utils/playgroundSession';
import { isPromptFile, partitionDroppedFiles, readPromptFile } from '@/utils/promptFile';
import ImageSliceDialog from './ImageSliceDialog';
import SketchCanvasDialog, { type SketchLayers, type SketchResult } from './SketchCanvasDialog';

const IMAGE_SCENARIO = 'imagegen';
// Base panel height with the reference-image row in its compact (empty)
// state. Once references are added the row grows into a thumbnail strip
// (see desktopPanelHeight below) — both panels share one height value so
// they stay visually aligned (see the comment on the grid below).
const PLAYGROUND_PANEL_HEIGHT = 348;
const REFERENCE_STRIP_EXTRA_HEIGHT = 48;
// Matches the Codex-native imagegen tool's reference-image cap (see
// .design/imageedit.md) — the common denominator across providers behind
// this scenario.
const MAX_EDIT_REFERENCE_IMAGES = 5;

// Shared by the lightbox's overlay buttons — restyling the bar should be one edit.
const overlayIconSx = {
    color: 'common.white',
    bgcolor: 'rgba(255, 255, 255, 0.08)',
    '&:hover': { bgcolor: 'rgba(255, 255, 255, 0.16)' },
} as const;

// Which gateway endpoint a run went through. Not a user choice: derived
// from whether the run had reference images (see handleSubmit). Shown on
// the history card so API users learn which endpoint does what they just did.
type Endpoint = 'generations' | 'edits';
type Quality = 'auto' | 'high' | 'medium' | 'low' | 'standard';

interface ImageResult {
    url?: string;
    b64_json?: string;
}

interface ReferenceImage {
    file: File;
    previewUrl: string;
    // Where the image came from. A sketch keeps its canvas re-openable (see
    // handleOpenSketch) — "done" is a state, not a lock.
    source: 'upload' | 'sketch';
    // A sketch also keeps the layers it was flattened from — strokes as the
    // points they were drawn from, figures as joints — so re-opening it gives
    // back an editable canvas rather than a picture of one. The request still
    // sends `file`; this rides along for the editor only.
    layers?: SketchLayers;
    // Read once when the image arrives, so the lightbox can name what this is
    // (`sheet.png · 1024×1024 px`) instead of showing an empty prompt line.
    width?: number;
    height?: number;
}

// Decodes an image just far enough to learn its pixel size. Failure is not
// worth surfacing — the caption simply drops the dimensions.
const readImageSize = (src: string): Promise<{ width: number; height: number } | null> => new Promise((resolve) => {
    const image = new Image();
    image.onload = () => resolve({ width: image.naturalWidth, height: image.naturalHeight });
    image.onerror = () => resolve(null);
    image.src = src;
});

// Which sketch the canvas dialog is working on: `null` closed, `index: null`
// a new sketch, otherwise the reference image being redrawn.
type SketchTarget = { index: number | null } | null;

// An image the user brought in to work on rather than to generate from: it
// lands in the results panel, not in the request. The two destinations are
// different intents ("edit this with a model" vs "cut this one up"), so they
// are different drops, each landing where its result shows up.
interface ImportedImage {
    id: string;
    src: string;
    name: string;
    bytes: number;
    width?: number;
    height?: number;
    createdAt: number;
}

interface GenerationRun {
    id: string;
    endpoint: Endpoint;
    createdAt: number;
    prompt: string;
    model: string;
    size: string;
    quality: Quality;
    images: ImageResult[];
    // Data URLs of the reference images a run was built from, kept for
    // display alongside the output — the "what did I ask for" half of the
    // history card (only set when the run went through `edits`).
    sourceImages?: string[];
    // How many images the run asked for — kept so a failed run can be retried
    // with exactly the request it made.
    count?: number;
    status?: 'pending' | 'completed' | 'failed';
    // Why a failed run failed, shown on its card. A run that fails stays in
    // the strip: silently removing it leaves the user with a toast that has
    // already gone and no record of what was asked.
    error?: string;
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
    // Where this image came from — same lightbox, different framing. A run's
    // output and its `source` originals carry a prompt and a model; a
    // `reference` waiting in the request and an `import` brought in to work on
    // carry neither, so those two are titled by their file instead.
    kind: 'output' | 'source' | 'reference' | 'import';
    // Set for `reference` and `import`: what to call this image and what it is.
    label?: string;
    caption?: string;
}

// Keep playground output while navigating between pages in the current app session.
// This deliberately stays in memory: base64 images can quickly exceed sessionStorage quotas.
let imageGenSessionRuns: GenerationRun[] = [];
let imageGenSessionImports: ImportedImage[] = [];

const formatBytes = (bytes: number): string => (bytes >= 1024 * 1024
    ? `${(bytes / (1024 * 1024)).toFixed(1)} MB`
    : `${Math.max(1, Math.round(bytes / 1024))} KB`);

interface ImportedImageCardProps {
    item: ImportedImage;
    onOpen: () => void;
    onUseAsReference: () => void;
    onRemove: () => void;
}

// An imported image sits in the results strip alongside the runs: same card
// chrome, same zoom-to-work-on-it gesture. Its subtitle says what it is —
// there is no prompt or model to report, only the file itself.
const ImportedImageCard: React.FC<ImportedImageCardProps> = ({ item, onOpen, onUseAsReference, onRemove }) => {
    const { t } = useTranslation();
    const dimensions = item.width && item.height ? `${item.width}×${item.height} px` : '';
    return (
        <Card
            data-testid="imagegen-imported-image"
            variant="outlined"
            sx={{
                flex: { xs: '0 0 min(82vw, 320px)', md: '0 0 clamp(280px, 46%, 360px)' },
                height: '100%',
                bgcolor: 'background.paper',
                scrollSnapAlign: 'start',
            }}
        >
            <CardContent sx={{ p: 1.5, height: '100%', '&:last-child': { pb: 1.5 } }}>
                <Stack spacing={1.25} sx={{ height: '100%' }}>
                    <Box sx={{ minWidth: 0 }}>
                        <Typography
                            variant="body2"
                            sx={{ fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                        >
                            {item.name}
                        </Typography>
                        <Typography
                            variant="caption"
                            sx={{ display: 'block', color: 'text.secondary', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                        >
                            {[t('playground.importedBadge', { defaultValue: 'Imported' }), dimensions, formatBytes(item.bytes)]
                                .filter(Boolean)
                                .join(' · ')}
                        </Typography>
                    </Box>
                    <Box sx={{ position: 'relative', flex: 1, minHeight: 0, borderRadius: 1, overflow: 'hidden', bgcolor: 'action.hover' }}>
                        <ButtonBase
                            onClick={onOpen}
                            aria-label={t('playground.openImported', { defaultValue: 'Open {{name}}', name: item.name })}
                            sx={{
                                width: '100%',
                                height: '100%',
                                display: 'block',
                                '&:hover .image-preview-overlay, &:focus-visible .image-preview-overlay': { opacity: 1 },
                            }}
                        >
                            <Box
                                component="img"
                                src={item.src}
                                alt={item.name}
                                sx={{ width: '100%', height: '100%', objectFit: 'contain', display: 'block' }}
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
                        </ButtonBase>
                        <Tooltip title={t('playground.useAsReference', { defaultValue: 'Use as reference' })}>
                            <IconButton
                                size="small"
                                onClick={onUseAsReference}
                                aria-label={t('playground.useAsReference', { defaultValue: 'Use as reference' })}
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
                        </Tooltip>
                        <IconButton
                            size="small"
                            onClick={onRemove}
                            aria-label={t('playground.removeImported', { defaultValue: 'Remove {{name}}', name: item.name })}
                            sx={{
                                position: 'absolute',
                                top: 8,
                                right: 8,
                                width: 30,
                                height: 30,
                                color: 'common.white',
                                bgcolor: 'rgba(15, 23, 42, 0.58)',
                                backdropFilter: 'blur(4px)',
                                '&:hover': { bgcolor: 'rgba(15, 23, 42, 0.78)' },
                            }}
                        >
                            <Close fontSize="small" />
                        </IconButton>
                    </Box>
                </Stack>
            </CardContent>
        </Card>
    );
};

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
    // The prompt in a dialog-sized editor: the panel's field is one column of a
    // fixed-height panel, which is the wrong place to read or rework a long one.
    const [promptEditorOpen, setPromptEditorOpen] = useState(false);
    const [size, setSize] = useState('1024x1024');
    const [quality, setQuality] = useState<Quality>('auto');
    const [count, setCount] = useState(1);
    const [referenceImages, setReferenceImages] = useState<ReferenceImage[]>([]);
    const [runs, setRuns] = useState<GenerationRun[]>(() => imageGenSessionRuns);
    const [imported, setImported] = useState<ImportedImage[]>(() => imageGenSessionImports);
    const [selectedImage, setSelectedImage] = useState<SelectedImage | null>(null);
    const [sliceTarget, setSliceTarget] = useState<SelectedImage | null>(null);
    const [sketchTarget, setSketchTarget] = useState<SketchTarget>(null);
    // Where generated images land on disk — read-only, shown so the user can
    // navigate there themselves; this page never opens it for them.
    const [outputDir, setOutputDir] = useState('');
    useEffect(() => {
        let cancelled = false;
        void api.getImageGenOutputDir().then((result) => {
            if (!cancelled && result?.success) setOutputDir(result.path ?? '');
        });
        return () => { cancelled = true; };
    }, []);
    const historyTrackRef = useRef<HTMLDivElement>(null);
    const referenceFileInputRef = useRef<HTMLInputElement>(null);
    const importFileInputRef = useRef<HTMLInputElement>(null);
    const promptFileInputRef = useRef<HTMLInputElement>(null);
    const pendingCount = runs.filter((run) => run.status === 'pending').length;
    const { copied: promptCopied, copy: copyPrompt } = useCopyFeedback();
    // The in-flight request behind each pending card, so its Cancel button
    // can abort the fetch instead of leaving the user to wait out the
    // gateway's timeout on a provider that has stopped answering.
    const inFlightRef = useRef(new Map<string, AbortController>());

    const updateRuns = useCallback((updater: (currentRuns: GenerationRun[]) => GenerationRun[]) => {
        const nextRuns = updater(imageGenSessionRuns);
        imageGenSessionRuns = nextRuns;
        setRuns(nextRuns);
    }, []);

    // The session outlives a reload: whatever the last visit left in
    // IndexedDB comes back on mount (only when memory is empty — navigating
    // between pages keeps the in-memory copy, which is newer), and every
    // change is written back. A run that was still pending when the page went
    // away can never complete, so it comes back as failed rather than as a
    // spinner that spins forever.
    const [sessionRestored, setSessionRestored] = useState(false);
    useEffect(() => {
        let cancelled = false;
        if (imageGenSessionRuns.length > 0 || imageGenSessionImports.length > 0) {
            setSessionRestored(true);
            return undefined;
        }
        void loadPlaygroundSession<GenerationRun, ImportedImage>().then(({ runs: savedRuns, imports: savedImports }) => {
            if (cancelled) return;
            if (imageGenSessionRuns.length === 0 && savedRuns.length > 0) {
                imageGenSessionRuns = savedRuns.map((run) => (run.status === 'pending'
                    ? { ...run, status: 'failed', error: t('playground.interruptedByReload', { defaultValue: 'Interrupted by a page reload' }) }
                    : run));
                setRuns(imageGenSessionRuns);
            }
            if (imageGenSessionImports.length === 0 && savedImports.length > 0) {
                imageGenSessionImports = savedImports;
                setImported(imageGenSessionImports);
            }
            setSessionRestored(true);
        });
        return () => { cancelled = true; };
    }, [t]);

    useEffect(() => {
        if (!sessionRestored) return;
        void savePlaygroundSession({ runs, imports: imported });
    }, [imported, runs, sessionRestored]);

    useEffect(() => {
        const frame = requestAnimationFrame(() => {
            const track = historyTrackRef.current;
            if (track) track.scrollTo({ left: track.scrollWidth, behavior: 'smooth' });
        });
        return () => cancelAnimationFrame(frame);
    }, [imported.length, pendingCount, runs.length]);

    // Appends newly picked/dropped files (image/* only) up to the reference
    // cap, converting each to a data URL up front so thumbnails and the
    // eventual run history render through the same representation.
    const handleAddReferenceImages = useCallback(async (files: FileList | File[]) => {
        const incoming = Array.from(files).filter((file) => file.type.startsWith('image/'));
        if (incoming.length === 0) return;
        const accepted = incoming.slice(0, Math.max(0, MAX_EDIT_REFERENCE_IMAGES - referenceImages.length));
        if (accepted.length === 0) return;
        const withPreviews = await Promise.all(accepted.map(async (file): Promise<ReferenceImage> => {
            const previewUrl = await fileToDataUrl(file);
            return { file, previewUrl, source: 'upload', ...(await readImageSize(previewUrl) ?? {}) };
        }));
        setReferenceImages((current) => [...current, ...withPreviews].slice(0, MAX_EDIT_REFERENCE_IMAGES));
    }, [referenceImages.length]);

    // Brings images into the results panel. Same decoding as a reference (data
    // URL up front, so one representation renders everywhere), different
    // destination: nothing here is sent to a model.
    const handleImportImages = useCallback(async (files: FileList | File[]) => {
        const incoming = Array.from(files).filter((file) => file.type.startsWith('image/'));
        if (incoming.length === 0) return;
        const items = await Promise.all(incoming.map(async (file): Promise<ImportedImage> => {
            const src = await fileToDataUrl(file);
            return {
                id: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
                src,
                name: file.name,
                bytes: file.size,
                createdAt: Date.now(),
                ...(await readImageSize(src) ?? {}),
            };
        }));
        imageGenSessionImports = [...imageGenSessionImports, ...items];
        setImported(imageGenSessionImports);
    }, []);

    const handleRemoveImport = useCallback((id: string) => {
        imageGenSessionImports = imageGenSessionImports.filter((item) => item.id !== id);
        setImported(imageGenSessionImports);
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

    const handleRemoveReferenceImage = useCallback((index: number) => {
        setReferenceImages((current) => current.filter((_, i) => i !== index));
    }, []);

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

    // Hands a completed output straight back in as the next run's reference —
    // the artifact for the next action, not just a notification that one
    // exists. Reuses the already-rendered src as the preview (it's already a
    // data URL/data-equivalent), so this never re-encodes the image.
    //
    // It joins the references rather than replacing them: the row holds up
    // to five, and "use this one too" is the common case. At the cap the
    // oldest makes room.
    const handleUseAsReference = useCallback(async (src: string) => {
        try {
            const blob = await fetchBlob(src);
            const file = new File([blob], `reference-${Date.now()}.png`, { type: blob.type || 'image/png' });
            const next: ReferenceImage = {
                file,
                previewUrl: src,
                source: 'upload',
                ...(await readImageSize(src) ?? {}),
            };
            setReferenceImages((current) => [...current, next].slice(-MAX_EDIT_REFERENCE_IMAGES));
        } catch {
            showNotification(
                t('playground.referenceLoadFailed', { defaultValue: 'Could not use this image as a reference' }),
                'error',
            );
        }
    }, [showNotification, t]);

    // A sketch is just another way to get a reference image: it lands in the
    // same list, goes through the same request, and shows up in the run
    // history like any upload. Redrawing replaces the sketch in place so it
    // keeps its position among the other references.
    // A brought-in image is a first-class image on this panel, not just a
    // request parameter: it opens in the same lightbox as a result, with the
    // same download and slicing tools. Its header names the file and its real
    // pixel size, since there is no prompt or model behind it.
    const handleOpenReference = useCallback((index: number) => {
        const ref = referenceImages[index];
        if (!ref) return;
        const dimensions = ref.width && ref.height ? `${ref.width}×${ref.height} px` : '';
        const kilobytes = `${Math.max(1, Math.round(ref.file.size / 1024))} KB`;
        setSelectedImage({
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
        });
    }, [referenceImages, t]);

    const handleOpenSketch = useCallback((index: number | null) => {
        if (index === null && referenceImages.length >= MAX_EDIT_REFERENCE_IMAGES) return;
        setSketchTarget({ index });
    }, [referenceImages.length]);

    const handleSketchSubmit = useCallback(async (result: SketchResult) => {
        const sketch: ReferenceImage = {
            file: result.file,
            previewUrl: result.previewUrl,
            source: 'sketch',
            layers: result.layers,
            ...(await readImageSize(result.previewUrl) ?? {}),
        };
        setReferenceImages((current) => {
            const index = sketchTarget?.index ?? null;
            if (index !== null && index < current.length) {
                return current.map((ref, i) => (i === index ? sketch : ref));
            }
            return [...current, sketch].slice(0, MAX_EDIT_REFERENCE_IMAGES);
        });
        setSketchTarget(null);
    }, [sketchTarget]);

    // The layers of the sketch being re-opened, or null for a new one.
    // Memoised because the canvas resets whenever this changes, and a fresh
    // object on every render would wipe what the user is drawing.
    const sketchInitial = useMemo<SketchLayers | null>(() => {
        const index = sketchTarget?.index;
        const target = index !== null && index !== undefined ? referenceImages[index] : undefined;
        if (!target) return null;
        if (target.layers) return target.layers;
        // No layers: a sketch flattened by an older build. It still opens, as
        // pixels to keep drawing on, just not as strokes or a posable figure.
        return { size: parseImageSize(size), strokes: [], figures: [], backdrop: target.previewUrl };
    }, [sketchTarget, referenceImages, size]);
    const hasSketchReference = referenceImages.some((ref) => ref.source === 'sketch');

    // Hands the finished pixels over, not a notification that they exist.
    const handleDownload = useCallback(async (image: SelectedImage) => {
        try {
            await downloadImage(image.src, `${slugify(image.prompt)}-${image.index + 1}`);
        } catch {
            showNotification(
                t('playground.downloadFailed', { defaultValue: 'Could not download this image' }),
                'error',
            );
        }
    }, [showNotification, t]);

    interface GenerationRequest {
        prompt: string;
        model: string;
        size: string;
        quality: Quality;
        count: number;
        sources: { file: File; previewUrl: string }[];
        // Set when re-running a failed run: its card flips back to pending
        // instead of a second card appearing.
        runId?: string;
    }

    // One path for a fresh request and a retry, so a retry is the same call
    // the original was and not a re-implementation that drifts.
    const runGeneration = useCallback(async (request: GenerationRequest) => {
        const runId = request.runId ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`;
        // The endpoint is a consequence of the inputs, not a mode the user
        // picks: references present → edits, none → generations. Which
        // providers can serve either is the gateway's concern, not this
        // panel's (see .design/imageedit.md).
        const endpoint: Endpoint = request.sources.length > 0 ? 'edits' : 'generations';
        const pendingRun: GenerationRun = {
            id: runId,
            createdAt: Date.now(),
            endpoint,
            prompt: request.prompt,
            model: request.model,
            size: request.size,
            quality: request.quality,
            count: request.count,
            images: [],
            sourceImages: endpoint === 'edits' ? request.sources.map((ref) => ref.previewUrl) : undefined,
            status: 'pending',
        };
        updateRuns((currentRuns) => (currentRuns.some((run) => run.id === runId)
            ? currentRuns.map((run) => (run.id === runId ? { ...pendingRun, createdAt: run.createdAt } : run))
            : [...currentRuns, pendingRun]));
        const controller = new AbortController();
        inFlightRef.current.set(runId, controller);
        try {
            const client = await getOpenAIClient(IMAGE_SCENARIO);
            const editFiles = request.sources.map((ref) => ref.file);
            const response = endpoint === 'edits'
                ? await client.images.edit({
                    image: editFiles.length === 1 ? editFiles[0] : editFiles,
                    model: request.model,
                    prompt: request.prompt,
                    n: request.count,
                    size: request.size as any,
                    quality: request.quality as any,
                }, { signal: controller.signal })
                : await client.images.generate({
                    model: request.model,
                    prompt: request.prompt,
                    n: request.count,
                    size: request.size as any,
                    quality: request.quality,
                }, { signal: controller.signal });
            const images = response.data ?? [];
            updateRuns((currentRuns) => currentRuns.map((run) => (
                run.id === runId ? { ...run, images, status: 'completed', error: undefined } : run
            )));
        } catch (error: any) {
            if (controller.signal.aborted) {
                // The user asked for this; the card records it so the request
                // is still there to retry, and no toast second-guesses them.
                updateRuns((currentRuns) => currentRuns.map((run) => (
                    run.id === runId
                        ? { ...run, status: 'failed', error: t('playground.cancelled', { defaultValue: 'Cancelled' }) }
                        : run
                )));
                return;
            }
            const status = error?.status ? `${error.status}: ` : '';
            const message = error?.error?.message || error?.message || t('playground.requestFailed', { defaultValue: 'Request failed' });
            updateRuns((currentRuns) => currentRuns.map((run) => (
                run.id === runId ? { ...run, status: 'failed', error: `${status}${message}` } : run
            )));
            showNotification(`${status}${message}`, 'error');
        } finally {
            inFlightRef.current.delete(runId);
        }
    }, [showNotification, t, updateRuns]);

    const handleCancelRun = useCallback((id: string) => {
        inFlightRef.current.get(id)?.abort();
    }, []);

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

    // A failed run's references only survive as data URLs, so they are read
    // back into files here; the request itself is the one the card records.
    const handleRetry = useCallback(async (run: GenerationRun) => {
        try {
            const sources = await Promise.all((run.sourceImages ?? []).map(async (src, index) => {
                const blob = await fetchBlob(src);
                return { file: new File([blob], `reference-${index + 1}.png`, { type: blob.type || 'image/png' }), previewUrl: src };
            }));
            await runGeneration({
                prompt: run.prompt,
                model: run.model,
                size: run.size,
                quality: run.quality,
                count: run.count ?? 1,
                sources,
                runId: run.id,
            });
        } catch {
            showNotification(t('playground.requestFailed', { defaultValue: 'Request failed' }), 'error');
        }
    }, [runGeneration, showNotification, t]);

    const handleRemoveRun = useCallback((id: string) => {
        updateRuns((currentRuns) => currentRuns.filter((run) => run.id !== id));
    }, [updateRuns]);

    // The three ways a reference image gets here, as equals. Drop is not in
    // the list because it has no button — the dashed box itself is the target.
    const referenceSources = [
        {
            key: 'browse',
            label: t('playground.referenceBrowse', { defaultValue: 'Browse' }),
            icon: <FileUpload fontSize="small" />,
            onClick: () => referenceFileInputRef.current?.click(),
        },
        {
            key: 'paste',
            label: t('playground.referencePaste', { defaultValue: 'Paste' }),
            icon: <ContentPaste fontSize="small" />,
            onClick: () => { void handlePasteFromClipboard(); },
        },
        {
            key: 'sketch',
            label: t('playground.sketch.action', { defaultValue: 'Sketch' }),
            icon: <Create fontSize="small" />,
            onClick: () => handleOpenSketch(null),
        },
    ];

    // One timeline for the results panel: images brought in to work on and
    // images the model produced, in the order they arrived. Two lists would
    // make "where did my image go" a question the user has to ask.
    const timeline = useMemo(() => ([
        ...imported.map((item) => ({ kind: 'import' as const, at: item.createdAt, item })),
        ...runs.map((run) => ({ kind: 'run' as const, at: run.createdAt ?? 0, run })),
    ].sort((a, b) => a.at - b.at)), [imported, runs]);

    const noModels = models.length === 0;
    const desktopPanelHeight = noModels && !loadingRules
        ? 'auto'
        : PLAYGROUND_PANEL_HEIGHT + (referenceImages.length > 0 ? REFERENCE_STRIP_EXTRA_HEIGHT : 0);

    return (
        <>
            <UnifiedCard
                size="full"
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

                        {/* Reference images are optional input, not a mode. Empty, the
                            row is a one-line invitation; with images it grows into a
                            thumbnail strip. Either way the request below adapts. */}
                        <Box>
                                <Typography variant="caption" sx={{ display: 'block', mb: 0.5, color: 'text.secondary' }}>
                                    {t('playground.referenceImages', { defaultValue: 'Reference images' })}
                                    {' · '}
                                    {t('playground.referenceOptional', { defaultValue: 'optional · drop images here to generate from them' })}
                                </Typography>
                                <Box
                                    onClick={() => referenceFileInputRef.current?.click()}
                                    onDragOver={(event) => event.preventDefault()}
                                    onDrop={(event) => {
                                        event.preventDefault();
                                        if (event.dataTransfer.files?.length) {
                                            handleDroppedFiles(event.dataTransfer.files, (images) => { void handleAddReferenceImages(images); });
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
                                        minHeight: referenceImages.length === 0 ? 44 : 64,
                                        alignItems: 'center',
                                        cursor: referenceImages.length < MAX_EDIT_REFERENCE_IMAGES ? 'pointer' : 'default',
                                    }}
                                >
                                    {referenceImages.length === 0 ? (
                                        <Stack
                                            direction="row"
                                            spacing={0.5}
                                            useFlexGap
                                            sx={{ width: '100%', alignItems: 'center', justifyContent: 'center', flexWrap: 'wrap' }}
                                        >
                                            {referenceSources.map((source) => (
                                                <Button
                                                    key={source.key}
                                                    size="small"
                                                    color="inherit"
                                                    startIcon={source.icon}
                                                    onClick={(event) => { event.stopPropagation(); source.onClick(); }}
                                                    sx={{ color: 'text.secondary', px: 1.25, '&:hover': { color: 'primary.main' } }}
                                                >
                                                    {source.label}
                                                </Button>
                                            ))}
                                        </Stack>
                                    ) : (
                                        <>
                                            {referenceImages.map((ref, index) => (
                                                <Box
                                                    key={index}
                                                    sx={{ position: 'relative', width: 56, height: 56, borderRadius: 1, overflow: 'hidden', flexShrink: 0 }}
                                                >
                                                    <ButtonBase
                                                        onClick={(event) => { event.stopPropagation(); handleOpenReference(index); }}
                                                        aria-label={t('playground.openReference', {
                                                            defaultValue: 'Open reference image {{number}}',
                                                            number: index + 1,
                                                        })}
                                                        sx={{
                                                            width: '100%',
                                                            height: '100%',
                                                            display: 'block',
                                                            '&:hover .reference-zoom, &:focus-visible .reference-zoom': { opacity: 1 },
                                                        }}
                                                    >
                                                        <Box
                                                            component="img"
                                                            src={ref.previewUrl}
                                                            alt={t('playground.referenceThumbAlt', { defaultValue: 'Reference image {{number}}', number: index + 1 })}
                                                            sx={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
                                                        />
                                                        <Box
                                                            className="reference-zoom"
                                                            sx={{
                                                                position: 'absolute',
                                                                inset: 0,
                                                                display: 'flex',
                                                                alignItems: 'center',
                                                                justifyContent: 'center',
                                                                color: 'common.white',
                                                                bgcolor: 'rgba(15, 23, 42, 0.42)',
                                                                opacity: 0,
                                                                transition: 'opacity 0.16s ease-out',
                                                            }}
                                                        >
                                                            <ZoomIn fontSize="small" />
                                                        </Box>
                                                    </ButtonBase>
                                                    {ref.source === 'sketch' && (
                                                        <Tooltip title={t('playground.sketch.editAction', { defaultValue: 'Edit sketch' })}>
                                                            <IconButton
                                                                size="small"
                                                                onClick={(event) => { event.stopPropagation(); handleOpenSketch(index); }}
                                                                aria-label={t('playground.sketch.editAction', { defaultValue: 'Edit sketch' })}
                                                                sx={{
                                                                    position: 'absolute',
                                                                    bottom: 2,
                                                                    right: 2,
                                                                    width: 20,
                                                                    height: 20,
                                                                    bgcolor: 'rgba(15, 23, 42, 0.7)',
                                                                    color: 'common.white',
                                                                    '&:hover': { bgcolor: 'rgba(15, 23, 42, 0.9)' },
                                                                }}
                                                            >
                                                                <Create sx={{ fontSize: 13 }} />
                                                            </IconButton>
                                                        </Tooltip>
                                                    )}
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
                                            {referenceImages.length < MAX_EDIT_REFERENCE_IMAGES && referenceSources.map((source) => (
                                                <Tooltip key={source.key} title={source.label}>
                                                    <ButtonBase
                                                        onClick={(event) => { event.stopPropagation(); source.onClick(); }}
                                                        aria-label={source.label}
                                                        sx={{
                                                            width: 56,
                                                            height: 56,
                                                            borderRadius: 1,
                                                            color: 'text.secondary',
                                                            border: '1px solid',
                                                            borderColor: 'divider',
                                                            '&:hover': { color: 'primary.main', borderColor: 'primary.main' },
                                                        }}
                                                    >
                                                        {source.icon}
                                                    </ButtonBase>
                                                </Tooltip>
                                            ))}
                                        </>
                                    )}
                                </Box>
                                {referenceImages.length > 0 && (
                                    <Typography variant="caption" sx={{ display: 'block', mt: 0.5, color: 'text.disabled' }}>
                                        {t('playground.referenceHint', {
                                            defaultValue: 'Up to {{max}} images · PNG, JPEG, or WebP · sent via images/edits',
                                            max: MAX_EDIT_REFERENCE_IMAGES,
                                        })}
                                    </Typography>
                                )}
                                <input
                                    ref={promptFileInputRef}
                                    type="file"
                                    accept=".txt,.text,.md,.markdown,.mdx,.rst,.org,.prompt,.json,.yaml,.yml,.toml,.csv,text/*,application/json"
                                    hidden
                                    onChange={(event) => {
                                        const file = event.target.files?.[0];
                                        if (file) void handleOpenPromptFile(file);
                                        event.target.value = '';
                                    }}
                                />
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
                            placeholder={hasSketchReference
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
                    <Box
                        data-testid="imagegen-preview-panel"
                        onDragOver={(event) => event.preventDefault()}
                        onDrop={(event) => {
                            event.preventDefault();
                            if (event.dataTransfer.files?.length) {
                                handleDroppedFiles(event.dataTransfer.files, (images) => { void handleImportImages(images); });
                            }
                        }}
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
                            alignItems: timeline.length === 0 ? 'center' : 'stretch',
                            justifyContent: timeline.length === 0 ? 'center' : 'flex-start',
                            overflow: 'hidden',
                        }}
                    >
                        <input
                            ref={importFileInputRef}
                            type="file"
                            accept="image/png,image/jpeg,image/webp"
                            multiple
                            hidden
                            onChange={(event) => {
                                if (event.target.files?.length) void handleImportImages(event.target.files);
                                event.target.value = '';
                            }}
                        />
                        {timeline.length === 0 ? (
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
                                    {t('playground.previewEmpty', { defaultValue: 'Generated and imported images appear here' })}
                                </Typography>
                                <Typography variant="caption" sx={{
                                    color: "text.disabled"
                                }}>
                                    {t('playground.previewHint', { defaultValue: 'Drop an image here to work on it without generating anything.' })}
                                </Typography>
                                <Stack direction="row" spacing={0.5} sx={{ pt: 0.5 }}>
                                    <Button
                                        size="small"
                                        color="inherit"
                                        startIcon={<FileUpload fontSize="small" />}
                                        onClick={() => importFileInputRef.current?.click()}
                                        sx={{ color: 'text.secondary', '&:hover': { color: 'primary.main' } }}
                                    >
                                        {t('playground.referenceBrowse', { defaultValue: 'Browse' })}
                                    </Button>
                                    <Button
                                        size="small"
                                        color="inherit"
                                        startIcon={<ContentPaste fontSize="small" />}
                                        onClick={() => { void handleImportFromClipboard(); }}
                                        sx={{ color: 'text.secondary', '&:hover': { color: 'primary.main' } }}
                                    >
                                        {t('playground.referencePaste', { defaultValue: 'Paste' })}
                                    </Button>
                                </Stack>
                            </Stack>
                        ) : (
                            <Stack spacing={1.5} sx={{ width: '100%', minWidth: 0, height: '100%' }}>
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                    <Typography variant="subtitle2" sx={{ minWidth: 0 }}>
                                        {t('playground.sessionOutputs', { defaultValue: 'Session images' })}
                                    </Typography>
                                    <Typography
                                        variant="caption"
                                        sx={{
                                            color: "text.secondary",
                                            flexShrink: 0,
                                            whiteSpace: 'nowrap',
                                            ml: 'auto',
                                        }}
                                    >
                                        {[
                                            runs.length > 0 && (runs.length === 1
                                                ? t('playground.runCountOne', { defaultValue: '1 generation' })
                                                : t('playground.runCount', {
                                                    defaultValue: '{{count}} generations',
                                                    count: runs.length,
                                                })),
                                            imported.length > 0 && t('playground.importCount', {
                                                defaultValue: '{{count}} imported',
                                                count: imported.length,
                                            }),
                                        ].filter(Boolean).join(' · ')}
                                    </Typography>
                                    {/* The same two ways in as the empty state offers: the
                                        strip filling up must not take them away. */}
                                    <Stack direction="row" spacing={0.5} sx={{ flexShrink: 0 }}>
                                        <Button
                                            size="small"
                                            color="inherit"
                                            startIcon={<FileUpload fontSize="small" />}
                                            onClick={() => importFileInputRef.current?.click()}
                                            sx={{ color: 'text.secondary', '&:hover': { color: 'primary.main' } }}
                                        >
                                            {t('playground.referenceBrowse', { defaultValue: 'Browse' })}
                                        </Button>
                                        <Button
                                            size="small"
                                            color="inherit"
                                            startIcon={<ContentPaste fontSize="small" />}
                                            onClick={() => { void handleImportFromClipboard(); }}
                                            sx={{ color: 'text.secondary', '&:hover': { color: 'primary.main' } }}
                                        >
                                            {t('playground.referencePaste', { defaultValue: 'Paste' })}
                                        </Button>
                                    </Stack>
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
                                    {timeline.map((entry) => (entry.kind === 'import' ? (
                                        <ImportedImageCard
                                            key={entry.item.id}
                                            item={entry.item}
                                            onOpen={() => setSelectedImage({
                                                src: entry.item.src,
                                                prompt: '',
                                                model: '',
                                                size: '',
                                                quality: 'auto',
                                                index: 0,
                                                kind: 'import',
                                                label: entry.item.name,
                                                caption: [
                                                    entry.item.width && entry.item.height
                                                        ? `${entry.item.width}×${entry.item.height} px`
                                                        : '',
                                                    formatBytes(entry.item.bytes),
                                                ].filter(Boolean).join(' · '),
                                            })}
                                            onUseAsReference={() => { void handleUseAsReference(entry.item.src); }}
                                            onRemove={() => handleRemoveImport(entry.item.id)}
                                        />
                                    ) : ((run) => (
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
                                                borderColor: run.status === 'failed' ? 'error.main' : undefined,
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
                                                        {t('playground.generatingNew', { defaultValue: 'Generating new images…' })}
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
                                                        {run.model} · {run.size} · {run.quality} · images/{run.endpoint}
                                                    </Typography>
                                                    <Button
                                                        size="small"
                                                        variant="outlined"
                                                        color="inherit"
                                                        startIcon={<Close fontSize="small" />}
                                                        onClick={() => handleCancelRun(run.id)}
                                                        data-testid="imagegen-cancel-run"
                                                    >
                                                        {t('playground.cancelRun', { defaultValue: 'Cancel' })}
                                                    </Button>
                                                </Stack>
                                            ) : run.status === 'failed' ? (
                                                <Stack spacing={1} sx={{ height: '100%', minWidth: 0 }}>
                                                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                                                        <ErrorOutline color="error" fontSize="small" />
                                                        <Typography variant="body2" sx={{ fontWeight: 500, flex: 1, minWidth: 0 }}>
                                                            {t('playground.runFailed', { defaultValue: 'Generation failed' })}
                                                        </Typography>
                                                        <IconButton
                                                            size="small"
                                                            onClick={() => handleRemoveRun(run.id)}
                                                            aria-label={t('playground.removeRun', { defaultValue: 'Remove this generation' })}
                                                            sx={{ mr: -0.5, mt: -0.5 }}
                                                        >
                                                            <Close fontSize="small" />
                                                        </IconButton>
                                                    </Box>
                                                    <Typography
                                                        variant="caption"
                                                        sx={{
                                                            color: 'error.main',
                                                            display: '-webkit-box',
                                                            WebkitLineClamp: 3,
                                                            WebkitBoxOrient: 'vertical',
                                                            overflow: 'hidden',
                                                            wordBreak: 'break-word',
                                                        }}
                                                    >
                                                        {run.error}
                                                    </Typography>
                                                    <Typography
                                                        variant="caption"
                                                        sx={{
                                                            color: 'text.secondary',
                                                            display: '-webkit-box',
                                                            WebkitLineClamp: 3,
                                                            WebkitBoxOrient: 'vertical',
                                                            overflow: 'hidden',
                                                        }}
                                                    >
                                                        {run.prompt}
                                                    </Typography>
                                                    <Typography
                                                        variant="caption"
                                                        sx={{ color: 'text.disabled', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                                                    >
                                                        {run.model} · {run.size} · {run.quality} · images/{run.endpoint}
                                                    </Typography>
                                                    <Box sx={{ flex: 1 }} />
                                                    <Button
                                                        size="small"
                                                        variant="outlined"
                                                        startIcon={<Refresh fontSize="small" />}
                                                        onClick={() => { void handleRetry(run); }}
                                                        sx={{ alignSelf: 'flex-start' }}
                                                    >
                                                        {t('playground.retry', { defaultValue: 'Retry' })}
                                                    </Button>
                                                </Stack>
                                            ) : (
                                            <Stack spacing={1.25} sx={{ height: '100%' }}>
                                                <Box sx={{ minWidth: 0 }}>
                                                    <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 0.75 }}>
                                                        <Typography
                                                            variant="body2"
                                                            sx={{
                                                                fontWeight: 500,
                                                                flex: 1,
                                                                minWidth: 0,
                                                                display: '-webkit-box',
                                                                WebkitLineClamp: 2,
                                                                WebkitBoxOrient: 'vertical',
                                                                overflow: 'hidden',
                                                            }}
                                                        >
                                                            {run.prompt}
                                                        </Typography>
                                                        <IconButton
                                                            size="small"
                                                            onClick={() => handleRemoveRun(run.id)}
                                                            aria-label={t('playground.removeRun', { defaultValue: 'Remove this generation' })}
                                                            sx={{ mr: -0.75, mt: -0.75, color: 'text.disabled', '&:hover': { color: 'text.primary' } }}
                                                        >
                                                            <Close sx={{ fontSize: 16 }} />
                                                        </IconButton>
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
                                                        {run.model} · {run.size} · {run.quality} · images/{run.endpoint}
                                                    </Typography>
                                                    {run.sourceImages && run.sourceImages.length > 0 && (
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
                                                                    aria-label={t('playground.useAsReference', { defaultValue: 'Use as reference' })}
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
                                    ))(entry.run)))}
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
                                {selectedImage?.label ?? selectedImage?.prompt}
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
                            {selectedImage?.label
                                ? selectedImage.caption
                                : `${selectedImage?.model} · ${selectedImage?.size} · ${selectedImage?.quality}`}
                        </Typography>
                    </Box>
                    <Stack direction="row" spacing={0.75} sx={{ flexShrink: 0 }}>
                        {!selectedImage?.label && (
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
                                sx={overlayIconSx}
                            >
                                <ContentCopy fontSize="small" />
                            </IconButton>
                        </Tooltip>
                        )}
                        <Tooltip title={t('playground.slice.action', { defaultValue: 'Split into tiles' })}>
                            <IconButton
                                onClick={() => setSliceTarget(selectedImage)}
                                aria-label={t('playground.slice.action', { defaultValue: 'Split into tiles' })}
                                sx={overlayIconSx}
                            >
                                <GridView fontSize="small" />
                            </IconButton>
                        </Tooltip>
                        <Tooltip title={t('playground.download', { defaultValue: 'Download' })}>
                            <IconButton
                                onClick={() => { if (selectedImage) void handleDownload(selectedImage); }}
                                aria-label={t('playground.download', { defaultValue: 'Download' })}
                                sx={overlayIconSx}
                            >
                                <Download fontSize="small" />
                            </IconButton>
                        </Tooltip>
                        {/* An image that is already a reference has nowhere to
                            be sent — the one edit it still affords is redrawing
                            it, and only if it came from the sketch canvas. */}
                        {selectedImage?.kind === 'reference' ? (
                            referenceImages[selectedImage.index]?.source === 'sketch' && (
                                <Tooltip title={t('playground.sketch.editAction', { defaultValue: 'Edit sketch' })}>
                                    <IconButton
                                        onClick={() => {
                                            handleOpenSketch(selectedImage.index);
                                            setSelectedImage(null);
                                        }}
                                        aria-label={t('playground.sketch.editAction', { defaultValue: 'Edit sketch' })}
                                        sx={overlayIconSx}
                                    >
                                        <Create fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            )
                        ) : (
                            <Tooltip title={t('playground.useAsReference', { defaultValue: 'Use as reference' })}>
                                <IconButton
                                    onClick={() => {
                                        if (!selectedImage) return;
                                        void handleUseAsReference(selectedImage.src);
                                        setSelectedImage(null);
                                    }}
                                    aria-label={t('playground.useAsReference', { defaultValue: 'Use as reference' })}
                                    sx={overlayIconSx}
                                >
                                    <Edit fontSize="small" />
                                </IconButton>
                            </Tooltip>
                        )}
                        <IconButton
                            onClick={() => setSelectedImage(null)}
                            aria-label={t('playground.closePreview', { defaultValue: 'Close image preview' })}
                            sx={overlayIconSx}
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
                            alt={selectedImage.label
                                ?? (selectedImage.kind === 'output'
                                    ? t('playground.resultAlt', { defaultValue: 'Generated image {{number}}', number: selectedImage.index + 1 })
                                    : t('playground.referenceThumbAlt', { defaultValue: 'Reference image {{number}}', number: selectedImage.index + 1 }))}
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
