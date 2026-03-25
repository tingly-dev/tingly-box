import { KeyboardArrowDown, KeyboardArrowUp } from '@mui/icons-material';
import { Alert, Box, Button, Stack, Typography } from '@mui/material';
import UnifiedCard from '@/components/UnifiedCard';
import { useState } from 'react';

interface CollapsibleGuideProps {
    platformName: string;
    platformGuide?: React.ReactNode;
}

const CollapsibleGuide: React.FC<CollapsibleGuideProps> = ({ platformName, platformGuide }) => {
    const [expanded, setExpanded] = useState(false);

    const handleToggle = () => {
        setExpanded(!expanded);
    };

    return (
        <UnifiedCard
            title={`${platformName} Setup Guide`}
            size="full"
            sx={{ mb: 2 }}
        >
            {/* Guide content with preview mode */}
            <Box
                sx={{
                    maxHeight: expanded ? 'none' : '120px',
                    overflow: 'hidden',
                    position: 'relative',
                    transition: 'max-height 0.3s ease-in-out',
                }}
            >
                <Stack spacing={2}>
                    {platformGuide}
                </Stack>

                {/* Fade overlay when collapsed */}
                {!expanded && (
                    <Box
                        sx={{
                            position: 'absolute',
                            bottom: 0,
                            left: 0,
                            right: 0,
                            height: '30px',
                            background: 'linear-gradient(to bottom, transparent, var(--mui-palette-background-paper))',
                            pointerEvents: 'none',
                        }}
                    />
                )}
            </Box>

            {/* Expand/Collapse Button */}
            <Box
                sx={{
                    display: 'flex',
                    justifyContent: 'center',
                    mt: 2,
                }}
            >
                <Button
                    onClick={handleToggle}
                    size="small"
                    endIcon={expanded ? <KeyboardArrowUp /> : <KeyboardArrowDown />}
                    sx={{
                        color: 'text.secondary',
                        '&:hover': {
                            backgroundColor: 'action.hover',
                        },
                    }}
                >
                    {expanded ? 'Show Less' : 'Show More'}
                </Button>
            </Box>
        </UnifiedCard>
    );
};

export default CollapsibleGuide;
