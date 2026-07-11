import { useState } from 'react';
import { Form, Input, Modal, Typography } from '@douyinfe/semi-ui';

export function AdminCredentialMigrationModal({
  visible,
  loading,
  onSubmit,
}: {
  visible: boolean;
  loading?: boolean;
  onSubmit: (username: string, password: string) => Promise<void> | void;
}) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState('');

  const submit = async () => {
    const name = username.trim();
    if (!name || name.toLowerCase() === 'admin') {
      setError('请设置非默认用户名');
      return;
    }
    if (!password || password === '123456') {
      setError('请设置非默认密码');
      return;
    }
    if (password !== confirm) {
      setError('两次密码不一致');
      return;
    }
    setError('');
    await onSubmit(name, password);
  };

  return (
    <Modal
      title="首次登录：请修改默认管理员凭据"
      visible={visible}
      closable={false}
      maskClosable={false}
      closeOnEsc={false}
      hasCancel={false}
      okText="保存并继续"
      confirmLoading={loading}
      onOk={() => void submit()}
    >
      <Typography.Paragraph type="danger" style={{ marginTop: 0 }}>
        当前账号仍使用默认 admin/123456。为保障安全，必须修改用户名和密码后才能使用后台。
      </Typography.Paragraph>
      <Form layout="vertical">
        <Form.Slot label="新用户名">
          <Input value={username} onChange={setUsername} placeholder="不要使用 admin" />
        </Form.Slot>
        <Form.Slot label="新密码">
          <Input mode="password" value={password} onChange={setPassword} placeholder="不要使用 123456" />
        </Form.Slot>
        <Form.Slot label="确认密码">
          <Input mode="password" value={confirm} onChange={setConfirm} placeholder="再次输入密码" />
        </Form.Slot>
      </Form>
      {error ? <Typography.Text type="danger">{error}</Typography.Text> : null}
    </Modal>
  );
}
