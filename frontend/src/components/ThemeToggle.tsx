import { Button } from '@douyinfe/semi-ui';
import { IconMoon, IconSun } from '@douyinfe/semi-icons';
import { useTheme } from '../theme';

export function ThemeToggle({ size = 'default' }: { size?: 'default' | 'small' | 'large' }) {
  const { mode, toggle } = useTheme();
  return (
    <Button
      size={size}
      theme="borderless"
      type="tertiary"
      icon={mode === 'dark' ? <IconSun /> : <IconMoon />}
      onClick={toggle}
      aria-label={mode === 'dark' ? '切换到浅色' : '切换到深色'}
      title={mode === 'dark' ? '浅色模式' : '深色模式'}
    />
  );
}
