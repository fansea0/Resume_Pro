import { useCallback, useEffect, useId, useRef, useState, type ChangeEvent, type ReactNode } from 'react';
import { getTargetJob } from '../api/targetJobs';
import { uploadResumeAvatar } from '../api/uploads';
import type {
  ExperienceRewriteContent,
  ProjectRewriteContent,
  SummaryRewriteContent,
} from '../types/ai';
import type {
  CampusItem,
  EducationItem,
  ExperienceItem,
  HonourItem,
  ProjectItem,
  ResumeDocument,
  ResumeProfile,
  ResumeSection,
  ResumeSectionType,
  SkillItem,
} from '../types/resume';
import type { TargetJob } from '../types/targetJob';
import {
  createCampusSection,
  createEducationSection,
  createHonoursSection,
  createInternshipSection,
  createProjectSection,
  createSkillsSection,
  createSummarySection,
  createWorkSection,
} from '../types/resume';
import { AiSuggestionPanel } from './AiSuggestionPanel';
import { TargetJobCard } from './TargetJobCard';

interface ResumeEditorProps {
  document: ResumeDocument;
  resumeId?: number;
  onBeforeAiRewrite?: (snapshot: ResumeDocument) => Promise<boolean>;
  onChange: (document: ResumeDocument) => void;
}

function nextId() {
  return crypto.randomUUID();
}

interface FieldVisibility {
  shown: boolean;
  onToggle: () => void;
}

function EyeButton({ label, shown, onToggle }: FieldVisibility & { label: string }) {
  const action = shown ? '隐藏' : '显示';
  return <button aria-label={`${action}${label}`} className="editor-field__visibility" onClick={onToggle} title={`${action}${label}`} type="button">
    <svg aria-hidden="true" viewBox="0 0 24 24">{shown ? <><path d="M2.5 12s3.4-5.5 9.5-5.5S21.5 12 21.5 12s-3.4 5.5-9.5 5.5S2.5 12 2.5 12Z" /><circle cx="12" cy="12" r="2.6" /></> : <><path d="m3 3 18 18" /><path d="M10.2 6.8A10.8 10.8 0 0 1 12 6.5c6.1 0 9.5 5.5 9.5 5.5a18.1 18.1 0 0 1-3.2 3.6M6.1 9A18.1 18.1 0 0 0 2.5 12s3.4 5.5 9.5 5.5c.9 0 1.7-.1 2.5-.4" /><path d="M9.8 9.8a3.1 3.1 0 0 0 4.4 4.4" /></> }</svg>
  </button>;
}

type IconName = 'add' | 'delete' | 'eye' | 'eyeOff' | 'moveDown' | 'moveUp';

function Icon({ name }: { name: IconName }) {
  const paths: Record<IconName, ReactNode> = {
    add: <><path d="M12 5v14M5 12h14" /></>,
    delete: <><path d="M5 7h14M10 11v5M14 11v5M8 7l1-2h6l1 2M7 7l.8 12h8.4L17 7" /></>,
    eye: <><path d="M2.5 12s3.4-5.5 9.5-5.5S21.5 12 21.5 12s-3.4 5.5-9.5 5.5S2.5 12 2.5 12Z" /><circle cx="12" cy="12" r="2.6" /></>,
    eyeOff: <><path d="m3 3 18 18" /><path d="M10.2 6.8A10.8 10.8 0 0 1 12 6.5c6.1 0 9.5 5.5 9.5 5.5a18.1 18.1 0 0 1-3.2 3.6M6.1 9A18.1 18.1 0 0 0 2.5 12s3.4 5.5 9.5 5.5c.9 0 1.7-.1 2.5-.4" /><path d="M9.8 9.8a3.1 3.1 0 0 0 4.4 4.4" /></>,
    moveDown: <><path d="M12 4v15M7 14l5 5 5-5" /></>,
    moveUp: <><path d="M12 20V5M7 10l5-5 5 5" /></>,
  };
  return <svg aria-hidden="true" viewBox="0 0 24 24">{paths[name]}</svg>;
}

function IconButton({ icon, label, ...props }: Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'children'> & { icon: IconName; label: string }) {
  return <button aria-label={label} className="editor-icon-button" title={label} type="button" {...props}><Icon name={icon} /></button>;
}

