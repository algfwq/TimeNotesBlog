import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Button, Input, SideSheet, Spin, TextArea, Toast, Typography } from '@douyinfe/semi-ui';
import { IconArrowLeft, IconComment, IconLikeThumb } from '@douyinfe/semi-icons';
import { blogWS } from '../lib/wsClient';
import { loadTNoteFromUrl, type NoteDocument } from '../lib/tnote';
import { ReaderView } from '../components/ReaderView';

type Note = {
  id: string;
  title: string;
  ownerName?: string;
  likeCount: number;
  commentCount: number;
  downloadUrl?: string;
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
  const [doc, setDoc] = useState<NoteDocument | null>(null);
  const [liked, setLiked] = useState(false);
  const [comments, setComments] = useState<Comment[]>([]);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [nickname, setNickname] = useState('');
  const [email, setEmail] = useState('');
  const [githubUrl, setGithubUrl] = useState('');
  const [content, setContent] = useState('');
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await blogWS.connect();
        const res = await blogWS.request<{ note: Note; liked: boolean }>('notes.get', { id });
        if (cancelled) return;
        setNote(res.note);
        setLiked(Boolean(res.liked));
        const url = res.note.downloadUrl || '';
        const absolute = url.startsWith('http') ? url : `${location.origin}${url}`;
        const loaded = await loadTNoteFromUrl(absolute);
        if (!cancelled) {
          setDoc(loaded);
        }
        const cl = await blogWS.request<{ comments: Comment[] }>('notes.comments.list', { id });
        if (!cancelled) {
          setComments(cl.comments || []);
        }
        void blogWS.request('visit.track', {
          path: `/note/${id}`,
          noteId: id,
          userAgent: navigator.userAgent,
        }).catch(() => undefined);
      } catch (e) {
        Toast.error(`打开失败：${String(e)}`);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [id]);

  const title = useMemo(() => note?.title || doc?.title || '手账', [note, doc]);

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

  const onComment = async () => {
    setSubmitting(true);
    try {
      const res = await blogWS.request<{ comment: Comment }>('notes.comment.create', {
        id,
        nickname,
        email,
        githubUrl,
        content,
      });
      setComments((prev) => [res.comment, ...prev]);
      setContent('');
      setNote((n) => (n ? { ...n, commentCount: (n.commentCount || 0) + 1 } : n));
      Toast.success('评论已发布');
    } catch (e) {
      Toast.error(String(e));
    } finally {
      setSubmitting(false);
    }
  };

  if (loading) {
    return (
      <div style={{ minHeight: '70vh', display: 'grid', placeItems: 'center' }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div className="page-shell">
      <div className="glass" style={{ borderRadius: 18, padding: 16, marginBottom: 18, display: 'flex', gap: 12, alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap' }}>
        <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
          <Button icon={<IconArrowLeft />} theme="borderless" type="tertiary" onClick={() => navigate('/')}>
            返回
          </Button>
          <div>
            <Typography.Title heading={4} style={{ margin: 0, color: '#f4f1ea' }}>{title}</Typography.Title>
            <div className="muted" style={{ fontSize: 13 }}>上传者 @{note?.ownerName || 'unknown'}</div>
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <Button icon={<IconLikeThumb />} theme={liked ? 'solid' : 'light'} type="primary" onClick={onLike}>
            {note?.likeCount ?? 0}
          </Button>
          <Button icon={<IconComment />} theme="light" onClick={() => setSheetOpen(true)}>
            评论 {note?.commentCount ?? 0}
          </Button>
        </div>
      </div>

      {doc ? <ReaderView document={doc} /> : (
        <div className="glass" style={{ borderRadius: 16, padding: 24 }}>无法加载手账内容</div>
      )}

      <SideSheet
        title="评论区"
        visible={sheetOpen}
        onCancel={() => setSheetOpen(false)}
        width={Math.min(420, window.innerWidth)}
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          <Input placeholder="昵称" value={nickname} onChange={setNickname} />
          <Input placeholder="联系邮箱（与 GitHub 二选一）" value={email} onChange={setEmail} />
          <Input placeholder="GitHub 主页链接（可选）" value={githubUrl} onChange={setGithubUrl} />
          <TextArea placeholder="写下你的评论" value={content} onChange={setContent} rows={4} />
          <Button theme="solid" type="primary" loading={submitting} onClick={onComment}>
            发表评论
          </Button>
          <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 12 }}>
            {comments.map((c) => {
              const av = avatarFor(c);
              return (
                <div key={c.id} className="glass" style={{ borderRadius: 12, padding: 12, display: 'flex', gap: 10 }}>
                  {typeof av === 'string' ? (
                    <img src={av} alt="" width={40} height={40} style={{ borderRadius: '50%' }} />
                  ) : (
                    <div style={{ width: 40, height: 40, borderRadius: '50%', background: av.color, display: 'grid', placeItems: 'center', fontWeight: 700 }}>
                      {av.ch}
                    </div>
                  )}
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div style={{ fontWeight: 600 }}>{c.nickname}</div>
                    <div className="muted" style={{ fontSize: 12 }}>
                      {c.githubUrl ? (
                        <a href={c.githubUrl} target="_blank" rel="noreferrer" style={{ color: '#7dd3fc' }}>
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
    </div>
  );
}
