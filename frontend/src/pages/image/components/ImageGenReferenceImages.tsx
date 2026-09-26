import { useRef } from 'react';
import { Box, Button, ButtonBase, IconButton, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Brush, Close, ContentPaste, Create, FileUpload, PhotoLibrary, ZoomIn } from '@/components/icons';
import { overlayActionSx, zoomScrimSx } from './ImageGenPlayground.chrome';
import type { ReferenceMask } from './ImageGenPlayground.types';
import type { SketchLayers } from './SketchCanvasDialog';

// Matches the Codex-native imagegen tool's reference-image cap (see
// .design/imageedit.md) — the common denominator across providers behind
// this scenario.
export const MAX_EDIT_REFERENCE_IMAGES = 5;

// Marks a drag as "one of this row's thumbnails moving", so the row's own
// file-drop target can tell a reorder apart from images arriving from outside.
export const REFERENCE_DND_TYPE = 'application/x-tingly-reference-index';

export interface ReferenceImage {
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
    // The region of THIS image the model may repaint. An attribute of the
    // image, not a mode and not a fourth kind of reference: it means nothing
    // away from the pixels it was painted on, so it travels with them, follows
    // them when the row is reordered, and is dropped when they are. Only the
    // first image's mask is sent — the API applies a mask to the first image —
    // which the row says out loud when one has been dragged off the front.
    // See .design/image-mask.md.
    mask?: ReferenceMask;
}

