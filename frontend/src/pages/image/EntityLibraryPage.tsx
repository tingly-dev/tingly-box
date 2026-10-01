import { useMemo, useState } from 'react';
import {
    Box,
    Button,
    ButtonBase,
    InputAdornment,
    Stack,
    TextField,
    ToggleButton,
    ToggleButtonGroup,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import UnifiedCard from '@/components/UnifiedCard';
import { Add, Search } from '@/components/icons';
import type { EntityKind, ImageEntity } from './entities/entityTypes';
import { ENTITY_KINDS } from './entities/entityTypes';
import { useEntities } from './entities/entityStore';
import { KIND_COLOR, KindIcon, useKindLabel } from './entities/entityUi';
import EntityEditorDialog, { type EntityDraft } from './entities/EntityEditorDialog';

type Filter = 'all' | EntityKind;

const EntityCard: React.FC<{ entity: ImageEntity; onOpen: () => void }> = ({ entity, onOpen }) => {
    const { t } = useTranslation();
    const [cover, ...rest] = entity.refs;
    return (
        <ButtonBase
            onClick={onOpen}
            sx={{
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'stretch',
                textAlign: 'left',
                borderRadius: 2,
                border: 1,
                borderColor: 'divider',
                overflow: 'hidden',
                bgcolor: 'background.paper',
                transition: 'border-color 120ms, box-shadow 120ms',
                '&:hover': { borderColor: KIND_COLOR[entity.kind].border, boxShadow: 2 },
            }}
        >
            <Box sx={{ position: 'relative', aspectRatio: entity.kind === 'character' ? '4 / 5' : '1 / 1', bgcolor: 'action.hover' }}>
                {cover && <Box component="img" src={cover} alt="" sx={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }} />}
                {rest.length > 0 && (
                    <Stack direction="row" spacing={0.5} sx={{ position: 'absolute', right: 8, bottom: 8 }}>
                        {rest.slice(0, 3).map((src, index) => (
                            <Box
                                key={index}
                                component="img"
                                src={src}
                                alt=""
                                sx={{ width: 30, height: 30, borderRadius: 1, objectFit: 'cover', border: '2px solid #fff', boxShadow: 1 }}
                            />
                        ))}
                    </Stack>
                )}
            </Box>
            <Box sx={{ p: 1.25 }}>
                <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                    <KindIcon kind={entity.kind} fontSize={15} />
                    <Typography sx={{ fontSize: 15, fontWeight: 600 }}>{entity.name}</Typography>
                </Stack>
                <Typography
                    sx={{
                        mt: 0.5,
                        fontSize: 12.5,
                        color: 'text.secondary',
                        display: '-webkit-box',
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: 'vertical',
                        overflow: 'hidden',
                        minHeight: 36,
                    }}
                >
                    {entity.prompt}
                </Typography>
                <Typography sx={{ mt: 0.75, fontSize: 12, color: 'text.secondary' }}>
                    {t('imageEntity.library.meta', {
                        defaultValue: '{{refs}} images · used {{uses}} times',
                        refs: entity.refs.length,
                        uses: entity.uses,
                    })}
                </Typography>
            </Box>
        </ButtonBase>
    );
};

