import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Empty, Spin, Typography } from '@douyinfe/semi-ui';
import { IconBook, IconComment, IconDownload, IconLikeThumb } from '@douyinfe/semi-icons';
import { blogWS } from '../lib/wsClient';
import { PublicNav } from '../components/PublicNav';
import { rememberHomeScroll, restoreHomeScroll } from '../lib/homeScroll';
import { defaultSiteSettings, isHeroVideo, resolveHeroBackground, type SiteSettings } from '../types/site';

type Note = {
  id: string;
  title: string;
  ownerName?: string;
  likeCount: number;
  commentCount: number;
  updatedAt: string;
  coverUrl?: string;
  publicDownload?: boolean;
  visible?: boolean;
};

function coverSrc(note: Note) {
  if (!note.coverUrl) return '';
  return note.coverUrl.startsWith('http') ? note.coverUrl : `${location.origin}${note.coverUrl}`;
}

function hexToRgba(hex: string, alpha: number): string {
  const h = hex.replace('#', '');
  if (h.length !== 6) return `rgba(11,13,18,${alpha})`;
  const r = parseInt(h.slice(0, 2), 16);
  const g = parseInt(h.slice(2, 4), 16);
  const b = parseInt(h.slice(4, 6), 16);
  return `rgba(${r},${g},${b},${alpha})`;
}