function AddSectionButton({ label, onClick }: { label: string; onClick: () => void }) {
  return <button aria-label={`添加${label}区块`} className="editor-add-section" onClick={onClick} title={`添加${label}区块`} type="button"><Icon name="add" /><span>{label}</span></button>;
}

function ResumePhotoField({ profile, resumeId, onChange }: { profile: ResumeProfile; resumeId?: number; onChange: (profile: ResumeProfile) => void }) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [isUploading, setIsUploading] = useState(false);
  const [error, setError] = useState('');
  const initial = profile.fullName.trim().slice(0, 1) || '简';

  const handleFile = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    if (!['image/jpeg', 'image/jpg', 'image/png', 'image/webp'].includes(file.type)) {
      setError('请选择 JPG、PNG 或 WebP 格式的证件照。');
      return;
    }
    setError('');
    setIsUploading(true);
    const avatarStorageKey = profile.avatarStorageKey || nextId();
    try {
      const { url } = await uploadResumeAvatar(file, avatarStorageKey, resumeId);
      onChange({ ...profile, avatarStorageKey, avatarUrl: url });
    } catch (uploadError) {
      setError(uploadError instanceof Error ? uploadError.message : '头像上传失败，请稍后重试。');
    } finally {
      setIsUploading(false);
    }
  };

  return <div className="editor-photo-field">
    <div className="editor-photo-field__preview" aria-label="简历证件照预览">
      {profile.avatarUrl ? <img alt="简历证件照预览" src={profile.avatarUrl} /> : <span aria-hidden="true">{initial}</span>}
    </div>
    <div className="editor-photo-field__content"><div><span className="editor-photo-field__label">简历证件照</span><p>建议使用清晰正面照，按 25 × 35 比例裁切展示。</p></div><button className="editor-photo-field__trigger" disabled={isUploading} onClick={() => inputRef.current?.click()} type="button">{isUploading ? '上传中…' : profile.avatarUrl ? '更换照片' : '上传照片'}</button>{profile.avatarUrl ? <IconButton icon={profile.showAvatar ? 'eyeOff' : 'eye'} label={profile.showAvatar ? '隐藏简历照片' : '显示简历照片'} onClick={() => onChange({ ...profile, showAvatar: !profile.showAvatar })} /> : null}<input accept=".jpg,.jpeg,.png,.webp,image/jpeg,image/jpg,image/png,image/webp" aria-label="上传简历证件照" className="editor-photo-field__input" onChange={handleFile} ref={inputRef} type="file" />{error ? <p className="editor-photo-field__error" role="alert">{error}</p> : null}</div>
  </div>;
}

function Field({ label, value, onChange, multiline = false, inputId, placeholder, helperText, visibility, actions }: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  multiline?: boolean;
  inputId?: string;
  placeholder?: string;
  helperText?: string;
  visibility?: FieldVisibility;
  actions?: ReactNode;
}) {
  const generatedId = useId();
  const id = inputId ?? `field-${label.replace(/\s/g, '-')}-${generatedId}`;
  const hintId = `${id}-hint`;
  return (
    <div className="editor-field">
      <div className="editor-field__label"><label htmlFor={id}>{label}</label>{visibility || actions ? <span className="editor-field__actions">{visibility ? <EyeButton label={label} {...visibility} /> : null}{actions}</span> : null}</div>
      {multiline ? <textarea id={id} value={value} onChange={(event) => onChange(event.target.value)} /> : <input aria-describedby={helperText ? hintId : undefined} id={id} placeholder={placeholder} value={value} onChange={(event) => onChange(event.target.value)} />}
      {helperText ? <span className="editor-field__hint" id={hintId}>{helperText}</span> : null}
    </div>
  );
}

function DetailsSection({ title, open = false, children }: { title: string; open?: boolean; children: ReactNode }) {
  return <details className="editor-section" open={open}><summary><span className="editor-section__title">{title}</span></summary><div className="editor-section__content">{children}</div></details>;
}

const sectionFactories: Record<ResumeSectionType, (id: string) => ResumeSection> = {
  summary: createSummarySection,
  work: createWorkSection,
  internship: createInternshipSection,
  project: createProjectSection,
  education: createEducationSection,
  skills: createSkillsSection,
  campus: createCampusSection,
  honours: createHonoursSection,
};

const sectionLabels: Record<ResumeSectionType, string> = {
  summary: '个人总结', work: '工作经历', internship: '实习经历', project: '项目经历',
  education: '教育经历', skills: '专业技能', campus: '校园经历', honours: '荣誉奖项',
};

