import { useState } from 'react';
import { Form, Input, Modal, Typography } from '@douyinfe/semi-ui';
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
  const [nickname, setNickname] = useState(initial?.nickname || '');
  const [email, setEmail] = useState(initial?.email || '');
  const [githubUrl, setGithubUrl] = useState(initial?.githubUrl || '');
  const [error, setError] = useState('');

  const submit = () => {
    const next: CommentIdentity = {
      nickname: nickname.trim(),
      email: email.trim(),
      githubUrl: githubUrl.trim(),
    };
    if (!next.nickname) {
      setError('请填写昵称');
      return;
    }
    if (!next.email && !next.githubUrl) {
      setError('邮箱或 GitHub 主页需二选一');
      return;
    }
    if (next.email && !next.email.includes('@')) {
      setError('邮箱格式不正确');
      return;
    }
    if (next.githubUrl) {
      try {
        const u = new URL(next.githubUrl);
        if (!['github.com', 'www.github.com'].includes(u.hostname.toLowerCase())) {
          setError('GitHub 链接无效');
          return;
        }
      } catch {
        setError('GitHub 链接无效');
        return;
      }
    }
    setError('');
    onSubmit(next);
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
        首次评论需要填写身份信息，之后会保存在本机 Cookie，无需重复输入。
      </Typography.Paragraph>
      <Form layout="vertical">
        <Form.Slot label="昵称">
          <Input value={nickname} onChange={setNickname} placeholder="显示名称" />
        </Form.Slot>
        <Form.Slot label="邮箱（与 GitHub 二选一）">
          <Input value={email} onChange={setEmail} placeholder="name@example.com" />
        </Form.Slot>
        <Form.Slot label="GitHub 主页（与邮箱二选一）">
          <Input value={githubUrl} onChange={setGithubUrl} placeholder="https://github.com/username" />
        </Form.Slot>
      </Form>
      {error ? <Typography.Text type="danger">{error}</Typography.Text> : null}
    </Modal>
  );
}
