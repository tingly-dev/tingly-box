import { Alert, Stack, Typography } from '@mui/material';
import UnifiedCard from '@/components/UnifiedCard';

interface PreviewNoticeProps {
    platformName: string;
}

const PreviewNotice: React.FC<PreviewNoticeProps> = ({ platformName }) => {
    return (
        <UnifiedCard
            title="Preview Version"
            size="full"
            sx={{ mb: 2 }}
        >
            <Alert severity="info" sx={{ mb: 2 }}>
                <Typography variant="body2">
                    This feature is currently in <strong>preview</strong>. It is designed to work with{' '}
                    <strong>Claude Code</strong> installed on your local machine with your config.
                </Typography>
            </Alert>
            <Typography variant="body2" color="text.secondary">
                The <strong>Remote Control</strong> Bot enables you to interact with <strong>Claude
                Code</strong> through {platformName}.
            </Typography>
            <Typography variant="body2" color="text.secondary">
                Make sure you have <strong>Claude Code CLI</strong> installed and configured before using this
                feature.
            </Typography>
            <Typography variant="body2" color="text.secondary">
                <strong>Once you enable a bot, the remote control is started with {platformName}, and vice
                    versa.</strong>
            </Typography>
        </UnifiedCard>
    );
};

export default PreviewNotice;