type RewriteableResumeSection = Extract<ResumeSection, { type: 'summary' | 'work' | 'internship' | 'project' }>;
type RewriteContent = SummaryRewriteContent | ExperienceRewriteContent | ProjectRewriteContent;
type TargetJobDetailState =
  | { status: 'unbound' }
  | { status: 'loading' | 'error'; id: number }
  | { status: 'ready'; id: number; targetJob: TargetJob };

interface ActiveAiSection {
  id: string;
  index: number;
  section: RewriteableResumeSection;
  type: RewriteableResumeSection['type'];
}

function isRewriteableSection(section: ResumeSection): section is RewriteableResumeSection {
  return section.type === 'summary'
    || section.type === 'work'
    || section.type === 'internship'
    || section.type === 'project';
}

function hasRewriteableContent(section: RewriteableResumeSection) {
  return section.type === 'summary'
    ? section.content.markdown.trim() !== ''
    : section.content.items.length > 0;
}

function mergeExperienceRewrite(
  section: Extract<RewriteableResumeSection, { type: 'work' | 'internship' }>,
  rewrite: ExperienceRewriteContent,
): ResumeSection {
  const rewriteById = new Map(rewrite.items.map((item) => [item.id, item]));
  return {
    ...section,
    content: {
      items: section.content.items.map((item) => {
        const rewritten = rewriteById.get(item.id);
        if (!rewritten) return item;
        return {
          ...item,
          ...(rewritten.achievements === undefined ? {} : { achievements: rewritten.achievements }),
          ...(rewritten.highlightsMarkdown === undefined ? {} : { highlightsMarkdown: rewritten.highlightsMarkdown }),
        };
      }),
    },
  };
}

function mergeProjectRewrite(
  section: Extract<RewriteableResumeSection, { type: 'project' }>,
  rewrite: ProjectRewriteContent,
): ResumeSection {
  const rewriteById = new Map(rewrite.items.map((item) => [item.id, item]));
  return {
    ...section,
    content: {
      items: section.content.items.map((item) => {
        const rewritten = rewriteById.get(item.id);
        if (!rewritten) return item;
        return {
          ...item,
          ...(rewritten.introduction === undefined ? {} : { introduction: rewritten.introduction }),
          ...(rewritten.achievements === undefined ? {} : { achievements: rewritten.achievements }),
          ...(rewritten.highlightsMarkdown === undefined ? {} : { highlightsMarkdown: rewritten.highlightsMarkdown }),
        };
      }),
    },
  };
}

function mergeRewrite(section: RewriteableResumeSection, rewrite: RewriteContent): ResumeSection {
  if (section.type === 'summary') {
    return { ...section, content: { markdown: (rewrite as SummaryRewriteContent).markdown } };
  }
  if (section.type === 'project') {
    return mergeProjectRewrite(section, rewrite as ProjectRewriteContent);
  }
  return mergeExperienceRewrite(section, rewrite as ExperienceRewriteContent);
}

