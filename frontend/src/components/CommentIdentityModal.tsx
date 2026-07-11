import { useMemo, useState } from 'react';
import { Form, Input, Modal, Tabs, TabPane, Typography } from '@douyinfe/semi-ui';
import type { CommentIdentity } from '../lib/commentIdentity';

export function CommentIdentityModal({
  visible,
  initial,
  onCancel,
  onSubmit,
}: {
  visible: boolean;
  initial?: CommentIdentity | null;
  onCancel: () => void;
  onSubmit: (identity: CommentIdentity) => void;
}) {
  const initialTab = useMemo(() => (initial?.githubUrl ? 'github' : 'email'), [initial]);
  const [tab, setTab] = useState<'email' | 'github'>(initialTab);
  const [nickname, setNickname] = useState(initial?.nickname || '');
  const [email, setEmail] = useState(initial?.email || '');
  const [githubUrl, setGithubUrl] = useState(initial?.githubUrl || '');
  const [error, setError] = useState('');

  const submit = () => {
    if (tab === 'email') {
      const next: CommentIdentity = {
        nickname: nickname.trim(),
        email: email.trim(),
        githubUrl: '',
      };
      if (!next.nickname) {
        setError('请填写昵称');
        return;
      }
      if (!next.email || !next.email.includes('@')) {
        setError('请填写有效邮箱');
        return;
      }
      setError('');
      onSubmit(next);
      return;
    }

    const gh = githubUrl.trim();
    try {
      const u = new URL(gh);
      if (!['github.com', 'www.github.com'].includes(u.hostname.toLowerCase())) {
        setError('GitHub 链接无效');
        return;
      }
      const user = u.pathname.split('/').filter(Boolean)[0] || '';
      if (!user) {
        setError('请填写 GitHub 主页，例如 https://github.com/username');
        return;
      }
      setError('');
      onSubmit({
        nickname: user,
        email: '',
        githubUrl: `https://github.com/${user}`,
      });
    } catch {
      setError('GitHub 链接无效');
    }
  };

  return (
    <Modal
      title="完善评论身份"
      visible={visible}
      onCancel={onCancel}
      onOk={submit}
      okText="保存并发表"
      cancelText="取消"
      maskClosable={false}
    >
      <Typography.Paragraph type="tertiary" style={{ marginTop: 0 }}>
        首次评论需要填写身份信息，之后会保存在本机 Cookie。
      </Typography.Paragraph>
      <Tabs
        type="line"
        activeKey={tab}
        onChange={(key) => {
          setTab(key === 'github' ? 'github' : 'email');
          setError('');
        }}
      >
        <TabPane tab="邮箱" itemKey="email">
          <Form layout="vertical" style={{ marginTop: 12 }}>
            <Form.Slot label="昵称（必填）">
              <Input value={nickname} onChange={setNickname} placeholder="显示名称" />
            </Form.Slot>
            <Form.Slot label="邮箱（必填）">
              <Input value={email} onChange={setEmail} placeholder="name@example.com" />
            </Form.Slot>
          </Form>
        </TabPane>
        <TabPane tab="GitHub" itemKey="github">
          <Form layout="vertical" style={{ marginTop: 12 }}>
            <Form.Slot label="GitHub 主页（必填）">
              <Input value={githubUrl} onChange={setGithubUrl} placeholder="https://github.com/username" />
            </Form.Slot>
            <Typography.Text type="tertiary">
              选择 GitHub 时无需填写昵称，昵称和头像将从 GitHub 账号读取。
            </Typography.Text>
          </Form>
        </TabPane>
      </Tabs>
      {error ? <Typography.Text type="danger">{error}</Typography.Text> : null}
    </Modal>
  );
}
