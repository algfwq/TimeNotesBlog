import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Spin, Typography } from '@douyinfe/semi-ui';
import { IconBook, IconComment, IconLikeThumb } from '@douyinfe/semi-icons';
import { blogWS } from '../lib/wsClient';

type Note = {
  id: string;
  title: string;
  ownerName?: string;
  likeCount: number;
  commentCount: number;
  updatedAt: string;
};

export function HomePage() {
  const [notes, setNotes] = useState<Note[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await blogWS.connect();
        const res = await blogWS.request<{ notes: Note[] }>('notes.list', {});
        if (!cancelled) {
          setNotes(res.notes || []);
        }
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
          {notes.map((note) => (
            <button
              key={note.id}
              type="button"
              className="note-card glass"
              style={{ border: 'none', color: 'inherit', textAlign: 'left', padding: 0 }}
              onClick={() => navigate(`/note/${note.id}`)}
            >
              <div className="note-cover">
                <div>
                  <div style={{ fontSize: 18, fontWeight: 700 }}>{note.title || '未命名手账'}</div>
                  <div className="muted" style={{ marginTop: 8, fontSize: 13 }}>
                    @{note.ownerName || 'unknown'}
                  </div>
                </div>
              </div>
              <div style={{ padding: '12px 14px', display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                <span className="muted" style={{ fontSize: 12 }}>
                  {new Date(note.updatedAt).toLocaleDateString()}
                </span>
                <span style={{ display: 'flex', gap: 10, fontSize: 12 }} className="muted">
                  <span><IconLikeThumb size="small" /> {note.likeCount}</span>
                  <span><IconComment size="small" /> {note.commentCount}</span>
                </span>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