export function ResumeEditor({ document, resumeId, onBeforeAiRewrite, onChange }: ResumeEditorProps) {
  const [targetJobState, setTargetJobState] = useState<TargetJobDetailState>(
    document.targetJobId === undefined ? { status: 'unbound' } : { status: 'loading', id: document.targetJobId },
  );
  const [activeAiSection, setActiveAiSection] = useState<ActiveAiSection>();
  const [preparingAi, setPreparingAi] = useState(false);
  const [aiPreparationError, setAiPreparationError] = useState('');
  const targetJobRequestVersion = useRef(0);
  const aiPreparationVersion = useRef(0);
  const documentRef = useRef(document);
  documentRef.current = document;

  const loadTargetJob = useCallback((id: number) => {
    const requestVersion = ++targetJobRequestVersion.current;
    setTargetJobState({ status: 'loading', id });
    void getTargetJob(id).then((targetJob) => {
      if (requestVersion === targetJobRequestVersion.current) {
        setTargetJobState({ status: 'ready', id, targetJob });
      }
    }).catch(() => {
      if (requestVersion === targetJobRequestVersion.current) {
        setTargetJobState({ status: 'error', id });
      }
    });
  }, []);

  useEffect(() => {
    if (document.targetJobId === undefined) {
      targetJobRequestVersion.current += 1;
      setTargetJobState({ status: 'unbound' });
      return;
    }
    loadTargetJob(document.targetJobId);
  }, [document.targetJobId, loadTargetJob]);

  const updateProfile = (field: keyof ResumeProfile, value: string) => onChange({ ...document, profile: { ...document.profile, [field]: value } });
  const updateSection = (id: string, update: (section: ResumeSection) => ResumeSection) => onChange({ ...document, sections: document.sections.map((section) => section.id === id ? update(section) : section) });
  const addSection = (type: ResumeSectionType) => onChange({ ...document, sections: [...document.sections, sectionFactories[type](nextId())] });
  const moveSection = (index: number, offset: number) => {
    const target = index + offset;
    if (target < 0 || target >= document.sections.length) return;
    const sections = [...document.sections];
    [sections[index], sections[target]] = [sections[target], sections[index]];
    onChange({ ...document, sections });
  };
  const removeSection = (id: string) => onChange({ ...document, sections: document.sections.filter((section) => section.id !== id) });
  const prepareAiRewrite = async (section: RewriteableResumeSection, index: number) => {
    if (preparingAi || !onBeforeAiRewrite) return;
    if (document.sections.filter((candidate) => candidate.id === section.id).length !== 1) {
      setAiPreparationError('区块标识重复，无法安全应用 AI 建议。请删除重复区块后重试。');
      return;
    }
    const snapshot = document;
    const requestVersion = ++aiPreparationVersion.current;
    setAiPreparationError('');
    setPreparingAi(true);
    let saved = false;
    try {
      saved = await onBeforeAiRewrite(snapshot);
    } catch {
      saved = false;
    }
    if (requestVersion !== aiPreparationVersion.current) return;
    setPreparingAi(false);
    if (!saved) {
      setAiPreparationError('保存最新内容后才能使用 AI 润色，请重试。');
      return;
    }
    if (documentRef.current !== snapshot) {
      setAiPreparationError('保存期间简历内容已变化，请重新点击 AI 润色。');
      return;
    }
    setActiveAiSection({ id: section.id, index, section, type: section.type });
  };
  const acceptRewrite = (active: ActiveAiSection, rewrite: RewriteContent) => {
    const matchingIndexes = document.sections.reduce<number[]>((indexes, section, index) => (
      section.id === active.id ? [...indexes, index] : indexes
    ), []);
    const current = document.sections[active.index];
    if (
      matchingIndexes.length !== 1
      || matchingIndexes[0] !== active.index
      || !current
      || current.type !== active.type
      || !isRewriteableSection(current)
    ) {
      setActiveAiSection(undefined);
      setAiPreparationError('当前区块已变化，未应用 AI 建议。请重新润色。');
      return;
    }
    const sections = [...document.sections];
    sections[active.index] = mergeRewrite(current, rewrite);
    onChange({ ...document, sections });
    setActiveAiSection(undefined);
  };

  const targetJob = targetJobState.status === 'ready'
    && targetJobState.id === document.targetJobId
    ? targetJobState.targetJob
    : undefined;
  const targetJobStatus = document.targetJobId === undefined
    ? undefined
    : targetJobState.status !== 'unbound' && targetJobState.id === document.targetJobId
      ? targetJobState.status
      : 'loading';

  return (
    <form
      {...(preparingAi ? { 'aria-busy': true, inert: '' } : {})}
      className="resume-editor"
      onSubmit={(event) => event.preventDefault()}
    >
      <DetailsSection open title="基本信息">
        <ResumePhotoField profile={document.profile} resumeId={resumeId} onChange={(profile) => onChange({ ...document, profile })} />
        <div className="editor-fields editor-fields--two">
          <Field label="姓名" value={document.profile.fullName} onChange={(value) => updateProfile('fullName', value)} />
          <Field label="求职方向" value={document.profile.headline} onChange={(value) => updateProfile('headline', value)} />
          <Field label="邮箱" value={document.profile.email} onChange={(value) => updateProfile('email', value)} />
          <Field label="电话" value={document.profile.phone} onChange={(value) => updateProfile('phone', value)} />
          <Field label="所在地" value={document.profile.location} onChange={(value) => updateProfile('location', value)} />
          <Field label="个人网站" value={document.profile.website} onChange={(value) => updateProfile('website', value)} />
        </div>
      </DetailsSection>

      <TargetJobCard
        onChange={(id) => onChange({ ...document, targetJobId: id ?? undefined })}
        onRetry={document.targetJobId === undefined ? undefined : () => loadTargetJob(document.targetJobId!)}
        targetJob={targetJob}
        targetJobId={document.targetJobId}
        targetJobStatus={targetJobStatus}
      />

      {preparingAi ? <p aria-live="polite" className="resume-library__status" role="status">正在保存最新简历内容…</p> : null}
      {aiPreparationError ? <p className="resume-library__error" role="alert">{aiPreparationError}</p> : null}

      {document.sections.map((section, index) => <SectionCard
        index={index}
        key={`${section.id}-${index}`}
        sectionCount={document.sections.length}
        section={section}
        onMove={moveSection}
        onPolish={isRewriteableSection(section) ? () => void prepareAiRewrite(section, index) : undefined}
        onRemove={removeSection}
        onUpdate={updateSection}
        polishDisabled={isRewriteableSection(section) && (
          preparingAi || !onBeforeAiRewrite || !resumeId || !hasRewriteableContent(section)
        )}
      />)}

      <DetailsSection title="添加区块">
        <div className="editor-add-section-grid">{(Object.keys(sectionLabels) as ResumeSectionType[]).map((type) => <AddSectionButton key={type} label={sectionLabels[type]} onClick={() => addSection(type)} />)}</div>
      </DetailsSection>

      {resumeId && activeAiSection?.section.type === 'summary' ? <AiSuggestionPanel
        onAccept={(rewrite) => acceptRewrite(activeAiSection, rewrite)}
        onClose={() => setActiveAiSection(undefined)}
        originalSection={activeAiSection.section}
        resumeId={resumeId}
        sectionType="summary"
      /> : null}
      {resumeId && activeAiSection?.section.type === 'work' ? <AiSuggestionPanel
        onAccept={(rewrite) => acceptRewrite(activeAiSection, rewrite)}
        onClose={() => setActiveAiSection(undefined)}
        originalSection={activeAiSection.section}
        resumeId={resumeId}
        sectionType="work"
      /> : null}
      {resumeId && activeAiSection?.section.type === 'internship' ? <AiSuggestionPanel
        onAccept={(rewrite) => acceptRewrite(activeAiSection, rewrite)}
        onClose={() => setActiveAiSection(undefined)}
        originalSection={activeAiSection.section}
        resumeId={resumeId}
        sectionType="internship"
      /> : null}
      {resumeId && activeAiSection?.section.type === 'project' ? <AiSuggestionPanel
        onAccept={(rewrite) => acceptRewrite(activeAiSection, rewrite)}
        onClose={() => setActiveAiSection(undefined)}
        originalSection={activeAiSection.section}
        resumeId={resumeId}
        sectionType="project"
      /> : null}
    </form>
  );
}

