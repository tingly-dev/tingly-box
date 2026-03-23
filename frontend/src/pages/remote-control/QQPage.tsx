import PlatformBotPage from './PlatformBotPage';
import { getPlatformGuide } from '@/constants/platformGuides';

const QQPage = () => {
    const config = getPlatformGuide('qq');

    return (
        <PlatformBotPage
            platformId="qq"
            platformName={config?.name || 'QQ'}
            platformGuide={config?.guide}
        />
    );
};

export default QQPage;