interface ReferenceThumbProps {
    image: ReferenceImage;
    index: number;
    total: number;
    dragging: boolean;
    dragOver: boolean;
    onOpen: () => void;
    onEditSketch: () => void;
    onEditMask: () => void;
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
    onEditMask,
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
                {image.mask ? (
                    <Box
                        component="img"
                        src={image.mask.previewUrl}
                        alt=""
                        aria-hidden
                        sx={{
                            position: 'absolute',
                            inset: 0,
                            width: '100%',
                            height: '100%',
                            objectFit: 'cover',
                            // Dimmed once this image is no longer the one the
                            // mask is sent with: the tint still says the work
                            // was not lost, but it stops looking live.
                            opacity: index === 0 ? 0.55 : 0.25,
                            pointerEvents: 'none',
                        }}
                    />
                ) : null}
                <Box className="reference-zoom" sx={zoomScrimSx}>
                    <ZoomIn fontSize="small" />
                </Box>
            </ButtonBase>
            {/* Painting a mask starts on the first image, because that is the
                one the API applies it to. A mask that is already painted keeps
                its button wherever the image is dragged, so it can still be
                edited or removed from the slot it landed in. */}
            {(index === 0 || image.mask) && (
                <Tooltip
                    title={image.mask
                        ? (index === 0
                            ? t('playground.mask.editAction', { defaultValue: 'Edit mask' })
                            : t('playground.mask.inactive', { defaultValue: 'Only the first image\u2019s mask is sent — drag this one to the front to use it' }))
                        : t('playground.mask.addAction', { defaultValue: 'Mask an area to change' })}
                >
                    <IconButton
                        size="small"
                        onClick={(event) => { event.stopPropagation(); onEditMask(); }}
                        aria-label={image.mask
                            ? t('playground.mask.editAction', { defaultValue: 'Edit mask' })
                            : t('playground.mask.addAction', { defaultValue: 'Mask an area to change' })}
                        sx={{
                            ...overlayActionSx(20),
                            position: 'absolute',
                            bottom: 2,
                            left: 2,
                            ...(image.mask && index === 0
                                ? { bgcolor: 'primary.main', '&:hover': { bgcolor: 'primary.dark' } }
                                : {}),
                        }}
                    >
                        <Brush sx={{ fontSize: 13 }} />
                    </IconButton>
                </Tooltip>
            )}
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

interface ReferenceImagesRowProps {
    referenceImages: ReferenceImage[];
    // Routes a drop on the row: a text file becomes the prompt, images land here.
    onDropFiles: (files: FileList | File[], onImages: (images: File[]) => void) => void;
    onAddReferenceImages: (files: FileList | File[]) => void;
    onOpenPromptFile: (file: File) => void;
    // The prompt-file input is shared with the prompt field's adornment and
    // the prompt editor dialog, so its ref stays with the panel.
    promptFileInputRef: React.RefObject<HTMLInputElement | null>;
    onOpenReference: (index: number) => void;
    onEditSketch: (index: number | null) => void;
    // Opens the picker over images kept in the Image library.
    onOpenLibrary: () => void;
    onEditMask: (index: number) => void;
    onRemoveReference: (index: number) => void;
    onReorder: (from: number, to: number) => void;
    onMoveByKey: (event: React.KeyboardEvent, index: number) => void;
    draggingReference: number | null;
    dragOverReference: number | null;
    onDragStart: (index: number) => void;
    onDragEnd: () => void;
    onDragEnter: (index: number) => void;
    onDragLeave: (index: number) => void;
}

// The reference-image row: an empty one-line invitation, or a thumbnail strip
// with drag-to-reorder plus the ways in (browse / paste / sketch) once images
// are waiting in the request.
export const ReferenceImagesRow: React.FC<ReferenceImagesRowProps> = ({
    referenceImages,
    onDropFiles,
    onAddReferenceImages,
    onOpenPromptFile,
    promptFileInputRef,
    onOpenReference,
    onEditSketch,
    onOpenLibrary,
    onEditMask,
    onRemoveReference,
    onReorder,
    onMoveByKey,
    draggingReference,
    dragOverReference,
    onDragStart,
    onDragEnd,
    onDragEnter,
    onDragLeave,
}) => {
    const { t } = useTranslation();
    const referenceFileInputRef = useRef<HTMLInputElement>(null);
    // Two different facts: the first image carries a mask (it will be sent),
    // or some other image does (it will not, and the row has to say so rather
    // than let the tint imply otherwise).
    const hasMaskedReference = referenceImages[0]?.mask !== undefined;
    const hasStrandedMask = !hasMaskedReference && referenceImages.some((ref) => ref.mask !== undefined);
    // The four ways a reference image gets here, as equals. Drop is not in
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
        },
        {
            key: 'sketch',
            label: t('playground.sketch.action', { defaultValue: 'Sketch' }),
            icon: <Create fontSize="small" />,
            onClick: () => onEditSketch(null),
        },
        {
            key: 'library',
            label: t('imageLibrary.referenceSource', { defaultValue: 'Library' }),
            icon: <PhotoLibrary fontSize="small" />,
            onClick: onOpenLibrary,
        },
    ];
    return (
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
                        onDropFiles(event.dataTransfer.files, (images) => onAddReferenceImages(images));
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
                                onClick={(event) => { event.stopPropagation(); source.onClick?.(); }}
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
                                onOpen={() => onOpenReference(index)}
                                onEditSketch={() => onEditSketch(index)}
                                onEditMask={() => onEditMask(index)}
                                onRemove={() => onRemoveReference(index)}
                                onReorder={onReorder}
                                onMoveByKey={onMoveByKey}
                                onDragStart={() => onDragStart(index)}
                                onDragEnd={onDragEnd}
                                onDragEnter={() => onDragEnter(index)}
                                onDragLeave={() => onDragLeave(index)}
                            />
                        ))}
                        {referenceImages.length < MAX_EDIT_REFERENCE_IMAGES && referenceSources.map((source) => (
                            <Tooltip key={source.key} title={source.label}>
                                <ButtonBase
                                    onClick={(event) => { event.stopPropagation(); source.onClick?.(); }}
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
                        hasMaskedReference
                            ? t('playground.mask.referenceHint', {
                                defaultValue: 'The tinted area of the first image is what the model may change · sent via images/edits',
                            })
                            : t('playground.referenceHint', {
                                defaultValue: 'Up to {{max}} images · PNG, JPEG, or WebP · sent via images/edits',
                                max: MAX_EDIT_REFERENCE_IMAGES,
                            }),
                        // A mask dragged off the front is still painted but no
                        // longer sent, and silence would read as "applied".
                        hasStrandedMask && t('playground.mask.strandedHint', {
                            defaultValue: 'only the first image\u2019s mask is sent',
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
                    if (file) onOpenPromptFile(file);
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
                    if (event.target.files?.length) onAddReferenceImages(event.target.files);
                    event.target.value = '';
                }}
            />
        </Box>
    );
};

export default ReferenceThumb;