function SectionCard({ section, index, sectionCount, onUpdate, onMove, onPolish, onRemove, polishDisabled }: {
  section: ResumeSection;
  index: number;
  sectionCount: number;
  onUpdate: (id: string, update: (section: ResumeSection) => ResumeSection) => void;
  onMove: (index: number, offset: number) => void;
  onPolish?: () => void;
  onRemove: (id: string) => void;
  polishDisabled?: boolean;
}) {
  const update = (updateSection: (section: ResumeSection) => ResumeSection) => onUpdate(section.id, updateSection);
  const label = section.title;
  return <DetailsSection open title={label}>
    <div className="editor-card__heading">
      <Field label={`区块标题 ${index + 1}`} value={section.title} onChange={(title) => update((current) => ({ ...current, title }))} />
      <div className="editor-section__actions">
        {onPolish ? <button aria-label={`AI 润色 ${label}`} disabled={polishDisabled} onClick={onPolish} title={`AI 润色 ${label}`} type="button"><span aria-hidden="true">✦</span> AI 润色</button> : null}
        <IconButton disabled={index === 0} icon="moveUp" label={`上移 ${label}`} onClick={() => onMove(index, -1)} />
        <IconButton disabled={index === sectionCount - 1} icon="moveDown" label={`下移 ${label}`} onClick={() => onMove(index, 1)} />
        <IconButton icon={section.visible ? 'eyeOff' : 'eye'} label={`${section.visible ? '隐藏' : '显示'} ${label}`} onClick={() => update((current) => ({ ...current, visible: !current.visible }))} />
        <IconButton icon="delete" label={`删除区块 ${label}`} onClick={() => onRemove(section.id)} />
      </div>
    </div>
    {section.type === 'summary' ? <Field label="个人总结 Markdown" multiline value={section.content.markdown} onChange={(markdown) => update((current) => ({ ...current, content: { markdown } } as ResumeSection))} /> : null}
    {(section.type === 'work' || section.type === 'internship') ? <ExperienceItemsEditor section={section} update={update} /> : null}
    {section.type === 'project' ? <ProjectItemsEditor section={section} update={update} /> : null}
    {section.type === 'education' ? <EducationItemsEditor section={section} update={update} /> : null}
    {section.type === 'skills' ? <SkillsItemsEditor section={section} update={update} /> : null}
    {section.type === 'campus' ? <CampusItemsEditor section={section} update={update} /> : null}
    {section.type === 'honours' ? <HonoursItemsEditor section={section} update={update} /> : null}
  </DetailsSection>;
}

