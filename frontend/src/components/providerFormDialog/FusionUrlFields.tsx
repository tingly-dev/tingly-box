import {Link, Stack, TextField} from '@mui/material';
import {useTranslation} from 'react-i18next';

interface FusionUrlFieldsProps {
    openAIUrl: string;
    anthropicUrl: string;
    /** Called with the new value on each keystroke (parent owns the state). */
    onOpenAIChange: (value: string) => void;
    onAnthropicChange: (value: string) => void;
    /** Commit the mirrored value to the parent form on blur. */
    onOpenAIBlur: () => void;
    onAnthropicBlur: () => void;
    /** When true, empty fields render in their error state. */
    baseUrlError: boolean;
    mode: 'add' | 'edit';
    /** Edit-mode downgrade link: convert the fusion provider to a single endpoint. */
    onConvertToSingle?: () => void;
}

/**
 * The two-URL body of the Fusion endpoint form: an OpenAI base URL and an
 * Anthropic base URL that share one API key (rendered separately by the parent).
 * Both sides are required; the parent's submit/verify logic gates on them.
 */
const FusionUrlFields = ({
                             openAIUrl,
                             anthropicUrl,
                             onOpenAIChange,
                             onAnthropicChange,
                             onOpenAIBlur,
                             onAnthropicBlur,
                             baseUrlError,
                             mode,
                             onConvertToSingle,
                         }: FusionUrlFieldsProps) => {
    const {t} = useTranslation();

    const fields = [
        {
            key: 'openai',
            label: t('providerDialog.customFusion.openAILabel'),
            placeholder: t('providerDialog.provider.customPlaceholder', {defaultValue: 'https://api.example.com/v1'}),
            value: openAIUrl,
            onChange: onOpenAIChange,
            onBlur: onOpenAIBlur,
            helperText: undefined as string | undefined,
        },
        {
            key: 'anthropic',
            label: t('providerDialog.customFusion.anthropicLabel'),
            placeholder: t('providerDialog.fusionForm.anthropicPlaceholder', {defaultValue: 'https://api.example.com/anthropic'}),
            value: anthropicUrl,
            onChange: onAnthropicChange,
            onBlur: onAnthropicBlur,
            helperText: t('providerDialog.fusionForm.help', {defaultValue: 'Both protocols share the API key below. Inbound requests are routed to the matching endpoint.'}),
        },
    ];

    return (
        <Stack spacing={2}>
            {fields.map((f) => (
                <TextField
                    key={f.key}
                    size="small"
                    fullWidth
                    required
                    label={f.label}
                    placeholder={f.placeholder}
                    value={f.value}
                    onChange={(e) => f.onChange(e.target.value)}
                    onBlur={f.onBlur}
                    error={baseUrlError && !f.value.trim()}
                    helperText={f.helperText}
                />
            ))}
            {mode === 'edit' && onConvertToSingle && (
                <Link
                    component="button"
                    type="button"
                    variant="caption"
                    underline="hover"
                    sx={{alignSelf: 'flex-start'}}
                    onClick={onConvertToSingle}
                >
                    {t('providerDialog.fusionForm.convertToSingle', {defaultValue: 'Convert to a single endpoint'})}
                </Link>
            )}
        </Stack>
    );
};

export default FusionUrlFields;