export function HomePage() {
  const [notes, setNotes] = useState<Note[]>([]);
  const [settings, setSettings] = useState<SiteSettings>(defaultSiteSettings);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [brokenCovers, setBrokenCovers] = useState<Record<string, boolean>>({});
  const [scrolledPastHero, setScrolledPastHero] = useState(false);
  const restoredScrollRef = useRef(false);
  const navigate = useNavigate();

  const openNote = (id: string) => {
    rememberHomeScroll(id);
    navigate(`/note/${id}`);
  };

  const loadNotes = async () => {
    await blogWS.connect();
    const res = await blogWS.request<{ notes: Note[] }>('notes.list', {});
    setNotes(res.notes || []);
  };

  const loadSettings = async () => {
    await blogWS.connect();
    const res = await blogWS.request<{ settings: SiteSettings }>('site.settings.get', {});
    if (res.settings) {
      setSettings({ ...defaultSiteSettings(), ...res.settings });
    }
  };

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await Promise.all([loadNotes(), loadSettings()]);
        void blogWS.request('visit.track', {
          path: '/',
          userAgent: navigator.userAgent,
        }).catch(() => undefined);
      } catch (e) {
        if (!cancelled) setError(String(e));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // After notes/settings finish loading, jump back to the pre-reader scroll offset.
  useLayoutEffect(() => {
    if (loading || restoredScrollRef.current) return;
    restoredScrollRef.current = true;
    if (restoreHomeScroll()) {
      setScrolledPastHero(window.scrollY > window.innerHeight * 0.55);
    }
  }, [loading, notes.length]);

  useEffect(() => {
    const onScroll = () => {
      setScrolledPastHero(window.scrollY > window.innerHeight * 0.55);
    };
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

  useEffect(() => {
    const unsubs = [
      blogWS.on('event.note.changed', (payload) => {
        const p = payload as { note?: Note };
        if (!p.note?.id) return;
        setNotes((prev) => {
          if (p.note!.visible === false) {
            return prev.filter((n) => n.id !== p.note!.id);
          }
          const idx = prev.findIndex((n) => n.id === p.note!.id);
          if (idx < 0) return [p.note!, ...prev];
          const next = prev.slice();
          next[idx] = { ...next[idx], ...p.note! };
          return next;
        });
        if (p.note.coverUrl) {
          setBrokenCovers((prev) => {
            if (!prev[p.note!.id]) return prev;
            const next = { ...prev };
            delete next[p.note!.id];
            return next;
          });
        }
      }),
      blogWS.on('event.note.deleted', (payload) => {
        const p = payload as { id?: string };
        if (!p.id) return;
        setNotes((prev) => prev.filter((n) => n.id !== p.id));
      }),
      blogWS.on('event.like.changed', (payload) => {
        const p = payload as { noteId?: string; likeCount?: number };
        if (!p.noteId) return;
        setNotes((prev) => prev.map((n) => (n.id === p.noteId ? { ...n, likeCount: Number(p.likeCount ?? n.likeCount) } : n)));
      }),
      blogWS.on('event.comment.created', (payload) => {
        const p = payload as { noteId?: string; commentCount?: number };
        if (!p.noteId) return;
        setNotes((prev) => prev.map((n) => (
          n.id === p.noteId
            ? { ...n, commentCount: Math.max(n.commentCount || 0, Number(p.commentCount ?? n.commentCount)) }
            : n
        )));
      }),
      blogWS.on('event.site-settings.changed', (payload) => {
        const p = payload as { settings?: SiteSettings };
        if (p.settings) {
          setSettings({ ...defaultSiteSettings(), ...p.settings });
        }
      }),
      blogWS.onSnapshot(async () => {
        try {
          await Promise.all([loadNotes(), loadSettings()]);
        } catch {
          // ignore
        }
      }),
    ];
    return () => {
      unsubs.forEach((u) => u());
    };
  }, []);

  const heroBg = useMemo(() => resolveHeroBackground(settings), [settings]);
  const heroVideo = isHeroVideo(settings);
  const overlay = hexToRgba(settings.overlayColor || '#0b0d12', Number(settings.overlayOpacity ?? 0.45));
  const focusPos = `${settings.focusX}% ${settings.focusY}%`;

  return (
    <div className="home-page">
      <div className={`home-sticky-nav ${scrolledPastHero ? 'is-visible' : ''}`}>
        <PublicNav compact brandTitle={settings.navTitle || settings.heroTitle || 'TimeNotes Blog'} />
      </div>

      <section
        className="home-hero"
        style={
          heroBg && !heroVideo
            ? {
                backgroundImage: `url(${heroBg})`,
                backgroundPosition: focusPos,
              }
            : undefined
        }
      >
        {heroBg && heroVideo ? (
          <video
            key={heroBg}
            className="home-hero-video"
            src={heroBg}
            autoPlay
            muted
            loop
            playsInline
            // silent wallpaper: no controls, no audio
            controls={false}
            style={{ objectPosition: focusPos }}
          />
        ) : null}
        <div className="home-hero-overlay" style={{ background: overlay }} />
        <div className="home-hero-inner">
          <div className="home-hero-top">
            <PublicNav brandTitle={settings.navTitle || settings.heroTitle || 'TimeNotes Blog'} />
          </div>
          <div className="home-hero-content">
            <div className="home-hero-badge">
              <IconBook /> 公开手账馆
            </div>
            <h1 className="home-hero-title">{settings.heroTitle || 'TimeNotes Blog'}</h1>
            <p className="home-hero-subtitle">{settings.heroSubtitle}</p>
            <button
              type="button"
              className="home-hero-cta"
              onClick={() => document.getElementById('notes-grid')?.scrollIntoView({ behavior: 'smooth' })}
            >
              向下浏览
            </button>
          </div>
          <div className="home-hero-scroll-hint" aria-hidden>↓</div>
        </div>
      </section>

      <section id="notes-grid" className="home-grid-section">
        <div className="home-grid-header">
          <Typography.Title heading={3} style={{ margin: 0 }}>手账本</Typography.Title>
          <Typography.Text type="tertiary">{notes.length} 本公开</Typography.Text>
        </div>

        {loading ? (
          <div className="home-grid-state">
            <Spin size="large" />
          </div>
        ) : error ? (
          <div className="home-grid-state glass-panel">
            <Typography.Text type="danger">加载失败：{error}</Typography.Text>
          </div>
        ) : notes.length === 0 ? (
          <div className="home-grid-state glass-panel">
            <Empty description="还没有公开的手账本" />
          </div>
        ) : (
          <div className="note-grid note-grid--manager">
            {notes.map((note) => {
              const src = coverSrc(note);
              const broken = !src || brokenCovers[note.id];
              return (
                <button
                  key={note.id}
                  type="button"
                  className="note-card glass-panel"
                  onClick={() => openNote(note.id)}
                >
                  <div className="note-cover">
                    {broken ? (
                      <div className="note-cover-broken">
                        <div>
                          <strong>封面缺失/损坏</strong>
                          <div style={{ fontSize: 13 }}>{note.title || '未命名手账'}</div>
                          <div style={{ fontSize: 12, marginTop: 8 }}>请通过 TimeNotes 客户端重新生成封面后上传</div>
                        </div>
                      </div>
                    ) : (
                      <img
                        className="note-cover-image"
                        src={src}
                        alt={note.title || 'cover'}
                        onError={() => setBrokenCovers((prev) => ({ ...prev, [note.id]: true }))}
                      />
                    )}
                  </div>
                  <div className="note-card-body">
                    <div className="note-card-title">{note.title || '未命名手账'}</div>
                    <div className="note-card-meta">@{note.ownerName || 'unknown'}</div>
                    <div className="note-card-footer">
                      <span>{new Date(note.updatedAt).toLocaleDateString()}</span>
                      <span className="note-card-stats">
                        {note.publicDownload ? (
                          <span className="public-download-tag" title="允许公开下载">
                            <IconDownload size="small" /> 可下载
                          </span>
                        ) : null}
                        <span><IconLikeThumb size="small" /> {note.likeCount}</span>
                        <span><IconComment size="small" /> {note.commentCount}</span>
                      </span>
                    </div>
                  </div>
                </button>
              );
            })}
          </div>
        )}
      </section>
    </div>
  );
}
