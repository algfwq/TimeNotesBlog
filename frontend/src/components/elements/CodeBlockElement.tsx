import { useMemo, type CSSProperties, type MouseEvent as ReactMouseEvent } from 'react';
import { Toast } from '@douyinfe/semi-ui';
import { IconCopy } from '@douyinfe/semi-icons';
import { copyTextToClipboard } from '../../lib/clipboard';
import { codeLanguageLabel, highlightCode, normalizeCodeLanguage } from '../../lib/codeHighlighting';
import type { NoteElement } from '../../types';

export function CodeBlockPreview({ element, selected = false, readOnly = false }: { element: NoteElement; selected?: boolean; readOnly?: boolean }) {
  const content = element.content ?? '';
  const language = normalizeCodeLanguage(element.style?.language);
  const highlighted = useMemo(() => highlightCode(content, language), [content, language]);
  return (
    <div
      className={`timenotes-code-block ${selected ? 'timenotes-code-block-selected' : ''} ${readOnly ? 'timenotes-code-block-readonly' : ''}`}
      style={codeBlockStyle(element.style ?? {})}
      onWheel={(event) => event.stopPropagation()}
      onPointerDown={readOnly ? (event) => event.stopPropagation() : undefined}
    >
      <CodeBlockHeader element={element} />
      <pre className="timenotes-code-pre timenotes-scrollbar">
        <code className={`language-${language}`} dangerouslySetInnerHTML={{ __html: highlighted }} />
      </pre>
    </div>
  );
}

function CodeBlockHeader({ element }: { element: NoteElement }) {
  const languageLabel = codeLanguageLabel(element.style?.language);
  const copyCode = async (event: ReactMouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
    const copied = await copyTextToClipboard(element.content ?? '');
    if (copied) {
      Toast.success('代码已复制');
    } else {
      Toast.error('复制失败');
    }
  };

  return (
    <div className="timenotes-code-header">
      <span className="timenotes-code-language">{languageLabel}</span>
      <button type="button" className="timenotes-code-copy" title="复制代码" onClick={copyCode} onMouseDown={(event) => event.stopPropagation()}>
        <IconCopy />
      </button>
    </div>
  );
}

function codeBlockStyle(style: Record<string, string | number | boolean>): CSSProperties {
  return {
    color: String(style.color ?? '#d7e2f0'),
    background: String(style.background ?? '#101828'),
    fontSize: Number(style.fontSize ?? 14),
    borderRadius: Number(style.borderRadius ?? 8),
  };
}
