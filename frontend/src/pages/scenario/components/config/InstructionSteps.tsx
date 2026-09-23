import { Box, Typography } from '@mui/material';
import React from 'react';

interface InstructionStepsProps {
    /** Numbered instruction lines shown before the value block. All but the
     * last get mb: 1.5; the last gets mb: 1 to hug the value block. */
    steps: React.ReactNode[];
    /** Indented monospace value block (URL / API key / description lines). */
    values?: React.ReactNode;
    /** Optional line rendered after the value block (e.g. "4. Click Verify"). */
    trailingStep?: React.ReactNode;
}

// Bordered step box used by the Xcode / Cursor info modals: numbered
// instruction lines plus the concrete connection values to enter.
export const InstructionSteps: React.FC<InstructionStepsProps> = ({
    steps,
    values,
    trailingStep,
}) => (
    <Box sx={{ bgcolor: 'background.paper', p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
        {steps.map((step, index) => (
            <Typography key={index} variant="subtitle2" sx={{ mb: index === steps.length - 1 ? 1 : 1.5 }}>
                {step}
            </Typography>
        ))}
        {values != null && (
            <Box sx={{ pl: 2, mb: 0.5 }}>{values}</Box>
        )}
        {trailingStep != null && (
            <Typography variant="subtitle2" sx={{ mt: 1.5 }}>{trailingStep}</Typography>
        )}
    </Box>
);
