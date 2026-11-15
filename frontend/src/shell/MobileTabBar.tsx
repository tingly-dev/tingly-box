import { Badge, BottomNavigation, BottomNavigationAction } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router-dom';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { mobile, safeAreaBottom } from '@tingly/ui/theme/mobile';
import type { MobileTab, MobileTabKey } from './mobileTabs';

export function MobileTabBar({ tabs, active }: { tabs: MobileTab[]; active: MobileTabKey }) {
    const { t } = useTranslation();
    return (
        <BottomNavigation
            component="nav"
            aria-label={t('mobile.tabBar')}
            showLabels
            value={active}
            sx={{ flexShrink: 0, height: 'auto', pb: safeAreaBottom, borderTop: 1, borderColor: 'divider', bgcolor: 'background.paper' }}
        >
            {tabs.map((tab) => (
                <BottomNavigationAction
                    key={tab.key}
                    value={tab.key}
                    component={RouterLink}
                    to={tab.to}
                    label={tab.label}
                    icon={tab.attention ? <Badge variant="dot" color="warning">{tab.icon}</Badge> : tab.icon}
                    sx={{ minWidth: 0, minHeight: mobile.bottomNavHeight, '& .MuiBottomNavigationAction-label': { fontSize: fontSizes.micro, '&.Mui-selected': { fontSize: fontSizes.micro, fontWeight: 600 } } }}
                />
            ))}
        </BottomNavigation>
    );
}
