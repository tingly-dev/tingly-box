import PlatformBotPage from './PlatformBotPage';
import { getPlatformGuide } from '@/constants/platformGuides';

const SlackPage = () => {
    const config = getPlatformGuide('slack');

    return (
        <PlatformBotPage
            platformId="slack"
            platformName={config?.name || 'Slack'}
            platformGuide={config?.guide}
        />
    );
};

export default SlackPage;
