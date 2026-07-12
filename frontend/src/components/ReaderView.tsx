import { ReadOnlyViewer, type ReaderChrome } from './ReadOnlyViewer';
import type { NoteDocument } from '../types';

/** Blog 阅读器：客户端双页书本阅读体验 + 可选博客互动 chrome。 */
export function ReaderView({
  document,
  chrome,
}: {
  document: NoteDocument;
  chrome?: ReaderChrome;
}) {
  return (
    <div className="reader-shell">
      <ReadOnlyViewer document={document} chrome={chrome} />
    </div>
  );
}
