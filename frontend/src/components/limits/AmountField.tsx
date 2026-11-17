import {Box, InputAdornment, TextField, Tooltip} from '@mui/material';
import {InfoOutlined as InfoOutlinedIcon} from '@/components/icons';
import {useEffect, useState} from 'react';
import {formatAmount, parseAmount} from './amount';

interface AmountFieldProps {
    label: string;
    value?: number;
    onChange: (next: number | undefined) => void;
    disabled?: boolean;
    /** What the number counts, spelled out on hover of an info icon inside the field. */
    info?: {label: string; text: string};
}

/**
 * A count that can be typed as "500K" or "2M". The text is the user's own
 * while they type; the number goes up as soon as it reads, and the field
 * settles to the canonical form (2M) on blur. Unreadable text flags the field
 * and leaves the last good value alone.
 */
export default function AmountField({label, value, onChange, disabled, info}: AmountFieldProps) {
    const [draft, setDraft] = useState(formatAmount(value));
    const parsed = parseAmount(draft);

    // Follow outside changes (dialog reset, another editor) but never fight typing:
    // only when what is shown no longer reads as the current value.
    useEffect(() => {
        const shown = parsed.kind === 'ok' ? parsed.value : parsed.kind === 'empty' ? undefined : value;
        if (shown !== value) setDraft(formatAmount(value));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [value]);

    return (
        <TextField
            size="small" disabled={disabled}
            label={label} value={draft}
            error={parsed.kind === 'invalid'}
            onChange={(e) => {
                setDraft(e.target.value);
                const p = parseAmount(e.target.value);
                if (p.kind === 'ok') onChange(p.value);
                else if (p.kind === 'empty') onChange(undefined);
            }}
            onBlur={() => setDraft(formatAmount(value))}
            slotProps={{
                htmlInput: {inputMode: 'decimal', 'aria-label': label},
                input: info ? {
                    endAdornment: (
                        <InputAdornment position="end">
                            {/* The app's field-info idiom (see the agent config panels): a quiet icon, tooltip on hover. */}
                            <Tooltip placement="top" arrow title={<Box sx={{whiteSpace: 'pre-line'}}>{info.text}</Box>}>
                                <InfoOutlinedIcon
                                    tabIndex={0} aria-label={info.label} data-testid="amount-info"
                                    sx={{fontSize: 14, color: 'text.disabled', cursor: 'help'}}
                                />
                            </Tooltip>
                        </InputAdornment>
                    ),
                } : undefined,
            }}
        />
    );
}
