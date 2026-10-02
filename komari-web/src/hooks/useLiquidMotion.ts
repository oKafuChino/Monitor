import { useEffect, useRef } from 'react';

/** Passive decoration: scrolling and inertia remain owned by the browser. */
export function useLiquidMotion(disabled: boolean) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const surface = ref.current;
    if (!surface) return;
    const preference = matchMedia('(prefers-reduced-motion: reduce)');
    let frame = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let lastY = window.scrollY;
    let lastTime = performance.now();
    let decorated: HTMLElement[] = [];
    const clear = () => {
      decorated.forEach(card => {
        card.style.removeProperty('--liquid-streak');
        card.style.removeProperty('--liquid-streak-opacity');
      });
      decorated = [];
    };
    const sample = () => {
      frame = 0;
      const now = performance.now();
      const y = window.scrollY;
      const velocity = (y - lastY) / Math.max(16, now - lastTime) * 1000;
      lastY = y;
      lastTime = now;
      clear();
      if (disabled || preference.matches || Math.abs(velocity) < 600) return;
      const length = Math.min(window.innerWidth < 768 ? 6 : 12, (Math.abs(velocity) - 600) / 100);
      // Only decorate the first three visible surfaces, never promote the full list.
      for (const card of surface.querySelectorAll<HTMLElement>('.km-node-card')) {
        const box = card.getBoundingClientRect();
        if (box.bottom < 0 || box.top > innerHeight) continue;
        card.style.setProperty('--liquid-streak', `${Math.sign(velocity) * length}px`);
        card.style.setProperty('--liquid-streak-opacity', '.12');
        decorated.push(card);
        if (decorated.length === 3) break;
      }
      clearTimeout(timer);
      timer = setTimeout(clear, 100);
    };
    const scroll = () => { if (!frame) frame = requestAnimationFrame(sample); };
    const visibility = () => { if (document.hidden) clear(); };
    window.addEventListener('scroll', scroll, { passive: true });
    preference.addEventListener('change', clear);
    document.addEventListener('visibilitychange', visibility);
    return () => {
      cancelAnimationFrame(frame);
      clearTimeout(timer);
      clear();
      window.removeEventListener('scroll', scroll);
      preference.removeEventListener('change', clear);
      document.removeEventListener('visibilitychange', visibility);
    };
  }, [disabled]);
  return ref;
}
