import { Button } from '@douyinfe/semi-ui';
import { IconMoon, IconSun } from '@douyinfe/semi-icons';
import { useTheme } from '../theme';

export function ThemeToggle({ size = 'default' }: { size?: 'default' | 'small' | 'large' }) {
  const { mode, toggle } = useTheme();
  const isDark = mode === 'dark';
  return (
    <Button
      size={size}
      theme="borderless"
      type="tertiary"
      className="theme-toggle-btn"
      icon={
        <span className={`theme-toggle-icon ${isDark ? 'is-dark' : 'is-light'}`} key={mode}>
          {isDark ? <IconSun /> : <IconMoon />}
        </span>
      }
      onClick={toggle}
      aria-label={isDark ? '切换到浅色' : '切换到深色'}
      title={isDark ? '浅色模式' : '深色模式'}
    />
  );
}
