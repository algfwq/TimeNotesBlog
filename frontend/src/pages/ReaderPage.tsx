import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Button, SideSheet, Spin, TextArea, Toast, Typography } from '@douyinfe/semi-ui';
import { blogWS } from '../lib/wsClient';
import { loadTNoteFromUrl, releaseTNoteObjectUrls, type LoadedTNote } from '../lib/tnote';
import { ReaderView } from '../components/ReaderView';
import { CommentIdentityModal } from '../components/CommentIdentityModal';
import { readCommentIdentity, writeCommentIdentity, type CommentIdentity } from '../lib/commentIdentity';

type Note = {
  id: string;
  title: string;
  ownerName?: string;
  likeCount: number;
  commentCount: number;
  downloadUrl?: string;
  publicDownload?: boolean;
  coverUrl?: string;
  visible?: boolean;
};

type NoteGetResponse = {
  note: Note;
  liked: boolean;
  canDownload?: boolean;
};

type Comment = {
  id: string;
  nickname: string;
  email: string;
  githubUrl: string;
  content: string;
  createdAt: string;
};

function avatarFor(c: Comment) {
  if (c.githubUrl) {
    try {
      const u = new URL(c.githubUrl);
      const user = u.pathname.split('/').filter(Boolean)[0];
      if (user) {
        return `https://github.com/${user}.png`;
      }
    } catch {
      // ignore
    }
  }
  const ch = (c.nickname || '?').trim().charAt(0).toUpperCase() || '?';
  const colors = ['#60a5fa', '#f472b6', '#34d399', '#fbbf24', '#a78bfa', '#fb7185'];
  let hash = 0;
  for (let i = 0; i < c.nickname.length; i++) {
    hash = (hash + c.nickname.charCodeAt(i) * 17) % colors.length;
  }
  return { ch, color: colors[hash] };
}

