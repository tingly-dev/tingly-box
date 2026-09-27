import { Autocomplete, Chip, TextField, ToggleButton, ToggleButtonGroup, type SxProps, type Theme } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { normalizeTags, type PieceKind } from './model';

// The words for each kind, in one place: the toggle, the list, the menu and
// the filter all say the same thing.
export const usePieceKindLabel = () => {
    const { t } = useTranslation();
    return (kind: PieceKind | 'all'): string => ({
        all: t('imageLibrary.kind.all', { defaultValue: 'All' }),
        prompt: t('imageLibrary.kind.prompt', { defaultValue: 'Prompt' }),
        term: t('imageLibrary.kind.term', { defaultValue: 'Term' }),
        phrase: t('imageLibrary.kind.phrase', { defaultValue: 'Phrase' }),
    })[kind];
};

interface KindToggleProps<K extends string> {
    kinds: K[];
    value: K;
    onChange: (kind: K) => void;
    sx?: SxProps<Theme>;
}

// Picks one kind. Used as a field (editor, split rows) and as a filter (the
// list), which is why the choices are passed in.
export const KindToggle = <K extends PieceKind | 'all'>({ kinds, value, onChange, sx }: KindToggleProps<K>) => {
    const label = usePieceKindLabel();
    return (
        <ToggleButtonGroup
            size="small"
            exclusive
            value={value}
            onChange={(_, next: K | null) => { if (next) onChange(next); }}
            sx={sx}
        >
            {kinds.map((kind) => <ToggleButton key={kind} value={kind}>{label(kind)}</ToggleButton>)}
        </ToggleButtonGroup>
    );
};

interface TagInputProps {
    value: string[];
    onChange: (tags: string[]) => void;
    // Tags already in use, offered first so the vocabulary stays small.
    options: string[];
    label: string;
}

export const TagInput: React.FC<TagInputProps> = ({ value, onChange, options, label }) => {
    const { t } = useTranslation();
    return (
        <Autocomplete
            multiple
            freeSolo
            size="small"
            options={options}
            value={value}
            onChange={(_, next) => onChange(normalizeTags(next))}
            renderValue={(tags, getItemProps) => tags.map((tag, index) => {
                const { key, ...itemProps } = getItemProps({ index });
                return <Chip key={key} size="small" label={tag} {...itemProps} />;
            })}
            renderInput={(params) => (
                <TextField
                    {...params}
                    label={label}
                    placeholder={value.length === 0
                        ? t('imageLibrary.tagsPlaceholder', { defaultValue: 'style, lighting, subject… (Enter to add)' })
                        : undefined}
                />
            )}
        />
    );
};
