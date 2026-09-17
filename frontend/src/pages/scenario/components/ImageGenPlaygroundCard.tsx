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
import ConfirmDialog from '@/components/ConfirmDialog';
import { CopyIconButton } from '@/components/CopyIconButton';
import { AutoAwesome, Close, ContentCopy, ContentPaste, Create, Description, Download, Edit, ErrorOutline, FileUpload, GridView, OpenInFull, Photo, Refresh, RestartAlt, ViewGallery, ZoomIn } from '@/components/icons';
import { useCopyFeedback } from '@/hooks/useCopyFeedback';
import { fontMono } from '@/theme/fonts';
import { parseImageSize } from '@tingly/vision';
import { api } from '@/services/api';
import { getOpenAIClient } from '@/services/modelApi';
import { downloadImage, fetchBlob, slugify } from '@/utils/download';
import { loadPlaygroundSession, savePlaygroundSession } from '@/utils/playgroundSession';
import { isPromptFile, partitionDroppedFiles, readPromptFile } from '@/utils/promptFile';
import ImageSliceDialog from './ImageSliceDialog';
import ImageGenGalleryDialog from './ImageGenGalleryDialog';
import type {
    Endpoint,
    GenerationRun,
    ImportedImage,
    Quality,
    SelectedImage,
} from './ImageGenPlayground.types';
import {
    addReferences,
    downloadStem,
    formatBytes,
    reorderReferences,
    resultSrc,
    runImage,
} from './imageGenSession';
import {
    fullBleedDialogPaperSx,
    hoverRevealSx,
    overlayActionSx,
    overlayPlateSx,
    panelActionLabelSx,
    panelActionSx,
    zoomScrimSx,
} from './ImageGenPlayground.chrome';
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
// Marks a drag as "one of this row's thumbnails moving", so the row's own
// file-drop target can tell a reorder apart from images arriving from outside.
const REFERENCE_DND_TYPE = 'application/x-tingly-reference-index';
// The results strip is "what's happening right now", not a scrollback buffer —
// dragging through dozens of past generations to find one belongs in the
// overview (searchable, grid, newest first), not here. Capping the strip to
// its most recent items and handing off anything older to a single "open the
// overview" tile keeps the strip a status readout instead of a second archive.
const HISTORY_STRIP_VISIBLE = 6;

// Shared by the lightbox's overlay buttons — restyling the bar should be one edit.
const overlayIconSx = {
    color: 'common.white',
    bgcolor: 'rgba(255, 255, 255, 0.08)',
    '&:hover': { bgcolor: 'rgba(255, 255, 255, 0.16)' },
} as const;

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

// Reads a File into a base64 data URL, the same representation already used
// for generated images (`data:image/png;base64,...`) so reference thumbnails
// and outputs render through one code path.
const fileToDataUrl = (file: File): Promise<string> => new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
});

// Keep playground output while navigating between pages in the current app session.
// This deliberately stays in memory: base64 images can quickly exceed sessionStorage quotas.
let imageGenSessionRuns: GenerationRun[] = [];
let imageGenSessionImports: ImportedImage[] = [];

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
                            <Box className="image-preview-overlay" sx={zoomScrimSx}>
                                <ZoomIn sx={{ fontSize: 30 }} />
                            </Box>
                        </ButtonBase>
                        <Tooltip title={t('playground.useAsReference', { defaultValue: 'Use as reference' })}>
                            <IconButton
                                size="small"
                                onClick={onUseAsReference}
                                aria-label={t('playground.useAsReference', { defaultValue: 'Use as reference' })}
                                sx={{ position: 'absolute', bottom: 8, right: 8, ...overlayActionSx() }}
                            >
                                <Edit fontSize="small" />
                            </IconButton>
                        </Tooltip>
                        <IconButton
                            size="small"
                            onClick={onRemove}
                            aria-label={t('playground.removeImported', { defaultValue: 'Remove {{name}}', name: item.name })}
                            sx={{ position: 'absolute', top: 8, right: 8, ...overlayActionSx() }}
                        >
                            <Close fontSize="small" />
                        </IconButton>
                    </Box>
                </Stack>
            </CardContent>
        </Card>
    );
};

interface RunSourceStripProps {
    sources: string[];
    onOpen: (index: number) => void;
    // Puts one of these images back into the request as a reference. The
    // materials of a past run are the most likely input of the next one, so
    // getting them there is a click on the thumbnail, not a download and a
    // re-upload.
    onUseAsReference: (src: string) => void;
    // The in-flight card centres its content; the other two are left-aligned.
    align?: 'flex-start' | 'center';
}

