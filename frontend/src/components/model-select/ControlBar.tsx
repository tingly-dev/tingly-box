import { Box } from '@mui/material';
import React from 'react';

interface ControlBarProps {
    children: React.ReactNode;
}

// ControlBar: the shared hover-reveal action strip anchored to a model card's
// bottom-right corner (Edit/Delete/Test). Stops its own clicks from bubbling
// to the card's onClick, which selects the model — every action inside it is
// a distinct gesture from selecting the card.
export function ControlBar({ children }: ControlBarProps) {
    return (
        <Box
            className="control-bar"
            sx={{
                position: 'absolute',
                bottom: 0,
                right: 0,
                height: 20,
                backgroundColor: 'grey.50',
                borderTop: 1,
                borderTopLeftRadius: 4,
                borderColor: 'grey.200',
                display: 'flex',
                alignItems: 'center',
                px: 0.5,
                opacity: 0,
                transition: 'opacity 0.2s',
                zIndex: 10,
            }}
            onClick={(e) => {
                e.stopPropagation();
                e.preventDefault();
            }}
            onMouseDown={(e) => {
                e.stopPropagation();
                e.preventDefault();
            }}
        >
            {children}
        </Box>
    );
}

export default ControlBar;
