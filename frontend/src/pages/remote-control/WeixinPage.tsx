import PlatformBotPage from './PlatformBotPage';
import { getPlatformGuide } from '@/constants/platformGuides';

const WeixinPage = () => {
    const config = getPlatformGuide('weixin');

    return (
        <PlatformBotPage
            platformId="weixin"
            platformName={config?.name || 'Weixin (微信)'}
            platformGuide={config?.guide}
        />
    );
};

export default WeixinPage;
