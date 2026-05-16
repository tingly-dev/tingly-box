import {Box, Typography} from '@mui/material';
import React from 'react';

const ProtocolBaseUrlDisplay: React.FC<{ url: string }> = ({url}) => {
    if (!url) return null;
    return (
        <Box
            sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 0.5,
                mt: 0.75,
                px: 1,
                py: 0.5,
                bgcolor: 'background.default',
                borderRadius: 0.75,
            }}
        >
            <Typography
                variant="caption"
                sx={{
                    fontFamily: 'monospace',
                    color: 'primary.main',
                    fontSize: '0.7rem',
                    wordBreak: 'break-all',
                }}
            >
                {url}
            </Typography>
        </Box>
    );
};

export default ProtocolBaseUrlDisplay;
