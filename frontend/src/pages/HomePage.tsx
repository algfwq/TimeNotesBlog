import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Spin, Typography } from '@douyinfe/semi-ui';
import { IconBook, IconComment, IconDownload, IconLikeThumb } from '@douyinfe/semi-icons';
import { blogWS } from '../lib/wsClient';

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
  if (!note.coverUrl) {
    return '';
  }
  return note.coverUrl.startsWith('http') ? note.coverUrl : `${location.origin}${note.coverUrl}`;
}

export function HomePage() {
  const [notes, setNotes] = useState<Note[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [brokenCovers, setBrokenCovers] = useState<Record<string, boolean>>({});
  const navigate = useNavigate();

  const loadNotes = async () => {
    await blogWS.connect();
    const res = await blogWS.request<{ notes: Note[] }>('notes.list', {});
    setNotes(res.notes || []);
  };

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await loadNotes();
        void blogWS.request('visit.track', {
          path: '/',
          userAgent: navigator.userAgent,
        }).catch(() => undefined);
      } catch (e) {
        if (!cancelled) {
          setError(String(e));
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
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
          if (idx < 0) {
            return [p.note!, ...prev];
          }
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
      blogWS.onSnapshot(async () => {
        try {
          await loadNotes();
        } catch {
          // ignore
        }
      }),
    ];
    return () => {
      unsubs.forEach((u) => u());
    };
  }, []);

  return (
    <div className="page-shell">
      <header className="glass" style={{ borderRadius: 22, padding: '28px 26px', marginBottom: 24 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <IconBook size="extra-large" style={{ color: '#7dd3fc' }} />
          <div>
            <h1 className="hero-title">TimeNotes Blog</h1>
            <p className="muted" style={{ margin: '6px 0 0' }}>
              浏览公开手账本 · 点赞 · 评论
            </p>
          </div>
        </div>
      </header>

      {loading ? (
        <div style={{ display: 'grid', placeItems: 'center', minHeight: 240 }}>
          <Spin size="large" />
        </div>
      ) : error ? (
        <div className="glass" style={{ borderRadius: 16, padding: 20 }}>
          <Typography.Text type="danger">加载失败：{error}</Typography.Text>
        </div>
      ) : notes.length === 0 ? (
        <div className="glass" style={{ borderRadius: 16, padding: 28, textAlign: 'center' }}>
          <p className="muted">还没有公开的手账本</p>
        </div>
      ) : (
        <div className="note-grid">
          {notes.map((note) => {
            const src = coverSrc(note);
            const broken = !src || brokenCovers[note.id];
            return (
              <button
                key={note.id}
                type="button"
                className="note-card glass"
                style={{ border: 'none', color: 'inherit', textAlign: 'left', padding: 0 }}
                onClick={() => navigate(`/note/${note.id}`)}
              >
                <div className="note-cover" style={{ padding: 0, display: 'block' }}>
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
                <div style={{ padding: '12px 14px' }}>
                  <div style={{ fontSize: 15, fontWeight: 700, marginBottom: 4 }}>{note.title || '未命名手账'}</div>
                  <div className="muted" style={{ fontSize: 12, marginBottom: 8 }}>@{note.ownerName || 'unknown'}</div>
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'center' }}>
                    <span className="muted" style={{ fontSize: 12 }}>
                      {new Date(note.updatedAt).toLocaleDateString()}
                    </span>
                    <span style={{ display: 'flex', gap: 10, fontSize: 12, alignItems: 'center' }} className="muted">
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
    </div>
  );
}
