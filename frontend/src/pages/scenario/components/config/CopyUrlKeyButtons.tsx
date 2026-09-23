import { Button, Stack } from '@mui/material';
import React from 'react';

interface CopyUrlKeyButtonsProps {
    /** Full endpoint URL copied by the "Copy URL" button. */
    url: string;
    /** Full token copied by the "Copy API Key" button. */
    token: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
}

// The Copy URL / Copy API Key button pair shared by the Xcode / Cursor /
// ClaudeDesktop info modals. The masked token *display* stays per-modal (the
// three modals embed it in different step layouts).
export const CopyUrlKeyButtons: React.FC<CopyUrlKeyButtonsProps> = ({
    url,
    token,
    copyToClipboard,
}) => (
    <Stack direction="row" spacing={1}>
        <Button
            variant="outlined"
            size="small"
            onClick={() => copyToClipboard(url, 'URL')}
            sx={{ flex: 1 }}
        >
            Copy URL
        </Button>
        <Button
            variant="outlined"
            size="small"
            onClick={() => copyToClipboard(token, 'API Key')}
            sx={{ flex: 1 }}
        >
            Copy API Key
        </Button>
    </Stack>
);
