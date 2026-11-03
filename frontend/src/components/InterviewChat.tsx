import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react';

import {
  answerInterview,
  getInterviewQuestions,
  type Interview,
  type InterviewGrade,
  type InterviewLabel,
  type InterviewQuestion,
} from '../api/interviews';

const LABEL_NAMES: Record<InterviewLabel, string> = { '0': '八股', '1': '算法', '2': '项目' };
// 与 .interview-dock textarea 的 max-height 保持一致（12rem @16px）。
const DOCK_MAX_HEIGHT_PX = 192;
const GRADE_CLASS: Record<InterviewGrade, string> = {
  C: 'c', 'B-': 'bm', B: 'b', 'B+': 'bp', A: 'a', S: 's',
};

export interface InterviewChatFinishedResult {
  overallGrade: InterviewGrade;
  overallReview?: string | null;
}

interface InterviewChatProps {
  interview: Interview;
  onExit: () => void;
  onFinished: (result: InterviewChatFinishedResult) => void;
}

function isAwaiting(question: InterviewQuestion) {
  return question.grade == null && question.userAnswer == null;
}

export function InterviewChat({ interview, onExit, onFinished }: InterviewChatProps) {
  const [questions, setQuestions] = useState<InterviewQuestion[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [answer, setAnswer] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState('');
  const mounted = useRef(false);
  const requestVersion = useRef(0);
  const activeRef = useRef<HTMLElement | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);

  const load = useCallback(async () => {
    const version = ++requestVersion.current;
    try {
      const rows = await getInterviewQuestions(interview.id);
      if (mounted.current && version === requestVersion.current) setQuestions(rows);
    } catch {
      if (mounted.current && version === requestVersion.current) setLoadError(true);
    }
  }, [interview.id]);

  useEffect(() => {
    mounted.current = true;
    setLoading(true);
    void load().finally(() => {
      if (mounted.current) setLoading(false);
    });
    return () => {
      mounted.current = false;
      requestVersion.current += 1;
    };
  }, [load]);

  const mains = questions.filter((question) => !question.parentId);
  const followUpsByParent = new Map<number, InterviewQuestion[]>();
  for (const question of questions) {
    if (!question.parentId) continue;
    const siblings = followUpsByParent.get(question.parentId) ?? [];
    siblings.push(question);
    followUpsByParent.set(question.parentId, siblings);
  }
  // 面试图按顺序推进，因此「当前题」= 未回答的追问（刚被抛出），否则 = 最早一道未回答的主问题。
  // 全部主问题在建面试时就一次性落库，取「最后一个未回答」会永远停在末题。
  const awaiting = questions.filter(isAwaiting);
  const current = awaiting.find((question) => question.parentId) ?? awaiting[0];
  const currentMainId = current ? (current.parentId ?? current.id) : null;
  // 只渲染到当前题为止：后续题面提前摊开等于给候选人透题。
  const visibleThrough = currentMainId == null
    ? mains.length - 1
    : Math.max(0, mains.findIndex((main) => main.id === currentMainId));
  const answeredMains = mains.filter((question) => question.grade != null).length;
  const followUpUsed = questions.filter((question) => question.parentId).length;
  const budgetLeft = Math.max(0, interview.configSnapshot.followUpBudget - followUpUsed);
  const totalMains = Math.max(interview.configSnapshot.questionCount, mains.length);
  const currentNumber = currentMainId == null
    ? Math.min(answeredMains + 1, totalMains)
    : visibleThrough + 1;

  useEffect(() => {
    if (!current) return;
    activeRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    textareaRef.current?.focus({ preventScroll: true });
  }, [current?.id]);

  // 输入框随内容长高，到 DOCK_MAX_HEIGHT 后内部滚动：长答案不会把题面顶出屏幕。
  useEffect(() => {
    const area = textareaRef.current;
    if (!area) return;
    area.style.height = 'auto';
    area.style.height = `${Math.min(area.scrollHeight, DOCK_MAX_HEIGHT_PX)}px`;
  }, [answer]);

  const submit = async (skip = false) => {
    if (submitting) return;
    if (!skip && !answer.trim()) return;
    if (skip && !window.confirm('跳过当前题？将按最低档 C 结算本题，不消耗追问预算，直接进入下一题。')) return;
    setSubmitting(true);
    setSubmitError('');
    try {
      const result = skip
        ? await answerInterview(interview.id, undefined, true)
        : await answerInterview(interview.id, answer.trim());
      if (!mounted.current) return;
      if (result.finished && result.overallGrade) {
        onFinished({ overallGrade: result.overallGrade, overallReview: result.overallReview });
        return;
      }
      setAnswer('');
      await load();
    } catch {
      if (mounted.current) setSubmitError('评分失败，请重试。');
    } finally {
      if (mounted.current) setSubmitting(false);
    }
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      event.preventDefault();
      void submit();
    }
  };

  const renderQuestionBody = (question: InterviewQuestion) => (
    question.userAnswer ? (
      <div className="interview-bubble">
        <p className="interview-bubble__label">我的回答</p>
        <p className="interview-bubble__text">{question.userAnswer}</p>
      </div>
    ) : null
  );

  const renderTurn = (question: InterviewQuestion, isCurrent: boolean) => {
    const index = mains.findIndex((main) => main.id === question.id);
    const followUps = followUpsByParent.get(question.id) ?? [];
    return (
      <article
        className={`interview-msg interview-msg--${isCurrent ? 'current' : 'past'}`}
        key={question.id}
        ref={isCurrent ? activeRef : undefined}
      >
        <header className="interview-msg__head">
          <span aria-hidden="true" className="interview-msg__avatar">面</span>
          <strong>面试官</strong>
          <span className={`interview-chip interview-chip--${question.label === '0' ? 'octopus' : question.label === '1' ? 'algorithm' : 'project'}`}>{LABEL_NAMES[question.label]}</span>
          {index >= 0 ? <span className="interview-msg__no">第 {index + 1} / {totalMains} 题</span> : null}
          {question.id === current?.id ? (
            <span className="interview-msg__wait"><span aria-hidden="true" />{submitting ? '评分中…' : '等待回答'}</span>
          ) : question.grade ? (
            <span className={`interview-grade interview-grade--${GRADE_CLASS[question.grade]}`}>{question.grade}</span>
          ) : null}
        </header>
        <p className="interview-msg__question">{question.question}</p>
        {renderQuestionBody(question)}
        {followUps.map((followUp) => {
          const isCurrentFollowUp = current?.id === followUp.id;
          return (
            <div className={`interview-follow${isCurrentFollowUp ? ' interview-follow--current' : ''}`} key={followUp.id}>
              <header className="interview-msg__head">
                <span className="interview-follow__chip">追问 {followUp.followUpRound}</span>
                {isCurrentFollowUp ? (
                  <span className="interview-msg__wait"><span aria-hidden="true" />{submitting ? '评分中…' : '等待回答'}</span>
                ) : followUp.grade ? (
                  <span className={`interview-grade interview-grade--${GRADE_CLASS[followUp.grade]}`}>{followUp.grade}</span>
                ) : null}
              </header>
              <p className="interview-msg__question">{followUp.question}</p>
              {renderQuestionBody(followUp)}
            </div>
          );
        })}
      </article>
    );
  };

  return (
    <section aria-label="AI 模拟面试进行中" className="interview-chat">
      <header className="interview-progress">
        <div className="interview-live-bar">
          <div className="interview-progress__heading">
            <p className="interview-progress__eyebrow">当前题目</p>
            <p className="interview-progress__count">第 <b>{currentNumber}</b> / {totalMains} 题</p>
          </div>
          <span className="interview-stat">进度 <b>{answeredMains}/{interview.configSnapshot.questionCount}</b></span>
          <span className="interview-stat">追问预算 <b>{budgetLeft}/{interview.configSnapshot.followUpBudget}</b></span>
          <span className="interview-stat interview-stat--live"><span aria-hidden="true" />进行中</span>
          <button onClick={onExit} type="button">返回列表</button>
        </div>
        <ol aria-hidden="true" className="interview-progress__track">
          {Array.from({ length: totalMains }, (_, position) => {
            const main = mains[position];
            const grade = main?.grade;
            const state = grade ? GRADE_CLASS[grade] : main?.id === currentMainId ? 'current' : 'todo';
            return (
              <li className={`interview-progress__seg interview-progress__seg--${state}`} key={position}>
                {grade ?? position + 1}
              </li>
            );
          })}
        </ol>
      </header>

      {loading ? (
        <p aria-live="polite" className="interview-chat__status" role="status">正在载入面试进度…</p>
      ) : loadError ? (
        <p className="interview-chat__error" role="alert">无法载入面试进度，请稍后重试。</p>
      ) : (
        <div className="interview-chat__stream">
          {mains.slice(0, visibleThrough + 1).map((main) => renderTurn(main, main.id === currentMainId))}
        </div>
      )}

      {submitError && <p className="interview-chat__error" role="alert">{submitError}</p>}

      {current ? (
        <div aria-busy={submitting} className="interview-dock">
          <textarea
            aria-label="回答输入"
            onChange={(event) => setAnswer(event.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="输入你的回答…"
            readOnly={submitting}
            ref={textareaRef}
            value={answer}
          />
          <div className="interview-dock__foot">
            <span aria-live="polite" className="interview-dock__hint">
              {submitting ? (
                <>
                  <span aria-hidden="true" className="interview-typing__dots"><i /><i /><i /></span>
                  面试官正在评分，约需 3~8 秒…
                </>
              ) : '⌘ / Ctrl + Enter 提交；不会答可跳过'}
            </span>
            {answer.length ? <span className="interview-dock__count">{answer.length} 字</span> : null}
            <div className="interview-dock__actions">
              <button className="interview-dock__skip" disabled={submitting} onClick={() => void submit(true)} type="button">跳过 · 不会</button>
              <button disabled={submitting || !answer.trim()} onClick={() => void submit()} type="button">
                {submitting ? '评分中…' : '提交回答'}
              </button>
            </div>
          </div>
        </div>
      ) : (
        !loading && !loadError && (
          <p aria-live="polite" className="interview-chat__status" role="status">本轮已答完，正在生成总评…</p>
        )
      )}
    </section>
  );
}
