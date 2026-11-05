import { useEffect, useState } from 'react';
import { Box, FormControl, FormHelperText, InputLabel, MenuItem, Select, TextField } from '@mui/material';
import { useTranslation } from 'react-i18next';
import {
    FRAME_COUNT_RANGE,
    FRAME_DURATION_RANGE,
    OUTPUT_FORMATS,
    clampInt,
    type OutputFormat,
    type OutputOptions,
} from './imageOutput';

interface ImageOutputFormatFieldsProps {
    value: OutputOptions;
    onChange: (next: OutputOptions) => void;
    // images/edits does not take the format yet: with references attached the
    // choice is kept but says it will not apply.
    hasReferences: boolean;
}

interface RangeFieldProps {
    label: string;
    value: number;
    range: { min: number; max: number; default: number };
    onCommit: (next: number) => void;
}

// A number that can be typed over: the draft is free text while the field has
// focus, a value inside the range is applied as it is typed, and anything else
// snaps to the nearest valid one on blur (clamping per keystroke would make
// "12" impossible to type past the minimum of "1").
const RangeField: React.FC<RangeFieldProps> = ({ label, value, range, onCommit }) => {
    const [draft, setDraft] = useState(String(value));
    useEffect(() => { setDraft(String(value)); }, [value]);
    return (
        <TextField
            size="small"
            type="number"
            label={label}
            value={draft}
            onChange={(event) => {
                setDraft(event.target.value);
                const typed = Number(event.target.value);
                if (event.target.value !== '' && Number.isInteger(typed) && typed >= range.min && typed <= range.max) {
                    onCommit(typed);
                }
            }}
            onBlur={() => {
                const next = clampInt(draft, range.min, range.max, range.default);
                setDraft(String(next));
                onCommit(next);
            }}
            slotProps={{ htmlInput: { min: range.min, max: range.max } }}
        />
    );
};

const FORMAT_LABELS: Record<OutputFormat, string> = { png: 'png', jpeg: 'jpeg', webp: 'webp', gif: 'gif' };

// The output-format choice of the playground's options. Format is a plain
// select like Size and Quality, defaulting to png; the frame settings exist
// only while gif is chosen, so the default panel is as small as it was.
const ImageOutputFormatFields: React.FC<ImageOutputFormatFieldsProps> = ({ value, onChange, hasReferences }) => {
    const { t } = useTranslation();
    const isGif = value.format === 'gif';
    const hint = hasReferences && value.format !== 'png'
        ? t('playground.outputFormat.editsHint', {
            defaultValue: 'References use images/edits, which does not take an output format yet — this run will return the default.',
        })
        : isGif
            ? t('playground.outputFormat.gifHint', {
                defaultValue: 'Experimental, Codex only — animated GIF support upstream is unverified.',
            })
            : '';

    return (
        <Box>
            <Box
                sx={{
                    display: 'grid',
                    gridTemplateColumns: {
                        xs: 'repeat(2, minmax(0, 1fr))',
                        sm: 'repeat(3, minmax(0, 1fr))',
                    },
                    gap: 1.5,
                }}
            >
                <FormControl size="small" sx={{ gridColumn: { xs: isGif ? '1 / -1' : 'auto', sm: 'auto' } }}>
                    <InputLabel id="image-output-format-label">
                        {t('playground.outputFormat.label', { defaultValue: 'Format' })}
                    </InputLabel>
                    <Select
                        labelId="image-output-format-label"
                        label={t('playground.outputFormat.label', { defaultValue: 'Format' })}
                        value={value.format}
                        onChange={(event) => onChange({ ...value, format: event.target.value as OutputFormat })}
                    >
                        {OUTPUT_FORMATS.map((format) => (
                            <MenuItem key={format} value={format}>{FORMAT_LABELS[format]}</MenuItem>
                        ))}
                    </Select>
                </FormControl>
                {isGif && (
                    <>
                        <RangeField
                            label={t('playground.outputFormat.frames', { defaultValue: 'Frames' })}
                            value={value.frameCount}
                            range={FRAME_COUNT_RANGE}
                            onCommit={(frameCount) => onChange({ ...value, frameCount })}
                        />
                        <RangeField
                            label={t('playground.outputFormat.duration', { defaultValue: 'Frame duration (ms)' })}
                            value={value.frameDurationMs}
                            range={FRAME_DURATION_RANGE}
                            onCommit={(frameDurationMs) => onChange({ ...value, frameDurationMs })}
                        />
                    </>
                )}
            </Box>
            {hint && <FormHelperText sx={{ mx: 0 }}>{hint}</FormHelperText>}
        </Box>
    );
};

export default ImageOutputFormatFields;
