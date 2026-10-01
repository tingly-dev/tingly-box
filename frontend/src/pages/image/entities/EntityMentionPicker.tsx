import { Box, ButtonBase, Paper, Popper, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Add } from '@/components/icons';
import type { ImageEntity } from './entityTypes';
import { EntityThumb, KIND_COLOR, KindIcon, useKindLabel } from './entityUi';

interface Props {
    anchorEl: HTMLElement | null;
    open: boolean;
    query: string;
    candidates: ImageEntity[];
    activeIndex: number;
    onHover: (index: number) => void;
    onSelect: (entity: ImageEntity) => void;
    onCreate: (name: string) => void;
}

// Anchored under the prompt field, as wide as it: the picker belongs to the
// field, not to a caret position the textarea cannot report.
const EntityMentionPicker: React.FC<Props> = ({
    anchorEl,
    open,
    query,
    candidates,
    activeIndex,
    onHover,
    onSelect,
    onCreate,
}) => {
    const { t } = useTranslation();
    const kindLabel = useKindLabel();
    const width = anchorEl?.offsetWidth ?? 360;

    return (
        <Popper
            open={open && Boolean(anchorEl)}
            anchorEl={anchorEl}
            placement="bottom-start"
            sx={{ zIndex: 1300, width }}
            modifiers={[{ name: 'offset', options: { offset: [0, 6] } }]}
        >
            {/* Clicks must not blur the textarea, or the caret the insert
                needs is gone before the click lands. */}
            <Paper elevation={8} onMouseDown={(event) => event.preventDefault()} sx={{ py: 0.75, maxHeight: 360, overflowY: 'auto' }}>
                <Typography sx={{ px: 1.5, pb: 0.5, fontSize: 12, color: 'text.secondary' }}>
                    {query
                        ? t('imageEntity.picker.matching', { defaultValue: 'Entities matching “{{query}}”', query })
                        : t('imageEntity.picker.title', { defaultValue: 'Reference an entity' })}
                </Typography>
                {candidates.map((entity, index) => {
                    const active = index === activeIndex;
                    return (
                        <ButtonBase
                            key={entity.id}
                            onClick={() => onSelect(entity)}
                            onMouseEnter={() => onHover(index)}
                            sx={{
                                width: '100%',
                                px: 1.5,
                                py: 0.75,
                                gap: 1.25,
                                justifyContent: 'flex-start',
                                textAlign: 'left',
                                bgcolor: active ? 'action.selected' : 'transparent',
                            }}
                        >
                            <EntityThumb src={entity.refs[0]} size={36} ratio={entity.kind === 'character' ? 4 / 3 : 1} />
                            <Box sx={{ minWidth: 0, flex: 1 }}>
                                <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                                    <Typography sx={{ fontSize: 14, fontWeight: 600 }}>{entity.name}</Typography>
                                    <Stack direction="row" spacing={0.25} sx={{ alignItems: 'center', color: KIND_COLOR[entity.kind].fg }}>
                                        <KindIcon kind={entity.kind} fontSize={13} />
                                        <Typography component="span" sx={{ fontSize: 12 }}>{kindLabel(entity.kind)}</Typography>
                                    </Stack>
                                </Stack>
                                <Typography noWrap sx={{ fontSize: 12, color: 'text.secondary' }}>{entity.prompt}</Typography>
                            </Box>
                            <Typography sx={{ fontSize: 12, color: 'text.secondary', flexShrink: 0 }}>
                                {t('imageEntity.refCount', { defaultValue: '{{count}} images', count: entity.refs.length })}
                            </Typography>
                        </ButtonBase>
                    );
                })}
                {candidates.length === 0 && (
                    <Typography sx={{ px: 1.5, py: 1, fontSize: 13, color: 'text.secondary' }}>
                        {t('imageEntity.picker.empty', { defaultValue: 'No entity by that name.' })}
                    </Typography>
                )}
                <ButtonBase
                    onClick={() => onCreate(query)}
                    sx={{ width: '100%', px: 1.5, py: 1, gap: 1, justifyContent: 'flex-start', borderTop: 1, borderColor: 'divider', mt: 0.5 }}
                >
                    <Add sx={{ fontSize: 18, color: 'primary.main' }} />
                    <Typography sx={{ fontSize: 13, color: 'primary.main', fontWeight: 500 }}>
                        {query
                            ? t('imageEntity.picker.createNamed', { defaultValue: 'New entity “{{query}}”…', query })
                            : t('imageEntity.picker.create', { defaultValue: 'New entity…' })}
                    </Typography>
                </ButtonBase>
                <Typography sx={{ px: 1.5, pt: 0.75, fontSize: 11, color: 'text.secondary' }}>
                    {t('imageEntity.picker.keys', { defaultValue: '↑↓ to move · Enter to insert · Esc to close' })}
                </Typography>
            </Paper>
        </Popper>
    );
};

export default EntityMentionPicker;
