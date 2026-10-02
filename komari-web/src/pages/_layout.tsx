import { LiveDataProvider } from '@/contexts/LiveDataContext';
import Footer from '../components/Footer';
import NavBar from '../components/NavBar';
import { Outlet } from 'react-router-dom';
import { usePublicInfo } from '@/contexts/PublicInfoContext';
import { useIsMobile } from '@/hooks/use-mobile';
import { useLocalStorage } from '@/hooks/useLocalStorage';
import { useTranslation } from 'react-i18next';
import '../styles/liquid.css';
import { useLiquidMotion } from '@/hooks/useLiquidMotion';

function PublicSurface() {
  const { publicInfo } = usePublicInfo();
  const isMobile = useIsMobile();
  const { t } = useTranslation();
  const settings = publicInfo?.theme_settings;
  const bgUrl = isMobile ? settings?.backgroundImageUrlMobile || settings?.backgroundImageUrlDesktop : settings?.backgroundImageUrlDesktop;
  const configuredWidth = Number(settings?.mainContentWidth ?? 100);
  const width = Number.isFinite(configuredWidth) ? Math.min(100, Math.max(30, configuredWidth)) : 100;
  const [reading, setReading] = useLocalStorage('komari-liquid-reading', false);
  const [reduced, setReduced] = useLocalStorage('komari-liquid-reduced-motion', false);
  const surfaceRef = useLiquidMotion(reduced === true);
  return <div ref={surfaceRef} className="km-layout layout flex flex-col w-full min-h-screen bg-cover bg-center bg-no-repeat" data-reading={reading === true} data-motion={reduced === true ? 'reduced' : 'normal'} style={bgUrl ? { backgroundImage: `url(${JSON.stringify(bgUrl)})` } : undefined}>
    <main className="km-main main-content h-full" style={{ width: `${width}vw`, marginInline: 'auto' }}>
      <NavBar />
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
