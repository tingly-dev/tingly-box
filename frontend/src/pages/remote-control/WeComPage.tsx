import PlatformBotPage from './PlatformBotPage';
import { getPlatformGuide } from '@/constants/platformGuides';

const WeComPage = () => {
    const config = getPlatformGuide('wecom');

    return (
        <PlatformBotPage
            platformId="wecom"
            platformName={config?.name || 'WeCom (企业微信)'}
            platformGuide={config?.guide}
        />
    );
};

export default WeComPage;
