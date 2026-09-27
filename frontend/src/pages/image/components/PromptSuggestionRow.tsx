import { Chip, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Add } from '@/components/icons';
import type { PromptSuggestion } from './promptSuggestions';

interface PromptSuggestionRowProps {
    suggestions: PromptSuggestion[];
    prompt: string;
    onInsert: (suggestion: PromptSuggestion) => void;
}

// The suggestion slot under the prompt. Nothing here is ever inserted without
// a click, and a suggestion already in the prompt is not offered again — the
// row empties as it is used, so it never nags.
const PromptSuggestionRow: React.FC<PromptSuggestionRowProps> = ({ suggestions, prompt, onInsert }) => {
    const { t } = useTranslation();
    const open = suggestions.filter((suggestion) => !prompt.includes(suggestion.text));
    if (open.length === 0) return null;
    return (
        <Stack direction="row" spacing={0.75} useFlexGap sx={{ flexWrap: 'wrap', alignItems: 'center', mt: -0.5 }}>
            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                {t('playground.suggest.title')}
            </Typography>
            {open.map((suggestion) => (
                <Tooltip
                    key={suggestion.id}
                    title={<span style={{ whiteSpace: 'pre-line' }}>{suggestion.text}</span>}
                    placement="top"
                >
                    <Chip
                        size="small"
                        variant="outlined"
                        icon={<Add sx={{ fontSize: 14 }} />}
                        label={suggestion.label}
                        onClick={() => onInsert(suggestion)}
                        sx={{ height: 22, fontSize: 12 }}
                    />
                </Tooltip>
            ))}
        </Stack>
    );
};

export default PromptSuggestionRow;