// The images a run was built from, as thumbnails that open in the lightbox.
// Shown in every card state — while a run is in flight and after it failed is
// exactly when "what did I actually send?" needs an answer, and a retry that
// can't show its own materials asks the user to remember them.
const RunSourceStrip: React.FC<RunSourceStripProps> = ({ sources, onOpen, onUseAsReference, align = 'flex-start' }) => {
    const { t } = useTranslation();
    if (sources.length === 0) return null;
    return (
        <Stack
            direction="row"
            spacing={0.5}
            data-testid="imagegen-run-sources"
            sx={{ width: '100%', mt: 0.75, justifyContent: align, overflowX: 'auto', flexShrink: 0, scrollbarWidth: 'thin' }}
        >
            {sources.map((src, i) => (
                <Box
                    key={i}
                    sx={{
                        position: 'relative',
                        width: 36,
                        height: 36,
                        flexShrink: 0,
                        '&:hover .source-reuse, &:focus-within .source-reuse': { opacity: 1 },
                    }}
                >
                    <ButtonBase
                        onClick={() => onOpen(i)}
                        aria-label={t('playground.viewSourceImage', {
                            defaultValue: 'View original image {{number}}',
                            number: i + 1,
                        })}
                        sx={{
                            display: 'block',
                            width: '100%',
                            height: '100%',
                            borderRadius: 0.5,
                            overflow: 'hidden',
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
                    <Tooltip title={t('playground.useAsReference', { defaultValue: 'Use as reference' })}>
                        <IconButton
                            className="source-reuse"
                            size="small"
                            onClick={(event) => { event.stopPropagation(); onUseAsReference(src); }}
                            aria-label={t('playground.useAsReference', { defaultValue: 'Use as reference' })}
                            data-testid="imagegen-source-use-as-reference"
                            sx={{
                                ...overlayActionSx(16),
                                position: 'absolute',
                                bottom: -2,
                                right: -2,
                                ...hoverRevealSx,
                                '&:hover, &:focus-visible': { opacity: 1 },
                            }}
                        >
                            <Edit sx={{ fontSize: 10 }} />
                        </IconButton>
                    </Tooltip>
                </Box>
            ))}
        </Stack>
    );
};

// A results-panel header action. Three of them do not fit a phone at full
// width, so the label drops away below `sm` and the icon carries the button —
// the action itself never disappears, and the aria-label keeps saying what it
// is.
const PanelAction: React.FC<{
    label: string;
    icon: React.ReactNode;
    onClick: () => void;
    testId?: string;
}> = ({ label, icon, onClick, testId }) => (
    <Tooltip title={label}>
        <Button
            size="small"
            color="inherit"
            startIcon={icon}
            onClick={onClick}
            aria-label={label}
            data-testid={testId}
            sx={panelActionSx}
        >
            <Box component="span" sx={panelActionLabelSx}>{label}</Box>
        </Button>
    </Tooltip>
);

interface ReferenceThumbProps {
    image: ReferenceImage;
    index: number;
    total: number;
    dragging: boolean;
    dragOver: boolean;
    onOpen: () => void;
    onEditSketch: () => void;
    onRemove: () => void;
    onReorder: (from: number, to: number) => void;
    onMoveByKey: (event: React.KeyboardEvent, index: number) => void;
    // The row owns which thumbnail is moving and which one it is over; this one
    // only reports the pointer events that change that.
    onDragStart: () => void;
    onDragEnd: () => void;
    onDragEnter: () => void;
    onDragLeave: () => void;
}

// One image waiting in the request. It is a thumbnail, a drag handle and a
// drop target at once, which is why it lives here rather than inline in the
// row: reading how reordering behaves should not mean scrolling through the
// panel's render tree.
const ReferenceThumb: React.FC<ReferenceThumbProps> = ({
    image,
    index,
    total,
    dragging,
    dragOver,
    onOpen,
    onEditSketch,
    onRemove,
    onReorder,
    onMoveByKey,
    onDragStart,
    onDragEnd,
    onDragEnter,
    onDragLeave,
}) => {
    const { t } = useTranslation();
    // Only a thumbnail of this row reorders. Files arriving from outside carry
    // no such type and fall through to the row's own drop target, which adds
    // them as new references.
    const isReorder = (event: React.DragEvent) => event.dataTransfer.types.includes(REFERENCE_DND_TYPE);
    return (
        <Box
            draggable
            onDragStart={(event) => {
                event.stopPropagation();
                event.dataTransfer.effectAllowed = 'move';
                event.dataTransfer.setData(REFERENCE_DND_TYPE, String(index));
                onDragStart();
            }}
            onDragEnd={onDragEnd}
            onDragOver={(event) => {
                if (!isReorder(event)) return;
                event.preventDefault();
                event.stopPropagation();
                event.dataTransfer.dropEffect = 'move';
                onDragEnter();
            }}
            onDragLeave={onDragLeave}
            onDrop={(event) => {
                if (!isReorder(event)) return;
                event.preventDefault();
                event.stopPropagation();
                onReorder(Number(event.dataTransfer.getData(REFERENCE_DND_TYPE)), index);
                onDragEnd();
            }}
            sx={{
                position: 'relative',
                width: 56,
                height: 56,
                borderRadius: 1,
                flexShrink: 0,
                cursor: 'grab',
                opacity: dragging ? 0.4 : 1,
                outline: dragOver && !dragging ? '2px solid' : 'none',
                outlineColor: 'primary.main',
                outlineOffset: 1,
                '&:active': { cursor: 'grabbing' },
            }}
        >
            <ButtonBase
                data-reference-index={index}
                onClick={(event) => { event.stopPropagation(); onOpen(); }}
                onKeyDown={(event) => onMoveByKey(event, index)}
                aria-label={t('playground.openReferenceReorderable', {
                    defaultValue: 'Reference image {{number}} of {{total}} — open it, or move it with the arrow keys',
                    number: index + 1,
                    total,
                })}
                sx={{
                    width: '100%',
                    height: '100%',
                    display: 'block',
                    borderRadius: 1,
                    overflow: 'hidden',
                    '&:hover .reference-zoom, &:focus-visible .reference-zoom': { opacity: 1 },
                }}
            >
                <Box
                    component="img"
                    src={image.previewUrl}
                    alt={t('playground.referenceThumbAlt', { defaultValue: 'Reference image {{number}}', number: index + 1 })}
                    decoding="async"
                    sx={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
                />
                <Box className="reference-zoom" sx={zoomScrimSx}>
                    <ZoomIn fontSize="small" />
                </Box>
            </ButtonBase>
            {image.source === 'sketch' && (
                <Tooltip title={t('playground.sketch.editAction', { defaultValue: 'Edit sketch' })}>
                    <IconButton
                        size="small"
                        onClick={(event) => { event.stopPropagation(); onEditSketch(); }}
                        aria-label={t('playground.sketch.editAction', { defaultValue: 'Edit sketch' })}
                        sx={{ ...overlayActionSx(20), position: 'absolute', bottom: 2, right: 2 }}
                    >
                        <Create sx={{ fontSize: 13 }} />
                    </IconButton>
                </Tooltip>
            )}
            <IconButton
                size="small"
                onClick={(event) => { event.stopPropagation(); onRemove(); }}
                aria-label={t('playground.removeReferenceImage', { defaultValue: 'Remove reference image {{number}}', number: index + 1 })}
                sx={{ ...overlayActionSx(20), position: 'absolute', top: -6, right: -6 }}
            >
                <Close sx={{ fontSize: 14 }} />
            </IconButton>
        </Box>
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
    // The thumbnail being dragged and the one it is hovering over. Order is
    // part of the request — providers read the reference list in order — so it
    // has to be editable in place rather than by removing and re-adding.
    const [draggingReference, setDraggingReference] = useState<number | null>(null);
    const [dragOverReference, setDragOverReference] = useState<number | null>(null);
    const [runs, setRuns] = useState<GenerationRun[]>(() => imageGenSessionRuns);
    const [imported, setImported] = useState<ImportedImage[]>(() => imageGenSessionImports);
    const [selectedImage, setSelectedImage] = useState<SelectedImage | null>(null);
    const [sliceTarget, setSliceTarget] = useState<SelectedImage | null>(null);
    // The overview: the strip answers "what is happening now", this answers
    // "where is the one I made twenty images ago" without dragging a scrollbar.
    const [galleryOpen, setGalleryOpen] = useState(false);
    const [sketchTarget, setSketchTarget] = useState<SketchTarget>(null);
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
    const historyTrackRef = useRef<HTMLDivElement>(null);
    // Reusing a run refills this panel; on a narrow layout it sits above the
    // results strip and off-screen, so the refilled form is scrolled back
    // into view instead of leaving the user to wonder where the request went.
    const controlsPanelRef = useRef<HTMLDivElement>(null);
    const referenceFileInputRef = useRef<HTMLInputElement>(null);
    const importFileInputRef = useRef<HTMLInputElement>(null);
    const promptFileInputRef = useRef<HTMLInputElement>(null);
    const pendingCount = runs.filter((run) => run.status === 'pending').length;
    const { copied: promptCopied, copy: copyPrompt } = useCopyFeedback();
    // The in-flight request behind each pending card, so its Cancel button
    // can abort the fetch instead of leaving the user to wait out the
    // gateway's timeout on a provider that has stopped answering.
    const inFlightRef = useRef(new Map<string, AbortController>());

    // The session's two halves have one writer each: the module-level copy (what
    // survives navigation between pages) and the React state must move together,
    // and a caller that has to remember both in the right order eventually won't.
    const updateRuns = useCallback((updater: (currentRuns: GenerationRun[]) => GenerationRun[]) => {
        imageGenSessionRuns = updater(imageGenSessionRuns);
        setRuns(imageGenSessionRuns);
    }, []);

    const updateImports = useCallback((updater: (currentImports: ImportedImage[]) => ImportedImage[]) => {
        imageGenSessionImports = updater(imageGenSessionImports);
        setImported(imageGenSessionImports);
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
            if (savedRuns.length > 0) {
                updateRuns((current) => (current.length > 0 ? current : savedRuns.map((run) => (run.status === 'pending'
                    ? { ...run, status: 'failed', error: t('playground.interruptedByReload', { defaultValue: 'Interrupted by a page reload' }) }
                    : run))));
            }
            if (savedImports.length > 0) {
                updateImports((current) => (current.length > 0 ? current : savedImports));
            }
            setSessionRestored(true);
        });
        return () => { cancelled = true; };
    }, [t, updateImports, updateRuns]);

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
        const { next: accepted, ignored } = addReferences(
            [] as File[],
            incoming,
            Math.max(0, MAX_EDIT_REFERENCE_IMAGES - referenceImages.length),
            'ignore',
        );
        // Dropping images on the floor without saying so leaves the user
        // believing a request carries pictures it does not.
        if (ignored > 0) {
            showNotification(
                t('playground.referenceCapReached', {
                    defaultValue: 'Only {{max}} reference images fit — {{ignored}} were left out',
                    max: MAX_EDIT_REFERENCE_IMAGES,
                    ignored,
                }),
                'warning',
            );
        }
        if (accepted.length === 0) return;
        const withPreviews = await Promise.all(accepted.map(async (file): Promise<ReferenceImage> => {
            const previewUrl = await fileToDataUrl(file);
            return { file, previewUrl, source: 'upload', ...(await readImageSize(previewUrl) ?? {}) };
        }));
        setReferenceImages((current) => addReferences(current, withPreviews, MAX_EDIT_REFERENCE_IMAGES, 'ignore').next);
    }, [referenceImages.length, showNotification, t]);

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
        updateImports((current) => [...current, ...items]);
    }, [updateImports]);

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
    }, []);

    // Deleting is one-way — the image leaves the session for good — so every
    // path into it (the strip's own button, the gallery tile's button) goes
    // through the same confirm below rather than firing immediately.
    const [pendingRemoval, setPendingRemoval] = useState<{ kind: 'run' | 'import'; id: string } | null>(null);

    const removeImport = useCallback((id: string) => {
        updateImports((current) => current.filter((item) => item.id !== id));
    }, [updateImports]);

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

    const handleRemoveReferenceImage = useCallback((index: number) => {
        setReferenceImages((current) => current.filter((_, i) => i !== index));
    }, []);

    // Moves one reference to another slot, keeping every other image's relative
    // order — the same result as dragging a card in a list.
    const handleReorderReference = useCallback((from: number, to: number) => {
        setReferenceImages((current) => reorderReferences(current, from, to));
    }, []);

    // The keyboard's version of the same drag: with a thumbnail focused, the
    // arrow keys walk it along the row. Focus follows the image it moved, not
    // the slot it left.
    const handleReferenceKeyDown = useCallback((event: React.KeyboardEvent, index: number) => {
        const delta = event.key === 'ArrowLeft' ? -1 : event.key === 'ArrowRight' ? 1 : 0;
        if (delta === 0) return;
        const target = index + delta;
        if (target < 0 || target >= referenceImages.length) return;
        event.preventDefault();
        event.stopPropagation();
        handleReorderReference(index, target);
        requestAnimationFrame(() => {
            const moved = document.querySelector<HTMLElement>(`[data-reference-index="${target}"]`);
            moved?.focus();
        });
    }, [handleReorderReference, referenceImages.length]);

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
            // The user pointed at this image, so it goes in; at the cap the
            // oldest makes room — and that is said out loud, because a reference
            // vanishing from the row unannounced is the request quietly changing
            // behind the user's back.
            if (referenceImages.length >= MAX_EDIT_REFERENCE_IMAGES) {
                showNotification(
                    t('playground.referenceEvicted', {
                        defaultValue: 'Added as a reference — the oldest one made room (max {{max}})',
                        max: MAX_EDIT_REFERENCE_IMAGES,
                    }),
                    'info',
                );
            }
            setReferenceImages((current) => addReferences(current, [next], MAX_EDIT_REFERENCE_IMAGES, 'evict').next);
        } catch {
            showNotification(
                t('playground.referenceLoadFailed', { defaultValue: 'Could not use this image as a reference' }),
                'error',
            );
        }
    }, [referenceImages.length, showNotification, t]);

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
            await downloadImage(image.src, downloadStem(image, slugify));
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

    // A run's references only survive as data URLs, so they are read back into
    // files here — the shape both retry (same request again) and re-entry (same
    // request, editable) need. The bytes and the pixel size come from two
    // independent decodes of the same data URL, so they are awaited together
    // rather than one after the other.
    const runSourcesToReferences = useCallback((run: GenerationRun): Promise<ReferenceImage[]> => Promise.all(
        (run.sourceImages ?? []).map(async (src, index): Promise<ReferenceImage> => {
            const [blob, size] = await Promise.all([fetchBlob(src), readImageSize(src)]);
            return {
                file: new File([blob], `reference-${index + 1}.png`, { type: blob.type || 'image/png' }),
                previewUrl: src,
                source: 'upload',
                ...(size ?? {}),
            };
        }),
    ), []);

    const handleRetry = useCallback(async (run: GenerationRun) => {
        try {
            const sources = await runSourcesToReferences(run);
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
    }, [runGeneration, runSourcesToReferences, showNotification, t]);

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
    }, [models, runSourcesToReferences, showNotification, t]);

    // Opens one of a run's source images in the same lightbox its outputs use.
    const handleOpenRunSource = useCallback((run: GenerationRun, index: number) => {
        const src = run.sourceImages?.[index];
        if (src) setSelectedImage(runImage(run, 'source', index, src));
    }, []);

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
    }, [updateImports, updateRuns]);

    const removeRun = useCallback((id: string) => {
        updateRuns((currentRuns) => currentRuns.filter((run) => run.id !== id));
    }, [updateRuns]);

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

    // The per-run actions that read the same in every card state: take the
    // prompt away as text (a prompt should never have to be selected by hand),
    // put the whole request back in the panel, and drop the card. While a run
    // is in flight Cancel is what removes it, so `onRemove` is left out there.
    const reuseLabel = t('playground.reuse.action', { defaultValue: 'Edit this request' });
    const copyPromptLabel = t('playground.copyPrompt', { defaultValue: 'Copy prompt' });
    const promptCopiedLabel = t('playground.promptCopied', { defaultValue: 'Copied' });
    const removeRunLabel = t('playground.removeRun', { defaultValue: 'Remove this generation' });

    const renderRunActions = (run: GenerationRun, onRemove?: () => void) => (
        <Stack direction="row" spacing={0} sx={{ flexShrink: 0, alignItems: 'center' }}>
            <CopyIconButton
                value={run.prompt}
                label={copyPromptLabel}
                copiedLabel={promptCopiedLabel}
                iconSize={16}
                color="text.disabled"
                sx={{ p: 0.5, '&:hover': { color: 'text.primary' } }}
            />
            <Tooltip title={reuseLabel}>
                <IconButton
                    size="small"
                    onClick={() => { void handleReuseRun(run); }}
                    aria-label={reuseLabel}
                    data-testid="imagegen-reuse-run"
                    sx={{ p: 0.5, color: 'text.disabled', '&:hover': { color: 'text.primary' } }}
                >
                    <RestartAlt sx={{ fontSize: 16 }} />
                </IconButton>
            </Tooltip>
            {onRemove && (
                // A destructive action, so it gets its own hover colour
                // (error, not the neutral text.primary the other two use) —
                // that is also what makes it findable, not just clickable.
                <Tooltip title={removeRunLabel}>
                    <IconButton
                        size="small"
                        onClick={onRemove}
                        aria-label={removeRunLabel}
                        data-testid="imagegen-remove-run"
                        sx={{ p: 0.5, color: 'text.disabled', '&:hover': { color: 'error.main' } }}
                    >
                        <Close sx={{ fontSize: 16 }} />
                    </IconButton>
                </Tooltip>
            )}
        </Stack>
    );

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

    // The strip only ever shows the tail (oldest-to-newest, so the newest —
    // what the user just did — is the one nearest the "Generate" button it
    // came from). Everything older collapses into the "view all" tile at the
    // far end instead of staying draggable here.
    const visibleTimeline = useMemo(
        () => (timeline.length > HISTORY_STRIP_VISIBLE ? timeline.slice(-HISTORY_STRIP_VISIBLE) : timeline),
        [timeline],
    );
    const olderTimelineCount = timeline.length - visibleTimeline.length;

    // The run behind the image currently in the lightbox, and every image that
    // run touched: what went in, then what came out. Without this an output on
    // screen says nothing about what it was made from — the lightbox is exactly
    // where "what did I reference here?" gets asked, and closing it to go read
    // the card is not an answer.
    const lightboxRun = useMemo(
        () => (selectedImage?.runId ? runs.find((run) => run.id === selectedImage.runId) : undefined),
        [runs, selectedImage?.runId],
    );
    const lightboxFilm = useMemo(() => {
        if (!lightboxRun) return [];
        const sources = (lightboxRun.sourceImages ?? []).map((src, index) => ({ src, kind: 'source' as const, index }));
        const outputs = lightboxRun.images
            .map((image, index) => ({ src: resultSrc(image), kind: 'output' as const, index }))
            .filter((item) => item.src);
        // One image with nothing to compare it to is not a filmstrip.
        return sources.length > 0 ? [...sources, ...outputs] : [];
    }, [lightboxRun]);

    const showLightboxFrame = useCallback((frame: (typeof lightboxFilm)[number]) => {
        if (lightboxRun) setSelectedImage(runImage(lightboxRun, frame.kind, frame.index, frame.src));
    }, [lightboxRun]);

    // ←/→ walk the filmstrip, the same gesture the reference row uses. Without
    // it, comparing an output against its original is a mouse-only move.
    const handleLightboxKeyDown = useCallback((event: React.KeyboardEvent) => {
        const delta = event.key === 'ArrowLeft' ? -1 : event.key === 'ArrowRight' ? 1 : 0;
        if (delta === 0 || lightboxFilm.length < 2 || !selectedImage) return;
        const current = lightboxFilm.findIndex(
            (frame) => frame.kind === selectedImage.kind && frame.index === selectedImage.index,
        );
        if (current === -1) return;
        event.preventDefault();
        // Wraps, so the strip has no dead end at either edge.
        showLightboxFrame(lightboxFilm[(current + delta + lightboxFilm.length) % lightboxFilm.length]);
    }, [lightboxFilm, selectedImage, showLightboxFrame]);

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
                        ref={controlsPanelRef}
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
                                                <ReferenceThumb
                                                    key={index}
                                                    image={ref}
                                                    index={index}
                                                    total={referenceImages.length}
                                                    dragging={draggingReference === index}
                                                    dragOver={dragOverReference === index}
                                                    onOpen={() => handleOpenReference(index)}
                                                    onEditSketch={() => handleOpenSketch(index)}
                                                    onRemove={() => handleRemoveReferenceImage(index)}
                                                    onReorder={handleReorderReference}
                                                    onMoveByKey={handleReferenceKeyDown}
                                                    onDragStart={() => setDraggingReference(index)}
                                                    onDragEnd={() => { setDraggingReference(null); setDragOverReference(null); }}
                                                    onDragEnter={() => setDragOverReference(index)}
                                                    onDragLeave={() => setDragOverReference((current) => (current === index ? null : current))}
                                                />
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
                                        {[
                                            t('playground.referenceHint', {
                                                defaultValue: 'Up to {{max}} images · PNG, JPEG, or WebP · sent via images/edits',
                                                max: MAX_EDIT_REFERENCE_IMAGES,
                                            }),
                                            // Order is only worth mentioning once there is an order.
                                            referenceImages.length > 1 && t('playground.referenceReorderHint', {
                                                defaultValue: 'drag a thumbnail to reorder',
                                            }),
                                        ].filter(Boolean).join(' · ')}
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
                                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                                    <Typography variant="subtitle2" sx={{ minWidth: 0 }}>
                                        {t('playground.sessionOutputs', { defaultValue: 'Session images' })}
                                    </Typography>
                                    <Typography
                                        variant="caption"
                                        sx={{
                                            color: "text.secondary",
                                            flexShrink: 0,
                                            whiteSpace: 'nowrap',
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
                                        strip filling up must not take them away. Plus the way
                                        out of the strip itself, which is what a session with
                                        thirty images actually needs. */}
                                    <Stack direction="row" spacing={0.5} sx={{ flexShrink: 0, ml: 'auto' }}>
                                        <PanelAction
                                            label={t('playground.gallery.action', { defaultValue: 'Overview' })}
                                            icon={<ViewGallery fontSize="small" />}
                                            onClick={() => setGalleryOpen(true)}
                                            testId="imagegen-open-gallery"
                                        />
                                        <PanelAction
                                            label={t('playground.referenceBrowse', { defaultValue: 'Browse' })}
                                            icon={<FileUpload fontSize="small" />}
                                            onClick={() => importFileInputRef.current?.click()}
                                        />
                                        <PanelAction
                                            label={t('playground.referencePaste', { defaultValue: 'Paste' })}
                                            icon={<ContentPaste fontSize="small" />}
                                            onClick={() => { void handleImportFromClipboard(); }}
                                        />
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
                                    {olderTimelineCount > 0 && (
                                        <ButtonBase
                                            onClick={() => setGalleryOpen(true)}
                                            data-testid="imagegen-history-view-all"
                                            aria-label={t('playground.gallery.viewOlderHint', {
                                                defaultValue: 'View all {{count}} images in the overview',
                                                count: timeline.length,
                                            })}
                                            sx={{
                                                flex: '0 0 84px',
                                                height: '100%',
                                                display: 'flex',
                                                flexDirection: 'column',
                                                alignItems: 'center',
                                                justifyContent: 'center',
                                                gap: 0.5,
                                                borderRadius: 1.5,
                                                border: '1px dashed',
                                                borderColor: 'divider',
                                                bgcolor: 'background.paper',
                                                color: 'text.secondary',
                                                scrollSnapAlign: 'start',
                                                '&:hover': { color: 'primary.main', borderColor: 'primary.main' },
                                            }}
                                        >
                                            <ViewGallery fontSize="small" />
                                            <Typography variant="caption" sx={{ fontWeight: 600, lineHeight: 1.2 }}>
                                                +{olderTimelineCount}
                                            </Typography>
                                            <Typography variant="caption" sx={{ fontSize: 10, color: 'text.disabled' }}>
                                                {t('playground.gallery.action', { defaultValue: 'Overview' })}
                                            </Typography>
                                        </ButtonBase>
                                    )}
                                    {visibleTimeline.map((entry) => (entry.kind === 'import' ? (
                                        <ImportedImageCard
                                            key={entry.item.id}
                                            item={entry.item}
                                            onOpen={() => handleOpenImport(entry.item)}
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
                                                    {/* A running request is still a request to copy or to
                                                        fork into the next one — the actions do not wait
                                                        for it to land. */}
                                                    <Box sx={{ alignSelf: 'flex-end', mt: -0.5, mr: -0.5 }}>
                                                        {renderRunActions(run)}
                                                    </Box>
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
                                                    <RunSourceStrip
                                                        sources={run.sourceImages ?? []}
                                                        align="center"
                                                        onOpen={(index) => handleOpenRunSource(run, index)}
                                                        onUseAsReference={(src) => { void handleUseAsReference(src); }}
                                                    />
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
                                                        <Box sx={{ mr: -0.5, mt: -0.5 }}>
                                                            {renderRunActions(run, () => handleRemoveRun(run.id))}
                                                        </Box>
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
                                                    {/* Retrying blind is not retrying: the images the failed
                                                        request was built from stay on the card, openable in
                                                        the same lightbox as any other image here. */}
                                                    <RunSourceStrip
                                                        sources={run.sourceImages ?? []}
                                                        onOpen={(index) => handleOpenRunSource(run, index)}
                                                        onUseAsReference={(src) => { void handleUseAsReference(src); }}
                                                    />
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
                                                        <Box sx={{ mr: -0.75, mt: -0.75 }}>
                                                            {renderRunActions(run, () => handleRemoveRun(run.id))}
                                                        </Box>
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
                                                    <RunSourceStrip
                                                        sources={run.sourceImages ?? []}
                                                        onOpen={(index) => handleOpenRunSource(run, index)}
                                                        onUseAsReference={(src) => { void handleUseAsReference(src); }}
                                                    />
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
                                                        const src = resultSrc(image);
                                                        return src ? (
                                                            <Box
                                                                key={`${run.id}-${index}`}
                                                                sx={{ position: 'relative', width: '100%', height: '100%', borderRadius: 1, overflow: 'hidden', bgcolor: 'action.hover' }}
                                                            >
                                                                <ButtonBase
                                                                    onClick={() => setSelectedImage(runImage(run, 'output', index, src))}
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
                                                                    <Box className="image-preview-overlay" sx={zoomScrimSx}>
                                                                        <ZoomIn sx={{ fontSize: 30 }} />
                                                                    </Box>
                                                                    <Box
                                                                        sx={{
                                                                            ...overlayActionSx(),
                                                                            position: 'absolute',
                                                                            top: 8,
                                                                            right: 8,
                                                                            display: 'flex',
                                                                            alignItems: 'center',
                                                                            justifyContent: 'center',
                                                                            borderRadius: '50%',
                                                                        }}
                                                                    >
                                                                        <ZoomIn fontSize="small" />
                                                                    </Box>
                                                                </ButtonBase>
                                                                <IconButton
                                                                    size="small"
                                                                    onClick={(event) => { event.stopPropagation(); void handleUseAsReference(src); }}
                                                                    aria-label={t('playground.useAsReference', { defaultValue: 'Use as reference' })}
                                                                    sx={{ position: 'absolute', bottom: 8, right: 8, ...overlayActionSx() }}
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
                onKeyDown={handleLightboxKeyDown}
                maxWidth={false}
                fullWidth
                slotProps={{
                    paper: {
                        sx: {
                            ...fullBleedDialogPaperSx,
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
                        {/* Zoomed in on a result is exactly where "now change one
                            word and run it again" happens — the request that made
                            this image goes back into the panel from here too. */}
                        {lightboxRun && (
                            <Tooltip title={reuseLabel}>
                                <IconButton
                                    onClick={() => {
                                        void handleReuseRun(lightboxRun);
                                        setSelectedImage(null);
                                    }}
                                    aria-label={reuseLabel}
                                    sx={overlayIconSx}
                                >
                                    <RestartAlt fontSize="small" />
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
                        position: 'relative',
                    }}
                >
                    {/* Top-left, over the artwork: the run's originals first, then
                        what it produced, the one on screen ringed. Clicking a frame
                        swaps the lightbox to it, so comparing an output against the
                        image it was made from is one click each way. */}
                    {lightboxFilm.length > 0 && (
                        <Stack
                            data-testid="imagegen-lightbox-film"
                            spacing={0.75}
                            sx={{
                                ...overlayPlateSx,
                                position: 'absolute',
                                top: 12,
                                left: 12,
                                maxHeight: 'calc(100% - 24px)',
                                overflowY: 'auto',
                                p: 0.75,
                                scrollbarWidth: 'thin',
                            }}
                        >
                            {lightboxFilm.map((frame, position) => {
                                const active = selectedImage?.kind === frame.kind && selectedImage.index === frame.index;
                                const label = frame.kind === 'source'
                                    ? t('playground.originalBadge', { defaultValue: 'Original' })
                                    : t('playground.generatedBadge', { defaultValue: 'Generated' });
                                // The first frame of each group carries the group's word;
                                // repeating it down the column would be noise.
                                const showLabel = position === 0 || lightboxFilm[position - 1].kind !== frame.kind;
                                return (
                                    <Box key={`${frame.kind}-${frame.index}`}>
                                        {showLabel && (
                                            <Typography
                                                variant="caption"
                                                sx={{ display: 'block', mb: 0.25, color: 'grey.400', fontSize: 10, lineHeight: 1.4 }}
                                            >
                                                {label}
                                            </Typography>
                                        )}
                                        <Tooltip title={label} placement="right">
                                            <ButtonBase
                                                onClick={() => showLightboxFrame(frame)}
                                                aria-label={frame.kind === 'source'
                                                    ? t('playground.viewSourceImage', {
                                                        defaultValue: 'View original image {{number}}',
                                                        number: frame.index + 1,
                                                    })
                                                    : t('playground.openResult', {
                                                        defaultValue: 'Open generated image {{number}}',
                                                        number: frame.index + 1,
                                                    })}
                                                aria-current={active}
                                                sx={{
                                                    display: 'block',
                                                    // Smaller on a phone, where the strip shares the
                                                    // width with the artwork it sits over.
                                                    width: { xs: 36, sm: 48 },
                                                    height: { xs: 36, sm: 48 },
                                                    borderRadius: 1,
                                                    overflow: 'hidden',
                                                    outline: active ? '2px solid' : '1px solid',
                                                    outlineColor: active ? 'primary.main' : 'rgba(255, 255, 255, 0.24)',
                                                    outlineOffset: -1,
                                                    opacity: active ? 1 : 0.72,
                                                    transition: 'opacity 0.16s ease-out',
                                                    '&:hover, &:focus-visible': { opacity: 1 },
                                                }}
                                            >
                                                <Box
                                                    component="img"
                                                    src={frame.src}
                                                    alt=""
                                                    sx={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
                                                />
                                            </ButtonBase>
                                        </Tooltip>
                                    </Box>
                                );
                            })}
                        </Stack>
                    )}
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
            <ImageGenGalleryDialog
                open={galleryOpen}
                runs={runs}
                imported={imported}
                onClose={() => setGalleryOpen(false)}
                onOpenOutput={(run, imageIndex, src) => setSelectedImage(runImage(run, 'output', imageIndex, src))}
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