function updateItems<T extends { id: string }>(items: T[], id: string, change: (item: T) => T) { return items.map((item) => item.id === id ? change(item) : item); }

function ExperienceItemsEditor({ section, update }: { section: Extract<ResumeSection, { type: 'work' | 'internship' }>; update: (fn: (s: ResumeSection) => ResumeSection) => void }) {
  const itemLabel = section.type === 'work' ? '工作' : '实习';
  const change = (id: string, fn: (item: ExperienceItem) => ExperienceItem) => update((current) => ({ ...section, content: { items: updateItems(section.content.items, id, fn) } }));
  const add = () => update((current) => ({ ...section, content: { items: [...section.content.items, { id: nextId(), company: '', role: '', startDate: '', endDate: '', location: '', achievements: '', highlightsMarkdown: '', showLocation: true, showAchievements: true }] } }));
  return <><div className="editor-section__actions"><IconButton icon="add" label={`添加${itemLabel}条目`} onClick={add} /></div>{section.content.items.map((item, index) => <div className="editor-card" key={item.id}>
    <div className="editor-fields editor-fields--two">
      <Field label={`公司 ${index + 1}`} value={item.company} onChange={(company) => change(item.id, (value) => ({ ...value, company }))} />
      <Field label={`职位 ${index + 1}`} value={item.role} onChange={(role) => change(item.id, (value) => ({ ...value, role }))} />
      <Field label={`开始时间 ${index + 1}`} value={item.startDate} onChange={(startDate) => change(item.id, (value) => ({ ...value, startDate }))} />
      <Field label={`结束时间 ${index + 1}`} value={item.endDate} onChange={(endDate) => change(item.id, (value) => ({ ...value, endDate }))} />
      <Field label={`地点 ${index + 1}`} placeholder="例如：武汉 · 光谷" value={item.location} visibility={{ shown: item.showLocation, onToggle: () => change(item.id, (value) => ({ ...value, showLocation: !value.showLocation })) }} onChange={(location) => change(item.id, (value) => ({ ...value, location }))} />
    </div>
    <Field label={`业绩贡献 ${index + 1}`} multiline value={item.achievements} visibility={{ shown: item.showAchievements, onToggle: () => change(item.id, (value) => ({ ...value, showAchievements: !value.showAchievements })) }} onChange={(achievements) => change(item.id, (value) => ({ ...value, achievements }))} />
    <Field label={`列表说明 ${index + 1}`} multiline value={item.highlightsMarkdown} onChange={(highlightsMarkdown) => change(item.id, (value) => ({ ...value, highlightsMarkdown }))} />
    <IconButton icon="delete" label={`删除${itemLabel}条目 ${index + 1}`} onClick={() => update((current) => ({ ...section, content: { items: section.content.items.filter((value) => value.id !== item.id) } }))} />
  </div>)}</>;
}

