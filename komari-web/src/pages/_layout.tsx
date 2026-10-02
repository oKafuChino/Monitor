import { LiveDataProvider } from '@/contexts/LiveDataContext';
import Footer from '../components/Footer';
import NavBar from '../components/NavBar';
import { Outlet, useLocation } from 'react-router-dom';
import { usePublicInfo } from '@/contexts/PublicInfoContext';
import { useIsMobile } from '@/hooks/use-mobile';
import { useLocalStorage } from '@/hooks/useLocalStorage';
import { useTranslation } from 'react-i18next';
import { ChevronDown } from 'lucide-react';
import '../styles/liquid.css';
import { useLiquidMotion } from '@/hooks/useLiquidMotion';

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
  const [reading, setReading] = useLocalStorage('komari-liquid-reading', false);
  const [reduced, setReduced] = useLocalStorage('komari-liquid-reduced-motion', false);
  const surfaceRef = useLiquidMotion(reduced === true);
  const wallpaper = bgUrl ? { backgroundImage: `url(${JSON.stringify(bgUrl)})` } : undefined;
  return <div ref={surfaceRef} className="km-layout layout flex flex-col w-full min-h-screen" data-home={home} data-reading={reading === true} data-motion={reduced === true ? 'reduced' : 'normal'} style={!home ? wallpaper : undefined}>
    {home && <header className="liquid-hero" style={wallpaper}>
      <div className="liquid-hero-nav"><NavBar /></div>
      <div className="liquid-hero-copy">
        <span className="liquid-hero-eyebrow">KOMARI MONITOR</span>
        <h1>{publicInfo?.sitename || 'Komari'}</h1>
        <p>{publicInfo?.description || t('liquid.welcome')}</p>
      </div>
      <a className="liquid-hero-scroll" href="#nodes"><span>{t('liquid.explore')}</span><ChevronDown size={22} /></a>
      <svg className="liquid-hero-wave" viewBox="0 0 1440 100" preserveAspectRatio="none" aria-hidden="true"><path opacity=".35" d="M0 35Q360 100 720 35T1440 35V100H0Z"/><path opacity=".55" d="M0 65Q360 0 720 60T1440 50V100H0Z"/><path d="M0 80Q360 35 720 75T1440 65V100H0Z"/></svg>
    </header>}
    <main id="nodes" className="km-main main-content h-full" style={{ width: `${width}vw`, marginInline: 'auto' }}>
      {!home && <NavBar />}
      <div className="liquid-preferences">
        <label><input type="checkbox" checked={reading === true} onChange={e => setReading(e.target.checked)} />{t('liquid.reading')}</label>
        <label><input type="checkbox" checked={reduced === true} onChange={e => setReduced(e.target.checked)} />{t('liquid.reducedMotion')}</label>
      </div>
      <Outlet />
    </main>
    <Footer />
  </div>;
}
export default function IndexLayout() {
  return <LiveDataProvider><PublicSurface /></LiveDataProvider>;
}
