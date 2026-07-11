import { ReadOnlyViewer } from './ReadOnlyViewer';
import type { NoteDocument } from '../types';

// Blog 阅读器直接复用客户端只读双页书本实现。
export function ReaderView({ document }: { document: NoteDocument }) {
  return (
    <div className="reader-shell">
      <ReadOnlyViewer document={document} />
    </div>
  );
}