function ProjectItemsEditor({ section, update }: { section: Extract<ResumeSection, { type: 'project' }>; update: (fn: (s: ResumeSection) => ResumeSection) => void }) {
  const change = (id: string, fn: (item: ProjectItem) => ProjectItem) => update(() => ({ ...section, content: { items: updateItems(section.content.items, id, fn) } }));
  const add = () => update(() => ({ ...section, content: { items: [...section.content.items, { id: nextId(), name: '', role: '', startDate: '', endDate: '', location: '', link: '', introduction: '', achievements: '', highlightsMarkdown: '', showLocation: true, showLink: true, showIntroduction: true, showAchievements: true }] } }));
  return <><div className="editor-section__actions"><IconButton icon="add" label="添加项目条目" onClick={add} /></div>{section.content.items.map((item, index) => <div className="editor-card" key={item.id}>
    <div className="editor-fields editor-fields--two">
      <Field label={`项目名称 ${index + 1}`} value={item.name} onChange={(name) => change(item.id, (value) => ({ ...value, name }))} />
      <Field label={`项目角色 ${index + 1}`} value={item.role} onChange={(role) => change(item.id, (value) => ({ ...value, role }))} />
      <Field label={`开始时间 ${index + 1}`} value={item.startDate} onChange={(startDate) => change(item.id, (value) => ({ ...value, startDate }))} />
      <Field label={`结束时间 ${index + 1}`} value={item.endDate} onChange={(endDate) => change(item.id, (value) => ({ ...value, endDate }))} />
      <Field label={`地点 ${index + 1}`} value={item.location} visibility={{ shown: item.showLocation, onToggle: () => change(item.id, (value) => ({ ...value, showLocation: !value.showLocation })) }} onChange={(location) => change(item.id, (value) => ({ ...value, location }))} />
      <Field label={`项目链接 ${index + 1}`} value={item.link} visibility={{ shown: item.showLink, onToggle: () => change(item.id, (value) => ({ ...value, showLink: !value.showLink })) }} onChange={(link) => change(item.id, (value) => ({ ...value, link }))} />
    </div>
    <Field label={`项目介绍 ${index + 1}`} multiline value={item.introduction} visibility={{ shown: item.showIntroduction, onToggle: () => change(item.id, (value) => ({ ...value, showIntroduction: !value.showIntroduction })) }} onChange={(introduction) => change(item.id, (value) => ({ ...value, introduction }))} />
    <Field label={`项目成绩 ${index + 1}`} multiline value={item.achievements} visibility={{ shown: item.showAchievements, onToggle: () => change(item.id, (value) => ({ ...value, showAchievements: !value.showAchievements })) }} onChange={(achievements) => change(item.id, (value) => ({ ...value, achievements }))} />
    <Field label={`列表说明 ${index + 1}`} multiline value={item.highlightsMarkdown} onChange={(highlightsMarkdown) => change(item.id, (value) => ({ ...value, highlightsMarkdown }))} />
    <IconButton icon="delete" label={`删除项目条目 ${index + 1}`} onClick={() => update((current) => {
      const currentSection = current as typeof section;
      return { ...currentSection, content: { items: currentSection.content.items.filter((value) => value.id !== item.id) } };
    })} />
  </div>)}</>;
}

function SimpleItemsEditor<T extends { id: string }>({ addLabel, items, addItem, removeItem, children, itemsClassName, cardClassName }: { addLabel: string; items: T[]; addItem: () => void; removeItem: (id: string) => void; children: (item: T, index: number) => ReactNode; itemsClassName?: string; cardClassName?: string }) {
  return <><div className="editor-section__actions"><IconButton icon="add" label={addLabel} onClick={addItem} /></div><div className={itemsClassName}>{items.map((item, index) => <div className={`editor-card${cardClassName ? ` ${cardClassName}` : ''}`} key={item.id}>{children(item, index)}<IconButton icon="delete" label={`删除条目 ${index + 1}`} onClick={() => removeItem(item.id)} /></div>)}</div></>;
}

