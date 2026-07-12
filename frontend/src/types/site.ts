export type SiteSettings = {
  heroTitle: string;
  heroSubtitle: string;
  backgroundMode: 'none' | 'url' | 'upload';
  backgroundUrl?: string;
  backgroundAssetUrl?: string;
  focusX: number;
  focusY: number;
  overlayColor: string;
  overlayOpacity: number;
  updatedAt?: string;
};

export const defaultSiteSettings = (): SiteSettings => ({
  heroTitle: 'TimeNotes Blog',
  heroSubtitle: '浏览公开手账本 · 点赞 · 评论',
  backgroundMode: 'none',
  focusX: 50,
  focusY: 40,
  overlayColor: '#0b0d12',
  overlayOpacity: 0.45,
});

export function resolveHeroBackground(settings: SiteSettings): string {
  if (settings.backgroundMode === 'upload' && settings.backgroundAssetUrl) {
    return settings.backgroundAssetUrl.startsWith('http')
      ? settings.backgroundAssetUrl
      : `${location.origin}${settings.backgroundAssetUrl}`;
  }
  if (settings.backgroundMode === 'url' && settings.backgroundUrl) {
    return settings.backgroundUrl;
  }
  return '';
}
