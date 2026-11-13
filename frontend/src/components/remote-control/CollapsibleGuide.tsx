import { KeyboardArrowDown, KeyboardArrowUp } from '@/components/icons';
import { Box, Button, Card, Collapse, Divider, Stack, Typography } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { radius } from '@tingly/ui/theme/radius';

interface CollapsibleGuideProps {
    platformName: string;
    platformGuide?: React.ReactNode;
    /** Default to expanded when the platform has no bots yet - first-time
        setup is exactly when the guide is needed most. */
    defaultExpanded?: boolean;
}

const CollapsibleGuide: React.FC<CollapsibleGuideProps> = ({ platformName, platformGuide, defaultExpanded = false }) => {
    const { t } = useTranslation();
    const [expanded, setExpanded] = useState(defaultExpanded);

    const handleToggle = () => {
        setExpanded(!expanded);
    };

    return (
        <Card variant="outlined" sx={{mb: 2, borderRadius: radius.card, boxShadow: 'none'}}>
            <Box sx={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 2, px: 2, py: 1.25}}>
                <Box sx={{minWidth: 0}}>
                    <Typography variant="body2" sx={{fontWeight: 600}}>
                        {t('remoteControl.guide.title', { defaultValue: '{{platform}} Setup Guide', platform: platformName })}
                    </Typography>
                    {!expanded && (
                        <Typography variant="caption" color="text.secondary">
                            {t('remoteControl.guide.collapsedHint', {defaultValue: 'Connection steps, credentials, and examples'})}
                        </Typography>
                    )}
                </Box>
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
                    {expanded
                        ? t('remoteControl.guide.showLess', { defaultValue: 'Show Less' })
                        : t('remoteControl.guide.showMore', { defaultValue: 'Show More' })}
                </Button>
            </Box>
            <Collapse in={expanded} unmountOnExit>
                <Divider/>
                <Box sx={{p: 2}}>
                    <Stack spacing={2}>{platformGuide}</Stack>
                </Box>
            </Collapse>
        </Card>
    );
};

export default CollapsibleGuide;
