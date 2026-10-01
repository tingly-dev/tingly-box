import { useState } from 'react';
import { Box, Button, Link, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { ExpandLess, ExpandMore, WarningAmber, tablerMui } from '@/components/icons';
import { IconAt } from '@tabler/icons-react';
import { fontMono } from '@/theme/fonts';
import type { Composition } from './composeEntities';
import type { ImageEntity } from './entityTypes';
import { EntityChip } from './entityUi';

const At = tablerMui(IconAt);

interface Props {
    composition: Composition;
    manualRefs: number;
    limit: number;
    onBeginMention: () => void;
    onOpenEntity: (entity: ImageEntity) => void;
}

// What this run will take from the referenced entities — shown before the
// run, in concrete values: which images, how many, and the prompt as the
// model will read it. Nothing about an `@name` is left to be guessed.
const EntityCompositionPanel: React.FC<Props> = ({ composition, manualRefs, limit, onBeginMention, onOpenEntity }) => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const [showPrompt, setShowPrompt] = useState(false);

    // Empty: one line that teaches the gesture, not a panel.
    if (composition.entities.length === 0) {
        return (
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center', minHeight: 32 }}>
                <Button size="small" variant="outlined" startIcon={<At sx={{ fontSize: 16 }} />} onClick={onBeginMention} sx={{ flexShrink: 0 }}>
                    {t('imageEntity.compose.reference', { defaultValue: 'Entity' })}
                </Button>
                <Typography sx={{ fontSize: 12.5, color: 'text.secondary' }}>
                    {t('imageEntity.compose.hint', { defaultValue: 'Type @ to bring in a saved character or style.' })}{' '}
                    <Link component="button" type="button" onClick={() => navigate('/image/entities')} sx={{ fontSize: 12.5, verticalAlign: 'baseline' }}>
                        {t('imageEntity.compose.openLibrary', { defaultValue: 'Entity library' })}
                    </Link>
                </Typography>
            </Stack>
        );
    }

    const truncated = composition.allocations.filter((item) => item.taken < item.available);

    return (
        <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1.5, p: 1.25 }}>
            <Stack direction="row" sx={{ alignItems: 'center', mb: 1 }}>
                <Typography sx={{ fontSize: 12.5, fontWeight: 600, flex: 1 }}>
                    {t('imageEntity.compose.title', { defaultValue: 'Referenced in this run' })}
                </Typography>
                <Typography sx={{ fontSize: 12, color: 'text.secondary', fontVariantNumeric: 'tabular-nums' }}>
                    {manualRefs > 0
                        ? t('imageEntity.compose.budgetWithManual', {
                            defaultValue: 'Reference images {{used}} + {{manual}} attached / {{limit}}',
                            used: composition.used,
                            manual: manualRefs,
                            limit,
                        })
                        : t('imageEntity.compose.budget', {
                            defaultValue: 'Reference images {{used}} / {{limit}}',
                            used: composition.used,
                            limit,
                        })}
                </Typography>
            </Stack>
            <Stack direction="row" sx={{ flexWrap: 'wrap', gap: 0.75 }}>
                {composition.allocations.map(({ entity, taken, available }) => (
                    <EntityChip
                        key={entity.id}
                        entity={entity}
                        detail={available > 0 ? `${taken}/${available}` : undefined}
                        onClick={() => onOpenEntity(entity)}
                    />
                ))}
                <Button size="small" onClick={onBeginMention} startIcon={<At sx={{ fontSize: 15 }} />} sx={{ minWidth: 0, height: 28, px: 1 }}>
                    {t('imageEntity.compose.add', { defaultValue: 'Add' })}
                </Button>
            </Stack>

            {/* The budget split is a default nobody chose, so it is said out
                loud — with the way to change it right there. */}
            {truncated.length > 0 && (
                <Stack direction="row" spacing={0.75} sx={{ mt: 1, alignItems: 'flex-start', color: 'warning.dark' }}>
                    <WarningAmber sx={{ fontSize: 16, mt: '2px' }} />
                    <Typography sx={{ fontSize: 12.5 }}>
                        {t('imageEntity.compose.truncated', {
                            defaultValue: 'Only {{limit}} reference images fit in one run. Taking the first {{taken}} of {{name}}\'s {{available}} — reorder them in the entity to choose which.',
                            limit,
                            name: truncated[0].entity.name,
                            taken: truncated[0].taken,
                            available: truncated[0].available,
                        })}
                        {truncated.length > 1 && ` ${t('imageEntity.compose.truncatedMore', { defaultValue: '(+{{count}} more)', count: truncated.length - 1 })}`}
                        {' '}
                        <Link component="button" type="button" onClick={() => onOpenEntity(truncated[0].entity)} sx={{ fontSize: 12.5, verticalAlign: 'baseline' }}>
                            {t('imageEntity.compose.reorder', { defaultValue: 'Reorder' })}
                        </Link>
                    </Typography>
                </Stack>
            )}

            <Button
                size="small"
                onClick={() => setShowPrompt((open) => !open)}
                endIcon={showPrompt ? <ExpandLess /> : <ExpandMore />}
                sx={{ mt: 0.5, ml: -0.75, color: 'text.secondary', fontSize: 12.5 }}
            >
                {t('imageEntity.compose.showPrompt', { defaultValue: 'Prompt the model receives' })}
            </Button>
            {showPrompt && (
                <Box
                    sx={{
                        mt: 0.5,
                        p: 1,
                        borderRadius: 1,
                        bgcolor: 'action.hover',
                        fontFamily: fontMono,
                        fontSize: 12,
                        whiteSpace: 'pre-wrap',
                        maxHeight: 140,
                        overflowY: 'auto',
                    }}
                >
                    {composition.expandedPrompt}
                    {composition.negative && `\n\n${t('imageEntity.compose.negative', { defaultValue: 'Avoid' })}：${composition.negative}`}
                </Box>
            )}
            <Typography sx={{ mt: 0.75, fontSize: 11, color: 'text.disabled' }}>
                {t('imageEntity.prototypeNote', { defaultValue: 'Prototype — runs do not send entity references yet.' })}
            </Typography>
        </Box>
    );
};

export default EntityCompositionPanel;
