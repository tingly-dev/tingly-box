import PlatformBotPage from './PlatformBotPage';
import { getPlatformGuide } from '@/constants/platformGuides';

const TelegramPage = () => {
    const config = getPlatformGuide('telegram');

    return (
        <PlatformBotPage
            platformId="telegram"
            platformName={config?.name || 'Telegram'}
            platformGuide={config?.guide}
        />
    );
};

export default TelegramPage;
