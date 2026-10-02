import { LiveDataProvider } from '@/contexts/LiveDataContext';
import Footer from '../components/Footer';
import NavBar from '../components/NavBar';
import { Outlet, useLocation } from 'react-router-dom';
import { usePublicInfo } from '@/contexts/PublicInfoContext';
import { useIsMobile } from '@/hooks/use-mobile';
import { useTranslation } from 'react-i18next';
import { ChevronDown } from 'lucide-react';
import '../styles/liquid.css';

function PublicSurface() {
  const { publicInfo } = usePublicInfo();
  const isMobile = useIsMobile();
  const { pathname } = useLocation();
  const home = pathname === '/';
  const { t } = useTranslation();
  const settings = publicInfo?.theme_settings;
  const bgUrl = isMobile ? settings?.backgroundImageUrlMobile || settings?.backgroundImageUrlDesktop : settings?.backgroundImageUrlDesktop;
  const configuredWidth = Number(settings?.mainContentWidth ?? 100);
  const width = Number.isFinite(configuredWidth) ? Math.min(100, Math.max(30, configuredWidth)) : 100;
  const wallpaper = bgUrl ? { backgroundImage: `url(${JSON.stringify(bgUrl)})` } : undefined;
  return <div className="km-layout layout flex flex-col w-full min-h-screen" data-home={home} style={!home ? wallpaper : undefined}>
    {home && <header className="liquid-hero" style={wallpaper}>
      <div className="liquid-hero-nav"><NavBar /></div>
      <div className="liquid-hero-copy">
        <span className="liquid-hero-eyebrow">KOMARI MONITOR</span>
        <h1>{publicInfo?.sitename || 'Komari'}</h1>
        <p>{publicInfo?.description || t('liquid.welcome')}</p>
      </div>
      <a className="liquid-hero-scroll" href="#nodes"><span>{t('liquid.explore')}</span><ChevronDown size={22} /></a>
      <div className="liquid-hero-wave" aria-hidden="true">
        {[0, 1, 2].map(layer => <svg key={layer} className={`hero-wave-layer hero-wave-layer-${layer}`} viewBox="0 0 2880 100" preserveAspectRatio="none">
          <path d="M0 50 Q360 0 720 50 T1440 50 Q1800 0 2160 50 T2880 50 V100 H0Z" />
        </svg>)}
      </div>
    </header>}
    <main id="nodes" className="km-main main-content h-full" style={{ width: `${width}vw`, marginInline: 'auto' }}>
      {!home && <NavBar />}
      <Outlet />
    </main>
    <Footer />
  </div>;
}
export default function IndexLayout() {
  return <LiveDataProvider><PublicSurface /></LiveDataProvider>;
}
