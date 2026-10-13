import { useEffect, type RefObject } from 'react';
import {
  EDITOR_ITEM_ANCHOR_ATTR,
  EDITOR_SECTION_ANCHOR_ATTR,
  LOCATED_CLASS,
  type LocateRequest,
} from '../components/ResumeLocate';

export const HIGHLIGHT_DURATION_MS = 1600;

/**
 * 按属性取值查节点，而不是拼进 CSS 选择器：id 来自持久化简历，可能包含
 * 任意字符，拼选择器既有转义风险又有 CSS 注入面。
 */
function findByAttr(root: HTMLElement, attr: string, value: string): HTMLElement | undefined {
  return Array.from(root.querySelectorAll<HTMLElement>(`[${attr}]`))
    .find((node) => node.getAttribute(attr) === value);
}

/** jsdom 与部分旧环境没有 matchMedia；缺失时按"不减弱动效"处理。 */
function prefersReducedMotion(): boolean {
  return typeof window !== 'undefined'
    && typeof window.matchMedia === 'function'
    && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/**
 * 消费 app.tsx 路由过来的定位请求：在编辑器自己的 DOM 内查找目标，
 * 展开折叠的 <details>，滚动并短暂高亮。目标不存在时静默返回。
 */
export function useLocateTarget(request: LocateRequest | undefined, rootRef: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    if (!request) return;
    const root = rootRef.current;
    if (!root) return;

    const target = (request.itemId ? findByAttr(root, EDITOR_ITEM_ANCHOR_ATTR, request.itemId) : undefined)
      ?? findByAttr(root, EDITOR_SECTION_ANCHOR_ATTR, request.sectionId);
    if (!target) return;

    const details = target.closest('details');
    if (details && !details.open) details.open = true;

    if (typeof target.scrollIntoView === 'function') {
      target.scrollIntoView({ behavior: prefersReducedMotion() ? 'auto' : 'smooth', block: 'start' });
    }

    target.classList.add(LOCATED_CLASS);
    const timer = window.setTimeout(() => target.classList.remove(LOCATED_CLASS), HIGHLIGHT_DURATION_MS);
    return () => {
      window.clearTimeout(timer);
      target.classList.remove(LOCATED_CLASS);
    };
  }, [request, rootRef]);
}