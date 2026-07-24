import { Box, Tab, Tabs } from '@mui/material';
import { useNavigate, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { BOT_PLATFORM_IDS, platformDisplayName, usePlatformGuide } from '@/constants/platformGuides';
import PlatformRemoteAgentPage from './PlatformRemoteAgentPage';

// RemoteAgentPage is the nav-facing entry for the Remote purpose: ONE sidebar
// row (under the "Bots" rail icon, alongside Overview and Notify), with
// platform selection moved in-page as tabs instead of nine separate sidebar
// rows. The routes it tabs between (/remote-agent/:platform) are unchanged —
// deep links and the BotCard purpose chip still work exactly as before.
// PlatformRemoteAgentPage itself is untouched: same guide, add, and pairing
// behavior it already had.
const RemoteAgentPage = () => {
    const { platform = 'weixin' } = useParams<{ platform: string }>();
    const navigate = useNavigate();
    const { t } = useTranslation();
    const platformName = usePlatformGuide(platform)?.name || platform;

    return (
        <Box>
            <Tabs
                value={BOT_PLATFORM_IDS.includes(platform as typeof BOT_PLATFORM_IDS[number]) ? platform : false}
                onChange={(_, next) => navigate(`/remote-agent/${next}`)}
                variant="scrollable"
                scrollButtons="auto"
                sx={{ mb: 1, minHeight: 40, '& .MuiTab-root': { minHeight: 40, py: 0.75 } }}
            >
                {BOT_PLATFORM_IDS.map((id) => (
                    <Tab key={id} value={id} label={platformDisplayName(id, t)} />
                ))}
            </Tabs>
            <PlatformRemoteAgentPage platformId={platform} platformName={platformName} />
        </Box>
    );
};

export default RemoteAgentPage;
