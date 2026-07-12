import { useNavigate } from 'react-router-dom';
import { Button, Typography } from '@douyinfe/semi-ui';
import { IconBook } from '@douyinfe/semi-icons';
import { ThemeToggle } from './ThemeToggle';

export function PublicNav({
  compact = false,
  brandTitle = 'TimeNotes Blog',
}: {
  compact?: boolean;
  brandTitle?: string;
}) {
  const navigate = useNavigate();
  const title = brandTitle.trim() || 'TimeNotes Blog';
  return (
    <header className={`public-nav ${compact ? 'public-nav--compact' : ''}`}>
      <button type="button" className="public-nav-brand" onClick={() => navigate('/')}>
        <IconBook size="large" />
        <Typography.Text strong>{title}</Typography.Text>
      </button>
      <div className="public-nav-actions">
        <ThemeToggle />
        {!compact ? (
          <Button theme="light" type="tertiary" onClick={() => {
            document.getElementById('notes-grid')?.scrollIntoView({ behavior: 'smooth' });
          }}>
            浏览手账
          </Button>
        ) : null}
      </div>
    </header>
  );
}
