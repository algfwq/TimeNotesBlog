import { Component, type ReactNode } from 'react';

type Props = {
  fallback?: ReactNode;
  children: ReactNode;
};

type State = {
  failed: boolean;
};

export class MediaErrorBoundary extends Component<Props, State> {
  state: State = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  render() {
    if (this.state.failed) {
      return (
        this.props.fallback ?? (
          <div className="grid h-full w-full place-items-center rounded-[8px] border border-dashed border-black/15 bg-white/70 px-3 text-center text-[11px] text-black/45">
            素材无法显示
          </div>
        )
      );
    }
    return this.props.children;
  }
}