function EducationItemsEditor({ section, update }: { section: Extract<ResumeSection, { type: 'education' }>; update: (fn: (s: ResumeSection) => ResumeSection) => void }) {
  const change = (id: string, fn: (item: EducationItem) => EducationItem) => update(() => ({ ...section, content: { items: updateItems(section.content.items, id, fn) } }));
  return <SimpleItemsEditor addLabel="添加教育条目" items={section.content.items} addItem={() => update(() => ({ ...section, content: { items: [...section.content.items, { id: nextId(), school: '', startDate: '', endDate: '', degree: '', college: '', major: '' }] } }))} removeItem={(id) => update(() => ({ ...section, content: { items: section.content.items.filter((item) => item.id !== id) } }))}>{(item, index) => <div className="editor-fields editor-fields--two"><Field label={`院校 ${index + 1}`} value={item.school} onChange={(school) => change(item.id, (value) => ({ ...value, school }))} /><Field label={`开始时间 ${index + 1}`} value={item.startDate} onChange={(startDate) => change(item.id, (value) => ({ ...value, startDate }))} /><Field label={`结束时间 ${index + 1}`} value={item.endDate} onChange={(endDate) => change(item.id, (value) => ({ ...value, endDate }))} /><Field label={`学历层次 ${index + 1}`} value={item.degree} onChange={(degree) => change(item.id, (value) => ({ ...value, degree }))} /><Field label={`学院 ${index + 1}`} value={item.college} onChange={(college) => change(item.id, (value) => ({ ...value, college }))} /><Field label={`专业 ${index + 1}`} value={item.major} onChange={(major) => change(item.id, (value) => ({ ...value, major }))} /></div>}</SimpleItemsEditor>;
}

function SkillsItemsEditor({ section, update }: { section: Extract<ResumeSection, { type: 'skills' }>; update: (fn: (s: ResumeSection) => ResumeSection) => void }) {
  const change = (id: string, fn: (item: SkillItem) => SkillItem) => update(() => ({ ...section, content: { items: updateItems(section.content.items, id, fn) } }));
  const remove = (id: string) => update(() => ({ ...section, content: { items: section.content.items.filter((item) => item.id !== id) } }));
  const add = () => update(() => ({ ...section, content: { items: [...section.content.items, { id: nextId(), name: '', note: '' }] } }));
  return <><div className="editor-section__actions"><IconButton icon="add" label="添加技能" onClick={add} /></div><div className="editor-item-grid editor-item-grid--skills">{section.content.items.map((item, index) => <div className="editor-card editor-card--skill" key={item.id}>
    <Field actions={<IconButton icon="delete" label={`删除技能 ${index + 1}`} onClick={() => remove(item.id)} />} label={`技能 ${index + 1}`} value={item.name} onChange={(name) => change(item.id, (value) => ({ ...value, name }))} />
    <Field label={`备注 ${index + 1}`} value={item.note} onChange={(note) => change(item.id, (value) => ({ ...value, note }))} />
  </div>)}</div></>;
}

function CampusItemsEditor({ section, update }: { section: Extract<ResumeSection, { type: 'campus' }>; update: (fn: (s: ResumeSection) => ResumeSection) => void }) {
  const change = (id: string, fn: (item: CampusItem) => CampusItem) => update(() => ({ ...section, content: { items: updateItems(section.content.items, id, fn) } }));
  return <SimpleItemsEditor addLabel="添加校园条目" items={section.content.items} addItem={() => update(() => ({ ...section, content: { items: [...section.content.items, { id: nextId(), name: '', role: '', highlightsMarkdown: '' }] } }))} removeItem={(id) => update(() => ({ ...section, content: { items: section.content.items.filter((item) => item.id !== id) } }))}>{(item, index) => <><Field label={`活动或部门 ${index + 1}`} value={item.name} onChange={(name) => change(item.id, (value) => ({ ...value, name }))} /><Field label={`担任角色 ${index + 1}`} value={item.role} onChange={(role) => change(item.id, (value) => ({ ...value, role }))} /><Field label={`列表说明 ${index + 1}`} multiline value={item.highlightsMarkdown} onChange={(highlightsMarkdown) => change(item.id, (value) => ({ ...value, highlightsMarkdown }))} /></>}</SimpleItemsEditor>;
}

function HonoursItemsEditor({ section, update }: { section: Extract<ResumeSection, { type: 'honours' }>; update: (fn: (s: ResumeSection) => ResumeSection) => void }) {
  const change = (id: string, fn: (item: HonourItem) => HonourItem) => update(() => ({ ...section, content: { items: updateItems(section.content.items, id, fn) } }));
  return <SimpleItemsEditor addLabel="添加荣誉奖项" items={section.content.items} addItem={() => update(() => ({ ...section, content: { items: [...section.content.items, { id: nextId(), name: '' }] } }))} removeItem={(id) => update(() => ({ ...section, content: { items: section.content.items.filter((item) => item.id !== id) } }))}>{(item, index) => <Field label={`奖项名称 ${index + 1}`} value={item.name} onChange={(name) => change(item.id, (value) => ({ ...value, name }))} />}</SimpleItemsEditor>;
}
