import { Box, Button, ButtonBase, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useRef } from 'react';
import { ContentPaste, FileUpload, Photo, ViewGallery } from '@/components/icons';
import GenerationRunCard from './GenerationRunCard';
import ImportedImageCard from './ImportedImageCard';
import PanelAction from './PanelAction';
import type { GenerationRun, ImportedImage, SelectedImage } from './ImageGenPlayground.types';

// One timeline for the results panel: images brought in to work on and
// images the model produced, in the order they arrived. Two lists would
// make "where did my image go" a question the user has to ask.
export type ImageGenTimelineEntry =
    | { kind: 'import'; at: number; item: ImportedImage }
    | { kind: 'run'; at: number; run: GenerationRun };

interface ImageGenResultsPanelProps {
    timeline: ImageGenTimelineEntry[];
    visibleTimeline: ImageGenTimelineEntry[];
    runsCount: number;
    importedCount: number;
    olderTimelineCount: number;
    desktopPanelHeight: string | number;
    // Attached to the scrolling strip so the session hook can keep the newest
    // card in view as new ones arrive.
    historyTrackRef: React.RefObject<HTMLDivElement | null>;
    onDropFiles: (files: FileList | File[], onImages: (images: File[]) => void) => void;
    onImportFiles: (files: FileList | File[]) => void;
    onImportFromClipboard: () => void;
    onOpenGallery: () => void;
    onOpenImport: (item: ImportedImage) => void;
    onUseAsReference: (src: string) => void;
    onRemoveImport: (id: string) => void;
    onCancelRun: (id: string) => void;
    onRetryRun: (run: GenerationRun) => void;
    onReuseRun: (run: GenerationRun) => void;
    onOpenRunSource: (run: GenerationRun, index: number) => void;
    onOpenOutput: (image: SelectedImage) => void;
    onRemoveRun: (id: string) => void;
}

// The results half of the playground: an empty-state invitation when the
// session is empty, otherwise the header (counts + ways in) over a
// horizontally scrolling strip of imported images and generation runs,
// oldest to newest, with an "overview" tile collapsing everything older
// than the visible tail.
const ImageGenResultsPanel: React.FC<ImageGenResultsPanelProps> = ({
    timeline,
    visibleTimeline,
    runsCount,
    importedCount,
    olderTimelineCount,
    desktopPanelHeight,
    historyTrackRef,
    onDropFiles,
    onImportFiles,
    onImportFromClipboard,
    onOpenGallery,
    onOpenImport,
    onUseAsReference,
    onRemoveImport,
    onCancelRun,
    onRetryRun,
    onReuseRun,
    onOpenRunSource,
    onOpenOutput,
    onRemoveRun,
}) => {
    const { t } = useTranslation();
    const importFileInputRef = useRef<HTMLInputElement>(null);
    return (
        <Box
            data-testid="imagegen-preview-panel"
            onDragOver={(event) => event.preventDefault()}
            onDrop={(event) => {
                event.preventDefault();
                if (event.dataTransfer.files?.length) {
                    onDropFiles(event.dataTransfer.files, onImportFiles);
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
                    if (event.target.files?.length) onImportFiles(event.target.files);
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
                            onClick={onImportFromClipboard}
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
                                runsCount > 0 && (runsCount === 1
                                    ? t('playground.runCountOne', { defaultValue: '1 generation' })
                                    : t('playground.runCount', {
                                        defaultValue: '{{count}} generations',
                                        count: runsCount,
                                    })),
                                importedCount > 0 && t('playground.importCount', {
                                    defaultValue: '{{count}} imported',
                                    count: importedCount,
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
                                onClick={onOpenGallery}
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
                                onClick={onImportFromClipboard}
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
                            // A size container, so cards can size themselves
                            // from the strip's height (`cqh`, see stripCardBasis).
                            containerType: 'size',
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
                                onClick={onOpenGallery}
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
                                onOpen={() => onOpenImport(entry.item)}
                                onUseAsReference={() => onUseAsReference(entry.item.src)}
                                onRemove={() => onRemoveImport(entry.item.id)}
                            />
                        ) : (
                            <GenerationRunCard
                                key={entry.run.id}
                                run={entry.run}
                                onCancel={onCancelRun}
                                onRetry={onRetryRun}
                                onReuse={onReuseRun}
                                onOpenSource={onOpenRunSource}
                                onUseAsReference={onUseAsReference}
                                onOpenOutput={onOpenOutput}
                                onRemove={onRemoveRun}
                            />
                        )))}
                    </Box>
                </Stack>
            )}
        </Box>
    );
};

export default ImageGenResultsPanel;
