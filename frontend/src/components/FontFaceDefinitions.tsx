import { useEffect, useMemo } from 'react';
import { fontFamilyForAsset } from '../lib/fonts';
import type { AssetMeta } from '../types';

export function FontFaceDefinitions({ fonts }: { fonts: AssetMeta[] }) {
  const fontFaces = useMemo(
    () =>
      fonts
        .filter((font) => font.dataUrl || font.dataBase64)
        .map((font) => ({
          family: fontFamilyForAsset(font),
          src: font.dataUrl ?? `data:${font.mimeType};base64,${font.dataBase64}`,
        })),
    [fonts],
  );
  const css = useMemo(
    () => fontFaces.map((font) => `@font-face{font-family:"${font.family}";src:url("${font.src}");font-display:swap;}`).join('\n'),
    [fontFaces],
  );

  useEffect(() => {
    const fontSet = globalThis.document?.fonts;
    if (!('FontFace' in window) || !fontSet || fontFaces.length === 0) {
      return;
    }
    const loaded = fontFaces.map((font) => {
      const face = new FontFace(font.family, `url("${font.src}")`, { display: 'swap' });
      fontSet.add(face);
      void face.load().catch(() => undefined);
      return face;
    });
    return () => {
      loaded.forEach((face) => fontSet.delete(face));
    };
  }, [fontFaces]);

  return css ? <style data-timenotes-fonts>{css}</style> : null;
}
