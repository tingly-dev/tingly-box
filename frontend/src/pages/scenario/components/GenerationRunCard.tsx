import {
    Box,
    Button,
    ButtonBase,
    Card,
    CardContent,
    CircularProgress,
    IconButton,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { CopyIconButton } from '@/components/CopyIconButton';
import { Close, Edit, ErrorOutline, Refresh, RestartAlt, ZoomIn } from '@/components/icons';
import { overlayActionSx, zoomScrimSx } from './ImageGenPlayground.chrome';
import { resultSrc, runGridLayout, runImage } from './imageGenSession';
import RunSourceStrip from './RunSourceStrip';
import type { GenerationRun, SelectedImage } from './ImageGenPlayground.types';

interface GenerationRunCardProps {
    run: GenerationRun;
    onCancel: (id: string) => void;
    onRetry: (run: GenerationRun) => void;
    // Puts the run's whole request back into the panel.
    onReuse: (run: GenerationRun) => void;
    onOpenSource: (run: GenerationRun, index: number) => void;
    onUseAsReference: (src: string) => void;
    onOpenOutput: (image: SelectedImage) => void;
    onRemove: (id: string) => void;
}

// The per-run actions that read the same in every card state: take the
// prompt away as text (a prompt should never have to be selected by hand),
// put the whole request back in the panel, and drop the card. While a run
// is in flight Cancel is what removes it, so `onRemove` is left out there.
const GenerationRunCard: React.FC<GenerationRunCardProps> = ({
    run,
    onCancel,
    onRetry,
    onReuse,
    onOpenSource,
    onUseAsReference,
    onOpenOutput,
    onRemove,
}) => {
    const { t } = useTranslation();
    const reuseLabel = t('playground.reuse.action', { defaultValue: 'Edit this request' });
    const copyPromptLabel = t('playground.copyPrompt', { defaultValue: 'Copy prompt' });
    const promptCopiedLabel = t('playground.promptCopied', { defaultValue: 'Copied' });
    const removeRunLabel = t('playground.removeRun', { defaultValue: 'Remove this generation' });
    // The request as it went over the wire, so the line doubles as the answer
    // to "what do I send to get this": n only when it was more than one.
    const requested = Math.max(run.count ?? 1, 1);
    const runMeta = `${run.model} · ${run.size} · ${run.quality} · images/${run.endpoint}${run.mask ? ' · mask' : ''}${requested > 1 ? ` · n=${requested}` : ''}`;
    // One slot per requested image, from the moment the run starts: the card
    // has its final shape while it is pending, results land in their slots,
    // and a slot that stays empty is a missing image the user can see in
    // place (a provider capped n, or one of Codex's parallel calls failed)
    // rather than one that silently isn't there.
    const slotCount = Math.max(requested, run.images.length);
    const layout = runGridLayout(slotCount);

    const renderRunActions = (runActions: GenerationRun, onRemoveClick?: () => void) => (
        <Stack direction="row" spacing={0} sx={{ flexShrink: 0, alignItems: 'center' }}>
            <CopyIconButton
                value={runActions.prompt}
                label={copyPromptLabel}
                copiedLabel={promptCopiedLabel}
                iconSize={16}
                color="text.disabled"
                sx={{ p: 0.5, '&:hover': { color: 'text.primary' } }}
            />
            <Tooltip title={reuseLabel}>
                <IconButton
                    size="small"
                    onClick={() => { onReuse(runActions); }}
                    aria-label={reuseLabel}
                    data-testid="imagegen-reuse-run"
                    sx={{ p: 0.5, color: 'text.disabled', '&:hover': { color: 'text.primary' } }}
                >
                    <RestartAlt sx={{ fontSize: 16 }} />
                </IconButton>
            </Tooltip>
            {onRemoveClick && (
                // A destructive action, so it gets its own hover colour
                // (error, not the neutral text.primary the other two use) —
                // that is also what makes it findable, not just clickable.
                <Tooltip title={removeRunLabel}>
                    <IconButton
                        size="small"
                        onClick={onRemoveClick}
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

    return (
        <Card
            data-testid="imagegen-generation-run"
            data-generation-status={run.status ?? 'completed'}
            variant="outlined"
            sx={{
                flex: layout.cols === 1
                    ? { xs: '0 0 min(82vw, 320px)', md: '0 0 clamp(280px, 46%, 360px)' }
                    // Never wider than the strip itself: a card you have to
                    // scroll inside is n images you cannot compare at once.
                    : { xs: `0 0 min(88vw, ${layout.cardWidth}px)`, md: `0 0 min(100%, ${layout.cardWidth}px)` },
                height: '100%',
                bgcolor: 'background.paper',
                borderStyle: run.status === 'pending' ? 'dashed' : 'solid',
                borderColor: run.status === 'failed' ? 'error.main' : undefined,
                scrollSnapAlign: 'start',
            }}
        >
            <CardContent sx={{ p: 1.5, height: '100%', '&:last-child': { pb: 1.5 } }}>
                {run.status === 'failed' ? (
                    <Stack spacing={1} sx={{ height: '100%', minWidth: 0 }}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                            <ErrorOutline color="error" fontSize="small" />
                            <Typography variant="body2" sx={{ fontWeight: 500, flex: 1, minWidth: 0 }}>
                                {t('playground.runFailed', { defaultValue: 'Generation failed' })}
                            </Typography>
                            <Box sx={{ mr: -0.5, mt: -0.5 }}>
                                {renderRunActions(run, () => onRemove(run.id))}
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
                            {runMeta}
                        </Typography>
                        {/* Retrying blind is not retrying: the images the failed
                            request was built from stay on the card, openable in
                            the same lightbox as any other image here. */}
                        <RunSourceStrip
                            sources={run.sourceImages ?? []}
                            onOpen={(index) => onOpenSource(run, index)}
                            onUseAsReference={onUseAsReference}
                        />
                        <Box sx={{ flex: 1 }} />
                        <Button
                            size="small"
                            variant="outlined"
                            startIcon={<Refresh fontSize="small" />}
                            onClick={() => onRetry(run)}
                            sx={{ alignSelf: 'flex-start' }}
                        >
                            {t('playground.retry', { defaultValue: 'Retry' })}
                        </Button>
                    </Stack>
                ) : (
                    // Pending and completed share one layout — the header, then
                    // the slot grid — so a run landing fills slots in place
                    // instead of swapping a spinner for a differently shaped card.
                    <Stack spacing={1.25} sx={{ height: '100%' }} aria-live="polite">
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
                                {/* A running request is still a request to copy or
                                    to fork into the next one — the actions do not
                                    wait for it to land. While it runs, Cancel is
                                    what removes it. */}
                                <Box sx={{ mr: -0.75, mt: -0.75 }}>
                                    {run.status === 'pending'
                                        ? renderRunActions(run)
                                        : renderRunActions(run, () => onRemove(run.id))}
                                </Box>
                            </Box>
                            <Typography
                                variant="caption"
                                sx={{
                                    display: 'block',
                                    color: 'text.secondary',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    whiteSpace: 'nowrap',
                                }}
                            >
                                {runMeta}
                            </Typography>
                            <RunSourceStrip
                                sources={run.sourceImages ?? []}
                                onOpen={(index) => onOpenSource(run, index)}
                                onUseAsReference={onUseAsReference}
                            />
                        </Box>
                        <Box
                            sx={{
                                display: 'grid',
                                gridTemplateColumns: `repeat(${layout.cols}, minmax(0, 1fr))`,
                                gridTemplateRows: `repeat(${layout.rows}, minmax(0, 1fr))`,
                                flex: 1,
                                minHeight: 0,
                                gap: 1,
                            }}
                        >
                            {Array.from({ length: slotCount }, (_, index) => {
                                const image = run.images[index];
                                const src = image ? resultSrc(image) : '';
                                if (src) {
                                    return (
                                        <Box
                                            key={`${run.id}-${index}`}
                                            sx={{ position: 'relative', width: '100%', height: '100%', minHeight: 0, borderRadius: 1, overflow: 'hidden', bgcolor: 'action.hover' }}
                                        >
                                            <ButtonBase
                                                onClick={() => onOpenOutput(runImage(run, 'output', index, src))}
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
                                                {/* The corner badge only where a tile has room for it; small
                                                    tiles open on click and show the hover scrim anyway. */}
                                                {layout.cols <= 2 && (
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
                                                )}
                                            </ButtonBase>
                                            <IconButton
                                                size="small"
                                                onClick={(event) => { event.stopPropagation(); onUseAsReference(src); }}
                                                aria-label={t('playground.useAsReference', { defaultValue: 'Use as reference' })}
                                                sx={{ position: 'absolute', bottom: 8, right: 8, ...overlayActionSx() }}
                                            >
                                                <Edit fontSize="small" />
                                            </IconButton>
                                        </Box>
                                    );
                                }
                                // An empty slot: still coming while the run is
                                // pending, missing once it has landed.
                                const pending = run.status === 'pending';
                                return (
                                    <Box
                                        key={`${run.id}-${index}`}
                                        data-testid={pending ? 'imagegen-slot-pending' : 'imagegen-slot-missing'}
                                        sx={{
                                            minHeight: 0,
                                            borderRadius: 1,
                                            border: 1,
                                            borderStyle: 'dashed',
                                            borderColor: 'divider',
                                            display: 'flex',
                                            flexDirection: 'column',
                                            alignItems: 'center',
                                            justifyContent: 'center',
                                            gap: 1,
                                            p: 1,
                                            textAlign: 'center',
                                        }}
                                    >
                                        {pending ? (
                                            <>
                                                <CircularProgress size={20} />
                                                {index === 0 && (
                                                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                                        {t('playground.generatingNew', { defaultValue: 'Generating new images…' })}
                                                    </Typography>
                                                )}
                                            </>
                                        ) : (
                                            <Typography variant="caption" sx={{ color: 'text.disabled' }}>
                                                {t('playground.emptyResult', { defaultValue: 'No image returned' })}
                                            </Typography>
                                        )}
                                    </Box>
                                );
                            })}
                        </Box>
                        {run.status === 'pending' && (
                            <Button
                                size="small"
                                variant="outlined"
                                color="inherit"
                                startIcon={<Close fontSize="small" />}
                                onClick={() => onCancel(run.id)}
                                data-testid="imagegen-cancel-run"
                                sx={{ alignSelf: 'center', flexShrink: 0 }}
                            >
                                {t('playground.cancelRun', { defaultValue: 'Cancel' })}
                            </Button>
                        )}
                    </Stack>
                )}
            </CardContent>
        </Card>
    );
};

export default GenerationRunCard;
