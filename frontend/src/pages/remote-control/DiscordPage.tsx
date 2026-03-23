import PlatformBotPage from './PlatformBotPage';
import { getPlatformGuide } from '@/constants/platformGuides';

const DiscordPage = () => {
    const config = getPlatformGuide('discord');

    return (
        <PlatformBotPage
            platformId="discord"
            platformName={config?.name || 'Discord'}
            platformGuide={config?.guide}
        />
    );
};

export default DiscordPage;
