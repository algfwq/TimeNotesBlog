import type { AssetMeta } from '../types';

export function fontFamilyForAsset(font: AssetMeta) {
  return `TNFont_${font.id.replace(/[^a-zA-Z0-9_-]/g, '_')}`;
}
