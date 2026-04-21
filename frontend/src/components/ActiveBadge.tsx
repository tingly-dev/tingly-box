import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import React from 'react';

/**
 * Small green badge with checkmark indicating active state.
 * Used on top of icon buttons to show which option is currently selected.
 */
export const ActiveBadge: React.FC = () => (
    <Box
        sx={{
            position: 'absolute',
            bottom: -1,
            right: -1,
            width: 12,
            height: 12,
            borderRadius: '50%',
            backgroundColor: 'success.main',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            border: '1.5px solid',
            borderColor: 'background.paper',
        }}
    >
        <Typography
            sx={{
                fontSize: '9px',
                lineHeight: 1,
                color: 'background.paper',
                fontWeight: 'bold',
            }}
        >
            {'\u2713'}
        </Typography>
    </Box>
);

export default ActiveBadge;
