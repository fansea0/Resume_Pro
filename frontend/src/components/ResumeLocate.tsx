import type { ReactNode } from 'react';

/** 预览侧区块锚点。与既有 `data-resume-section={type}` 并存，不可互相替代。 */
export const SECTION_ANCHOR_ATTR = 'data-resume-section-id';
/** 预览侧条目锚点。语义独立于既有的 `data-resume-entry`（后者语义已不一致，勿动）。 */
export const ITEM_ANCHOR_ATTR = 'data-resume-item-id';
/** 编辑器侧区块锚点。 */
export const EDITOR_SECTION_ANCHOR_ATTR = 'data-editor-section-id';
/** 编辑器侧条目锚点。 */
export const EDITOR_ITEM_ANCHOR_ATTR = 'data-editor-item-id';
/** 定位命中时短暂施加的高亮 class。 */
export const LOCATED_CLASS = 'is-located';

export interface LocateTarget {
  sectionId: string;
  itemId?: string;
}

/** `app.tsx` 持有的完整请求；nonce 保证重复点击同一目标也能重放。 */
export interface LocateRequest extends LocateTarget {
  nonce: number;
}

export type LocateHandler = (target: LocateTarget) => void;

function locateLabel(label: string) {
  return `定位到编辑器的「${label}」`;
}

/**
 * 区块标题定位按钮。必须放在原 `<h2>` 内部：既保留标题语义（读屏仍按标题朗读），
 * 又获得原生按钮语义与键盘可达性。不要改成覆盖整个区块的透明遮罩——
 * 那会拦截预览正文的文字选中与项目链接点击。
 */
export function SectionLocateButton({ children, label, onLocate, sectionId }: {
  children: ReactNode;
  label: string;
  onLocate?: LocateHandler;
  sectionId: string;
}) {
  if (!onLocate) return <>{children}</>;
  return (
    <button aria-label={locateLabel(label)} className="resume-locate" onClick={() => onLocate({ sectionId })} title={locateLabel(label)} type="button">
      {children}
    </button>
  );
}

/** 条目标题定位按钮，同样包裹在原 `<h3>` 内部。 */
export function EntryLocateButton({ children, itemId, label, onLocate, sectionId }: {
  children: ReactNode;
  itemId: string;
  label: string;
  onLocate?: LocateHandler;
  sectionId: string;
}) {
  if (!onLocate) return <>{children}</>;
  return (
    <button aria-label={locateLabel(label)} className="resume-locate" onClick={() => onLocate({ sectionId, itemId })} title={locateLabel(label)} type="button">
      {children}
    </button>
  );
}