export function ReaderPage() {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [note, setNote] = useState<Note | null>(null);
  const [loaded, setLoaded] = useState<LoadedTNote | null>(null);
  const [liked, setLiked] = useState(false);
  const [comments, setComments] = useState<Comment[]>([]);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [content, setContent] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [identityOpen, setIdentityOpen] = useState(false);
  const [identity, setIdentity] = useState<CommentIdentity | null>(() => readCommentIdentity());
  const [canDownload, setCanDownload] = useState(false);
  const [unavailable, setUnavailable] = useState('');
  const objectUrlsRef = useRef<string[]>([]);

  const replaceLoaded = (next: LoadedTNote | null) => {
    releaseTNoteObjectUrls(objectUrlsRef.current);
    objectUrlsRef.current = next?.objectUrls ?? [];
    setLoaded(next);
  };

  const loadAll = async () => {
    await blogWS.connect();
    const res = await blogWS.request<NoteGetResponse>('notes.get', { id });
    setNote(res.note);
    setLiked(Boolean(res.liked));
    setCanDownload(Boolean(res.canDownload));
    setUnavailable('');
    const url = res.note.downloadUrl || '';
    if (url) {
      const absolute = url.startsWith('http') ? url : `${location.origin}${url}`;
      const next = await loadTNoteFromUrl(absolute);
      replaceLoaded(next);
    } else {
      replaceLoaded(null);
    }
    const cl = await blogWS.request<{ comments: Comment[] }>('notes.comments.list', { id });
    setComments(cl.comments || []);
    void blogWS.request('visit.track', {
      path: `/note/${id}`,
      noteId: id,
      userAgent: navigator.userAgent,
    }).catch(() => undefined);
  };

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await loadAll();
      } catch (e) {
        if (!cancelled) Toast.error(`打开失败：${String(e)}`);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
      releaseTNoteObjectUrls(objectUrlsRef.current);
      objectUrlsRef.current = [];
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  // Block browser page-zoom (trackpad pinch / ctrl+wheel) while the reader is open.
  // Notebook zoom is handled inside the stage with a non-passive listener.
  useEffect(() => {
    const blockBrowserZoom = (e: WheelEvent) => {
      if (e.ctrlKey || e.metaKey) {
        e.preventDefault();
      }
    };
    const blockGesture = (e: Event) => {
      e.preventDefault();
    };
    document.addEventListener('wheel', blockBrowserZoom, { passive: false, capture: true });
    document.addEventListener('gesturestart', blockGesture, { passive: false } as AddEventListenerOptions);
    document.addEventListener('gesturechange', blockGesture, { passive: false } as AddEventListenerOptions);
    return () => {
      document.removeEventListener('wheel', blockBrowserZoom, true);
      document.removeEventListener('gesturestart', blockGesture);
      document.removeEventListener('gesturechange', blockGesture);
    };
  }, []);

  useEffect(() => {
    const unsubs = [
      blogWS.on('event.like.changed', (payload) => {
        const p = payload as { noteId?: string; likeCount?: number };
        if (p.noteId !== id) return;
        setNote((n) => (n ? { ...n, likeCount: Number(p.likeCount || n.likeCount) } : n));
      }),
      blogWS.on('event.comment.created', (payload) => {
        const p = payload as { noteId?: string; comment?: Comment; commentCount?: number };
        if (p.noteId !== id || !p.comment) return;
        setComments((prev) => (prev.some((c) => c.id === p.comment!.id) ? prev : [p.comment!, ...prev]));
        setNote((n) => {
          if (!n) return n;
          const nextCount = Number(p.commentCount ?? n.commentCount);
          return { ...n, commentCount: Math.max(n.commentCount || 0, nextCount) };
        });
      }),
      blogWS.on('event.note.deleted', (payload) => {
        const p = payload as { id?: string };
        if (p.id !== id) return;
        setUnavailable('该手账已被删除');
        Toast.warning('该手账已不可用');
        window.setTimeout(() => navigate('/'), 1200);
      }),
      blogWS.on('event.note.changed', (payload) => {
        const p = payload as { note?: Note };
        if (!p.note || p.note.id !== id) return;
        if (p.note.visible === false) {
          setUnavailable('该手账已隐藏');
          Toast.warning('该手账已不可用');
          window.setTimeout(() => navigate('/'), 1200);
          return;
        }
        setNote((n) => (n ? { ...n, ...p.note } : p.note || n));
        if (p.note.publicDownload !== undefined) {
          setCanDownload(Boolean(p.note.publicDownload));
        }
      }),
      blogWS.onSnapshot(async () => {
        try {
          await loadAll();
        } catch {
          // ignore reconnect snapshot failures
        }
      }),
    ];
    return () => {
      unsubs.forEach((u) => u());
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, navigate]);

  const title = useMemo(() => note?.title || loaded?.document.title || '手账', [note, loaded]);

  const onLike = async () => {
    if (liked) {
      Toast.info('每个 IP 只能点赞一次');
      return;
    }
    try {
      const res = await blogWS.request<{ likeCount: number; liked: boolean }>('notes.like', { id });
      setLiked(true);
      setNote((n) => (n ? { ...n, likeCount: res.likeCount } : n));
      Toast.success('点赞成功');
    } catch (e) {
      Toast.error(String(e));
    }
  };

  const submitComment = async (who: CommentIdentity) => {
    setSubmitting(true);
    try {
      const res = await blogWS.request<{ comment: Comment }>('notes.comment.create', {
        id,
        nickname: who.nickname,
        email: who.email,
        githubUrl: who.githubUrl,
        content,
      });
      let inserted = false;
      setComments((prev) => {
        if (prev.some((c) => c.id === res.comment.id)) {
          return prev;
        }
        inserted = true;
        return [res.comment, ...prev];
      });
      setContent('');
      if (inserted) {
        setNote((n) => (n ? { ...n, commentCount: (n.commentCount || 0) + 1 } : n));
      }
      Toast.success('评论已发布');
    } catch (e) {
      Toast.error(String(e));
    } finally {
      setSubmitting(false);
    }
  };

  const onComment = async () => {
    if (!content.trim()) {
      Toast.warning('请输入评论内容');
      return;
    }
    const who = identity || readCommentIdentity();
    if (!who) {
      setIdentityOpen(true);
      return;
    }
    await submitComment(who);
  };

  const onDownload = async () => {
    try {
      const res = await blogWS.request<NoteGetResponse & { exportUrl?: string }>('notes.get', { id });
      const url = res.exportUrl || '';
      if (!url) throw new Error('download unavailable');
      window.location.href = url.startsWith('http') ? url : `${location.origin}${url}`;
    } catch (e) {
      Toast.error(String(e));
    }
  };

  if (loading) {
    return (
      <div className="reader-page reader-page--state">
        <Spin size="large" />
      </div>
    );
  }

  if (unavailable) {
    return (
      <div className="reader-page reader-page--state">
        <div className="glass-panel" style={{ borderRadius: 16, padding: 28, textAlign: 'center' }}>
          <Typography.Title heading={4} style={{ marginTop: 0 }}>{unavailable}</Typography.Title>
          <Button theme="solid" type="primary" onClick={() => navigate('/')}>返回首页</Button>
        </div>
      </div>
    );
  }

  return (
    <div className="reader-page">
      {loaded?.document ? (
        <ReaderView
          document={loaded.document}
          chrome={{
            title,
            ownerName: note?.ownerName,
            liked,
            likeCount: note?.likeCount ?? 0,
            commentCount: note?.commentCount ?? 0,
            canDownload,
            onBack: () => navigate('/'),
            onLike: () => void onLike(),
            onComment: () => setSheetOpen(true),
            onDownload: () => void onDownload(),
          }}
        />
      ) : (
        <div className="reader-page--state">
          <div className="glass-panel" style={{ borderRadius: 16, padding: 24 }}>无法加载手账内容</div>
        </div>
      )}

      <SideSheet
        title="评论区"
        visible={sheetOpen}
        onCancel={() => setSheetOpen(false)}
        width={Math.min(420, window.innerWidth)}
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          <TextArea placeholder="写下你的评论" value={content} onChange={setContent} rows={4} />
          <Button theme="solid" type="primary" loading={submitting} onClick={() => void onComment()}>
            发表评论
          </Button>
          <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 12 }}>
            {comments.map((c) => {
              const av = avatarFor(c);
              return (
                <div key={c.id} className="comment-card" style={{ borderRadius: 12, padding: 12, display: 'flex', gap: 10 }}>
                  {typeof av === 'string' ? (
                    <img src={av} alt="" width={40} height={40} style={{ borderRadius: '50%' }} />
                  ) : (
                    <div style={{ width: 40, height: 40, borderRadius: '50%', background: av.color, display: 'grid', placeItems: 'center', fontWeight: 700, color: '#fff' }}>
                      {av.ch}
                    </div>
                  )}
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div style={{ fontWeight: 600 }}>{c.nickname}</div>
                    <div className="muted" style={{ fontSize: 12 }}>
                      {c.githubUrl ? (
                        <a href={c.githubUrl} target="_blank" rel="noreferrer">
                          {c.githubUrl}
                        </a>
                      ) : (
                        c.email
                      )}
                    </div>
                    <div style={{ marginTop: 6, whiteSpace: 'pre-wrap' }}>{c.content}</div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </SideSheet>

      <CommentIdentityModal
        visible={identityOpen}
        initial={identity}
        onCancel={() => setIdentityOpen(false)}
        onSubmit={(next) => {
          writeCommentIdentity(next);
          setIdentity(next);
          setIdentityOpen(false);
          void submitComment(next);
        }}
      />
    </div>
  );
}