// The library is where entities are looked at and maintained. Creating one
// mostly happens elsewhere — from a result worth keeping, or from @ in the
// prompt — so this page's own "new" button is the least common way in.
const EntityLibraryPage: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const kindLabel = useKindLabel();
    const entities = useEntities();
    const [filter, setFilter] = useState<Filter>('all');
    const [search, setSearch] = useState('');
    const [draft, setDraft] = useState<EntityDraft | null>(null);

    const visible = useMemo(() => {
        const query = search.trim().toLowerCase();
        return entities.filter((entity) => !query
            || entity.name.toLowerCase().includes(query)
            || entity.prompt.toLowerCase().includes(query));
    }, [entities, search]);

    const counts = useMemo(() => ({
        all: entities.length,
        character: entities.filter((entity) => entity.kind === 'character').length,
        style: entities.filter((entity) => entity.kind === 'style').length,
    }), [entities]);

    const sections = ENTITY_KINDS.filter((kind) => filter === 'all' || filter === kind);

    const sectionTitle: Record<EntityKind, string> = {
        character: t('imageEntity.library.characters', { defaultValue: 'Characters' }),
        style: t('imageEntity.library.styles', { defaultValue: 'Styles' }),
    };
    const sectionHelp: Record<EntityKind, string> = {
        character: t('imageEntity.library.charactersHelp', { defaultValue: 'Who or what is in the picture' }),
        style: t('imageEntity.library.stylesHelp', { defaultValue: 'How the picture is drawn' }),
    };

    return (
        <>
            <UnifiedCard
                size="full"
                titleHeadingLevel={1}
                title={t('imageEntity.library.title', { defaultValue: 'Entities' })}
                subtitle={t('imageEntity.library.subtitle', {
                    defaultValue: 'Characters and styles you reuse across generations. Bring one into a prompt with @name.',
                })}
                rightAction={(
                    <Button variant="contained" startIcon={<Add />} onClick={() => setDraft({ kind: filter === 'style' ? 'style' : 'character' })}>
                        {t('imageEntity.library.new', { defaultValue: 'New entity' })}
                    </Button>
                )}
            >
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} sx={{ mb: 3, alignItems: { sm: 'center' } }}>
                    <ToggleButtonGroup
                        size="small"
                        exclusive
                        value={filter}
                        onChange={(_, value: Filter | null) => { if (value) setFilter(value); }}
                    >
                        <ToggleButton value="all" sx={{ px: 1.5 }}>
                            {t('imageEntity.library.all', { defaultValue: 'All' })} · {counts.all}
                        </ToggleButton>
                        {ENTITY_KINDS.map((kind) => (
                            <ToggleButton key={kind} value={kind} sx={{ px: 1.5, gap: 0.5 }}>
                                <KindIcon kind={kind} fontSize={15} />
                                {kindLabel(kind)} · {counts[kind]}
                            </ToggleButton>
                        ))}
                    </ToggleButtonGroup>
                    <TextField
                        size="small"
                        value={search}
                        onChange={(event) => setSearch(event.target.value)}
                        placeholder={t('imageEntity.library.search', { defaultValue: 'Search by name or description' })}
                        sx={{ width: { sm: 280 } }}
                        slotProps={{
                            input: {
                                startAdornment: (
                                    <InputAdornment position="start">
                                        <Search sx={{ fontSize: 18 }} />
                                    </InputAdornment>
                                ),
                            },
                        }}
                    />
                </Stack>

                <Stack spacing={4}>
                    {sections.map((kind) => {
                        const items = visible.filter((entity) => entity.kind === kind);
                        return (
                            <Box key={kind} component="section">
                                <Stack direction="row" spacing={1} sx={{ alignItems: 'baseline', mb: 1.5 }}>
                                    <Typography component="h2" sx={{ fontSize: 16, fontWeight: 600 }}>{sectionTitle[kind]}</Typography>
                                    <Typography sx={{ fontSize: 13, color: 'text.secondary' }}>{sectionHelp[kind]}</Typography>
                                </Stack>
                                <Box
                                    sx={{
                                        display: 'grid',
                                        gridTemplateColumns: kind === 'character'
                                            ? 'repeat(auto-fill, minmax(180px, 1fr))'
                                            : 'repeat(auto-fill, minmax(200px, 1fr))',
                                        gap: 2,
                                    }}
                                >
                                    {items.map((entity) => (
                                        <EntityCard key={entity.id} entity={entity} onOpen={() => setDraft(entity)} />
                                    ))}
                                    {/* The add tile also says where entities usually
                                        come from, so the empty case teaches the
                                        common path instead of only this one. */}
                                    <ButtonBase
                                        onClick={() => setDraft({ kind })}
                                        sx={{
                                            borderRadius: 2,
                                            border: '1px dashed',
                                            borderColor: 'divider',
                                            minHeight: 200,
                                            flexDirection: 'column',
                                            gap: 1,
                                            p: 2,
                                            color: 'text.secondary',
                                            '&:hover': { borderColor: KIND_COLOR[kind].border, color: KIND_COLOR[kind].fg },
                                        }}
                                    >
                                        <Add />
                                        <Typography sx={{ fontSize: 14, fontWeight: 500 }}>
                                            {kind === 'character'
                                                ? t('imageEntity.library.newCharacter', { defaultValue: 'New character' })
                                                : t('imageEntity.library.newStyle', { defaultValue: 'New style' })}
                                        </Typography>
                                        <Typography sx={{ fontSize: 12, textAlign: 'center' }}>
                                            {t('imageEntity.library.fromResultTip', {
                                                defaultValue: 'Or open any result in the Playground and choose “Save as entity”.',
                                            })}
                                        </Typography>
                                    </ButtonBase>
                                </Box>
                            </Box>
                        );
                    })}
                </Stack>
            </UnifiedCard>
            <EntityEditorDialog
                draft={draft}
                onClose={() => setDraft(null)}
                onUse={(entity) => navigate(`/image/playground?use=${encodeURIComponent(entity.name)}`)}
            />
        </>
    );
};

export default EntityLibraryPage;
