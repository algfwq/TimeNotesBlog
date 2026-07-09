import { useMemo, type CSSProperties } from 'react';
import type { AssetMeta, NoteDocument } from '../lib/tnote';
import { assetMap } from '../lib/tnote';

function resolveAsset(map: Record<string, AssetMeta>, id?: string) {
  if (!id) return undefined;
  return map[id];
}

export function ReaderView({ document }: { document: NoteDocument }) {
  const map = useMemo(() => assetMap(document), [document]);
  const pages = document.pages || [];
  const elements = document.elements || [];

  return (
    <div className="reader-stage glass">
      {pages.map((page) => {
        const pageEls = elements
          .filter((el) => el.pageId === page.id)
          .sort((a, b) => (a.zIndex || 0) - (b.zIndex || 0));
        const bgAsset = resolveAsset(map, page.backgroundAssetId);
        return (
          <div
            key={page.id}
            className="reader-page"
            style={{
              width: page.width,
              height: page.height,
              background: page.background || '#fffaf0',
              backgroundImage: bgAsset?.dataUrl ? `url(${bgAsset.dataUrl})` : undefined,
              backgroundSize: 'cover',
              backgroundPosition: 'center',
            }}
          >
            {pageEls.map((el) => {
              const style: CSSProperties = {
                left: el.x,
                top: el.y,
                width: el.width,
                height: el.height,
                transform: el.rotation ? `rotate(${el.rotation}deg)` : undefined,
              };
              if (el.type === 'text') {
                return (
                  <div
                    key={el.id}
                    className="reader-el"
                    style={style}
                    dangerouslySetInnerHTML={{ __html: String(el.content || '') }}
                  />
                );
              }
              if (el.type === 'code') {
                return (
                  <pre
                    key={el.id}
                    className="reader-el"
                    style={{
                      ...style,
                      margin: 0,
                      padding: 8,
                      background: '#1e1e1e',
                      color: '#d4d4d4',
                      fontSize: 12,
                      overflow: 'auto',
                    }}
                  >
                    {String(el.content || '')}
                  </pre>
                );
              }
              if (el.type === 'image' || el.type === 'sticker') {
                const asset = resolveAsset(map, el.assetId);
                return (
                  <div key={el.id} className="reader-el" style={style}>
                    {asset?.dataUrl ? (
                      <img src={asset.dataUrl} alt="" style={{ width: '100%', height: '100%', objectFit: 'contain' }} />
                    ) : null}
                  </div>
                );
              }
              if (el.type === 'audio') {
                const asset = resolveAsset(map, el.assetId);
                return (
                  <div key={el.id} className="reader-el" style={style}>
                    {asset?.dataUrl ? <audio controls src={asset.dataUrl} style={{ width: '100%' }} /> : null}
                  </div>
                );
              }
              if (el.type === 'video') {
                const asset = resolveAsset(map, el.assetId);
                return (
                  <div key={el.id} className="reader-el" style={style}>
                    {asset?.dataUrl ? <video controls src={asset.dataUrl} style={{ width: '100%', height: '100%' }} /> : null}
                  </div>
                );
              }
              if (el.type === 'drawing' || el.type === 'tape') {
                const pts = el.points || [];
                const d = pts.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ');
                return (
                  <svg key={el.id} className="reader-el" style={style} viewBox={`0 0 ${el.width} ${el.height}`}>
                    <path d={d} stroke="#333" strokeWidth={2} fill="none" />
                  </svg>
                );
              }
              return (
                <div
                  key={el.id}
                  className="reader-el"
                  style={{
                    ...style,
                    background: 'rgba(0,0,0,0.06)',
                    display: 'grid',
                    placeItems: 'center',
                    fontSize: 12,
                  }}
                >
                  {el.type}
                </div>
              );
            })}
          </div>
        );
      })}
    </div>
  );
}
