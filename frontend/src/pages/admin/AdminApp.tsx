import { useEffect, useMemo, useState } from 'react';
import { Button, Input, Modal, Select, Switch, Table, TabPane, Tabs, Toast, Typography, Upload } from '@douyinfe/semi-ui';
import { IconCloud, IconDelete, IconEyeOpened, IconUser, IconHistogram } from '@douyinfe/semi-icons';
import { VChart } from '@visactor/react-vchart';
import { blogWS } from '../../lib/wsClient';

type User = {
  id: string;
  username: string;
  role: string;
  canUpload: boolean;
};

type Note = {
  id: string;
  title: string;
  filename: string;
  ownerName?: string;
  visible: boolean;
  likeCount: number;
  commentCount: number;
  sizeBytes: number;
  updatedAt: string;
};

type Stats = {
  todayCount: number;
  recentCount: number;
  daily: Array<{ date: string; count: number }>;
  locations: Array<{ country: string; region: string; city: string; lat: number; lng: number; count: number }>;
  noteStats: Array<{ noteId: string; title: string; likeCount: number; commentCount: number; visible: boolean }>;
};

const TOKEN_KEY = 'tn_blog_admin_token';

export function AdminApp() {
  const [token, setToken] = useState(localStorage.getItem(TOKEN_KEY) || '');
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  const [authed, setAuthed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [users, setUsers] = useState<User[]>([]);
  const [notes, setNotes] = useState<Note[]>([]);
  const [stats, setStats] = useState<Stats | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [newUser, setNewUser] = useState({ username: '', password: '', role: 'user', canUpload: true });

  const login = async () => {
    setBusy(true);
    try {
      await blogWS.connect();
      const res = await blogWS.login(username, password);
      if (res.role !== 'admin') {
        throw new Error('需要管理员账号');
      }
      localStorage.setItem(TOKEN_KEY, res.token);
      setToken(res.token);
      setAuthed(true);
      Toast.success('登录成功');
      await refreshAll();
    } catch (e) {
      Toast.error(String(e));
    } finally {
      setBusy(false);
    }
  };

  const ensureAuth = async () => {
    if (!token) return;
    try {
      await blogWS.connect();
      await blogWS.loginWithToken(token);
      setAuthed(true);
      await refreshAll();
    } catch {
      localStorage.removeItem(TOKEN_KEY);
      setToken('');
      setAuthed(false);
    }
  };

  const refreshAll = async () => {
    const [u, n, s] = await Promise.all([
      blogWS.request<{ users: User[] }>('admin.users.list', {}),
      blogWS.request<{ notes: Note[] }>('admin.notes.list', {}),
      blogWS.request<Stats>('admin.stats', {}),
    ]);
    setUsers(u.users || []);
    setNotes(n.notes || []);
    setStats(s);
  };

  useEffect(() => {
    void ensureAuth();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const visitSpec = useMemo(() => {
    const values = (stats?.daily || []).map((d) => ({ date: d.date, count: d.count }));
    return {
      type: 'area',
      data: [{ id: 'daily', values }],
      xField: 'date',
      yField: 'count',
      title: { text: '近 14 日访问量' },
      axes: [{ orient: 'left' }, { orient: 'bottom' }],
    };
  }, [stats]);

  const mapSpec = useMemo(() => {
    const values = (stats?.locations || []).map((l) => ({
      lat: l.lat,
      lng: l.lng,
      size: l.count,
      name: [l.city, l.region, l.country].filter(Boolean).join(', '),
    }));
    return {
      type: 'common',
      data: [{ id: 'loc', values }],
      series: [
        {
          type: 'scatter',
          xField: 'lng',
          yField: 'lat',
          sizeField: 'size',
          size: { type: 'linear', range: [6, 24] },
          point: { style: { fill: '#60a5fa', fillOpacity: 0.75 } },
        },
      ],
      title: { text: '访客地理分布（散点近似）' },
      axes: [
        { orient: 'left', title: { text: 'Lat' }, min: -90, max: 90 },
        { orient: 'bottom', title: { text: 'Lng' }, min: -180, max: 180 },
      ],
    };
  }, [stats]);

  const uploadFile = async (file: File) => {
    const buf = await file.arrayBuffer();
    const bytes = new Uint8Array(buf);
    const hashBuf = await crypto.subtle.digest('SHA-256', bytes);
    const sha256 = [...new Uint8Array(hashBuf)].map((b) => b.toString(16).padStart(2, '0')).join('');
    const start = await blogWS.request<{ uploadId: string; chunkSize: number }>('admin.notes.upload.start', {
      filename: file.name,
      title: file.name.replace(/\.tnote$/i, ''),
      size: bytes.length,
    });
    const chunkSize = start.chunkSize || 256 * 1024;
    for (let offset = 0, index = 0; offset < bytes.length; offset += chunkSize, index++) {
      const slice = bytes.subarray(offset, Math.min(offset + chunkSize, bytes.length));
      let binary = '';
      for (let i = 0; i < slice.length; i++) binary += String.fromCharCode(slice[i]);
      const data = btoa(binary);
      await blogWS.request('admin.notes.upload.chunk', {
        uploadId: start.uploadId,
        index,
        data,
      }, 120000);
    }
    await blogWS.request('admin.notes.upload.finish', {
      uploadId: start.uploadId,
      sha256,
    }, 120000);
    Toast.success('上传成功');
    await refreshAll();
  };

  const openInClient = async (note: Note) => {
    try {
      const res = await blogWS.request<{ note: { downloadUrl?: string; filename: string } }>('notes.get', { id: note.id });
      const url = res.note.downloadUrl || '';
      const absolute = url.startsWith('http') ? url : `${location.origin}${url}`;
      const resp = await fetch('http://127.0.0.1:54088/api/blog-bridge/open', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ downloadUrl: absolute, filename: res.note.filename || note.filename }),
      });
      if (!resp.ok) {
        throw new Error(await resp.text());
      }
      Toast.success('已请求客户端打开');
    } catch (e) {
      Toast.error(`打开失败，请确认 TimeNotes 客户端已启动：${String(e)}`);
    }
  };

  if (!authed) {
    return (
      <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', padding: 20 }}>
        <div className="glass-strong" style={{ width: min(400, '100%'), borderRadius: 18, padding: 24 }}>
          <Typography.Title heading={3} style={{ color: '#f4f1ea', marginTop: 0 }}>后台登录</Typography.Title>
          <p className="muted">使用管理员账号登录（含 PoW 验证）</p>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginTop: 16 }}>
            <Input prefix={<IconUser />} value={username} onChange={setUsername} placeholder="用户名" />
            <Input mode="password" value={password} onChange={setPassword} placeholder="密码" onEnterPress={login} />
            <Button theme="solid" type="primary" loading={busy} onClick={login}>登录</Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="admin-layout">
      <aside className="admin-side glass-strong">
        <Typography.Title heading={4} style={{ color: '#f4f1ea', marginTop: 0 }}>TimeNotes Admin</Typography.Title>
        <p className="muted" style={{ fontSize: 12 }}>安全后台 · 本次会话已验证</p>
        <Button block theme="borderless" type="danger" style={{ marginTop: 16 }} onClick={() => {
          localStorage.removeItem(TOKEN_KEY);
          setAuthed(false);
          setToken('');
        }}>
          退出登录
        </Button>
      </aside>
      <main className="admin-main">
        <Tabs type="line" keepDOM={false}>
          <TabPane tab={<span><IconHistogram /> 仪表盘</span>} itemKey="dash">
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit,minmax(180px,1fr))', gap: 12, marginBottom: 16 }}>
              <div className="glass" style={{ borderRadius: 14, padding: 16 }}>
                <div className="muted">今日访问</div>
                <div style={{ fontSize: 28, fontWeight: 700 }}>{stats?.todayCount ?? 0}</div>
              </div>
              <div className="glass" style={{ borderRadius: 14, padding: 16 }}>
                <div className="muted">近 14 日访问</div>
                <div style={{ fontSize: 28, fontWeight: 700 }}>{stats?.recentCount ?? 0}</div>
              </div>
              <div className="glass" style={{ borderRadius: 14, padding: 16 }}>
                <div className="muted">手账数</div>
                <div style={{ fontSize: 28, fontWeight: 700 }}>{notes.length}</div>
              </div>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr', gap: 12 }}>
              <div className="glass" style={{ borderRadius: 14, padding: 8, minHeight: 320 }}>
                {stats ? <VChart spec={visitSpec as never} /> : null}
              </div>
              <div className="glass" style={{ borderRadius: 14, padding: 8, minHeight: 320 }}>
                {stats ? <VChart spec={mapSpec as never} /> : null}
              </div>
            </div>
            <div className="glass" style={{ borderRadius: 14, padding: 12, marginTop: 12 }}>
              <Typography.Title heading={5} style={{ color: '#f4f1ea' }}>手账互动</Typography.Title>
              <Table
                dataSource={stats?.noteStats || []}
                rowKey="noteId"
                pagination={false}
                columns={[
                  { title: '标题', dataIndex: 'title' },
                  { title: '点赞', dataIndex: 'likeCount', width: 90 },
                  { title: '评论', dataIndex: 'commentCount', width: 90 },
                  { title: '可见', dataIndex: 'visible', width: 90, render: (v) => (v ? '是' : '否') },
                ]}
              />
            </div>
          </TabPane>

          <TabPane tab={<span><IconCloud /> 手账管理</span>} itemKey="notes">
            <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
              <Upload
                action=""
                accept=".tnote"
                showUploadList={false}
                customRequest={async ({ file, onSuccess, onError }) => {
                  try {
                    const raw = (file as { fileInstance?: File }).fileInstance || (file as unknown as File);
                    await uploadFile(raw);
                    onSuccess?.({});
                  } catch (e) {
                    Toast.error(String(e));
                    onError?.({ status: 500 });
                  }
                }}
              >
                <Button theme="solid" type="primary">上传 .tnote</Button>
              </Upload>
              <Button onClick={() => void refreshAll()}>刷新</Button>
            </div>
            <Table
              dataSource={notes}
              rowKey="id"
              columns={[
                { title: '标题', dataIndex: 'title' },
                { title: '文件名', dataIndex: 'filename' },
                { title: '上传者', dataIndex: 'ownerName' },
                { title: '点赞', dataIndex: 'likeCount', width: 80 },
                { title: '评论', dataIndex: 'commentCount', width: 80 },
                {
                  title: '可见',
                  dataIndex: 'visible',
                  width: 100,
                  render: (v: boolean, r: Note) => (
                    <Switch
                      checked={v}
                      onChange={async (checked) => {
                        await blogWS.request('admin.notes.set_visible', { id: r.id, visible: checked });
                        await refreshAll();
                      }}
                    />
                  ),
                },
                {
                  title: '操作',
                  width: 220,
                  render: (_: unknown, r: Note) => (
                    <div style={{ display: 'flex', gap: 6 }}>
                      <Button size="small" icon={<IconEyeOpened />} onClick={() => void openInClient(r)}>编辑</Button>
                      <Button
                        size="small"
                        type="danger"
                        icon={<IconDelete />}
                        onClick={async () => {
                          await blogWS.request('admin.notes.delete', { id: r.id });
                          Toast.success('已删除');
                          await refreshAll();
                        }}
                      >
                        删除
                      </Button>
                    </div>
                  ),
                },
              ]}
            />
          </TabPane>

          <TabPane tab={<span><IconUser /> 用户管理</span>} itemKey="users">
            <div style={{ marginBottom: 12 }}>
              <Button theme="solid" type="primary" onClick={() => setCreateOpen(true)}>添加用户</Button>
            </div>
            <Table
              dataSource={users}
              rowKey="id"
              columns={[
                { title: '用户名', dataIndex: 'username' },
                { title: '角色', dataIndex: 'role', width: 100 },
                {
                  title: '可上传',
                  dataIndex: 'canUpload',
                  width: 100,
                  render: (v: boolean, r: User) => (
                    <Switch
                      checked={v}
                      onChange={async (checked) => {
                        await blogWS.request('admin.users.update', { id: r.id, canUpload: checked });
                        await refreshAll();
                      }}
                    />
                  ),
                },
                {
                  title: '设为管理员',
                  width: 120,
                  render: (_: unknown, r: User) => (
                    <Switch
                      checked={r.role === 'admin'}
                      onChange={async (checked) => {
                        await blogWS.request('admin.users.update', { id: r.id, role: checked ? 'admin' : 'user' });
                        await refreshAll();
                      }}
                    />
                  ),
                },
                {
                  title: '操作',
                  width: 120,
                  render: (_: unknown, r: User) => (
                    <Button
                      size="small"
                      type="danger"
                      onClick={async () => {
                        await blogWS.request('admin.users.delete', { id: r.id });
                        Toast.success('已删除用户');
                        await refreshAll();
                      }}
                    >
                      删除
                    </Button>
                  ),
                },
              ]}
            />
          </TabPane>
        </Tabs>
      </main>

      <Modal
        title="添加用户"
        visible={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={async () => {
          await blogWS.request('admin.users.create', newUser);
          Toast.success('用户已创建');
          setCreateOpen(false);
          setNewUser({ username: '', password: '', role: 'user', canUpload: true });
          await refreshAll();
        }}
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          <Input
            placeholder="用户名"
            value={newUser.username}
            onChange={(v) => setNewUser((s) => ({ ...s, username: v }))}
          />
          <Input
            mode="password"
            placeholder="密码"
            value={newUser.password}
            onChange={(v) => setNewUser((s) => ({ ...s, password: v }))}
          />
          <Select
            value={newUser.role}
            onChange={(v) => setNewUser((s) => ({ ...s, role: String(v) }))}
          >
            <Select.Option value="user">user</Select.Option>
            <Select.Option value="admin">admin</Select.Option>
          </Select>
        </div>
      </Modal>
    </div>
  );
}

function min(a: number, b: string | number) {
  if (typeof b === 'string') return `min(${a}px, ${b})`;
  return Math.min(a, b);
}
