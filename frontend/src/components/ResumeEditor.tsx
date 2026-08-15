import { useId, type ReactNode } from 'react';
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

interface ResumeEditorProps {
  document: ResumeDocument;
  onChange: (document: ResumeDocument) => void;
}

function nextId() {
  return crypto.randomUUID();
}

function Field({ label, value, onChange, multiline = false, inputId, placeholder, helperText }: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  multiline?: boolean;
  inputId?: string;
  placeholder?: string;
  helperText?: string;
}) {
  const generatedId = useId();
  const id = inputId ?? `field-${label.replace(/\s/g, '-')}-${generatedId}`;
  const hintId = `${id}-hint`;
  return (
    <div className="editor-field">
      <label htmlFor={id}>{label}</label>
      {multiline ? <textarea id={id} value={value} onChange={(event) => onChange(event.target.value)} /> : <input aria-describedby={helperText ? hintId : undefined} id={id} placeholder={placeholder} value={value} onChange={(event) => onChange(event.target.value)} />}
      {helperText ? <span className="editor-field__hint" id={hintId}>{helperText}</span> : null}
    </div>
  );
}

function Toggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: () => void }) {
  return <label className="editor-toggle"><input checked={checked} onChange={onChange} type="checkbox" /><span aria-hidden="true" className="editor-toggle__control" /><span className="editor-toggle__label">{label}</span></label>;
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

export function ResumeEditor({ document, onChange }: ResumeEditorProps) {
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

  return (
    <form className="resume-editor" onSubmit={(event) => event.preventDefault()}>
      <DetailsSection open title="基本信息">
        <div className="editor-fields editor-fields--two">
          <Field label="姓名" value={document.profile.fullName} onChange={(value) => updateProfile('fullName', value)} />
          <Field label="求职方向" value={document.profile.headline} onChange={(value) => updateProfile('headline', value)} />
          <Field label="邮箱" value={document.profile.email} onChange={(value) => updateProfile('email', value)} />
          <Field label="电话" value={document.profile.phone} onChange={(value) => updateProfile('phone', value)} />
          <Field label="所在地" value={document.profile.location} onChange={(value) => updateProfile('location', value)} />
          <Field label="个人网站" value={document.profile.website} onChange={(value) => updateProfile('website', value)} />
        </div>
      </DetailsSection>

      <DetailsSection title="添加区块">
        <div className="editor-section__actions">{(Object.keys(sectionLabels) as ResumeSectionType[]).map((type) => <button key={type} onClick={() => addSection(type)} type="button">添加{sectionLabels[type]}区块</button>)}</div>
      </DetailsSection>

      {document.sections.map((section, index) => <SectionCard
        index={index}
        key={section.id}
        sectionCount={document.sections.length}
        section={section}
        onMove={moveSection}
        onRemove={removeSection}
        onUpdate={updateSection}
      />)}
    </form>
  );
}

function SectionCard({ section, index, sectionCount, onUpdate, onMove, onRemove }: {
  section: ResumeSection;
  index: number;
  sectionCount: number;
  onUpdate: (id: string, update: (section: ResumeSection) => ResumeSection) => void;
  onMove: (index: number, offset: number) => void;
  onRemove: (id: string) => void;
}) {
  const update = (updateSection: (section: ResumeSection) => ResumeSection) => onUpdate(section.id, updateSection);
  const label = section.title;
  return <DetailsSection open title={label}>
    <div className="editor-card__heading">
      <Field label={`区块标题 ${index + 1}`} value={section.title} onChange={(title) => update((current) => ({ ...current, title }))} />
      <div className="editor-section__actions">
        <button aria-label={`上移 ${label}`} disabled={index === 0} onClick={() => onMove(index, -1)} type="button">上移</button>
        <button aria-label={`下移 ${label}`} disabled={index === sectionCount - 1} onClick={() => onMove(index, 1)} type="button">下移</button>
        <button aria-label={`${section.visible ? '隐藏' : '显示'} ${label}`} onClick={() => update((current) => ({ ...current, visible: !current.visible }))} type="button">{section.visible ? '隐藏' : '显示'}</button>
        <button aria-label={`删除区块 ${label}`} onClick={() => onRemove(section.id)} type="button">删除</button>
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
  return <><div className="editor-section__actions"><button onClick={add} type="button">添加{itemLabel}条目</button></div>{section.content.items.map((item, index) => <div className="editor-card" key={item.id}>
    <div className="editor-fields editor-fields--two">
      <Field label={`公司 ${index + 1}`} value={item.company} onChange={(company) => change(item.id, (value) => ({ ...value, company }))} />
      <Field label={`职位 ${index + 1}`} value={item.role} onChange={(role) => change(item.id, (value) => ({ ...value, role }))} />
      <Field label={`开始时间 ${index + 1}`} value={item.startDate} onChange={(startDate) => change(item.id, (value) => ({ ...value, startDate }))} />
      <Field label={`结束时间 ${index + 1}`} value={item.endDate} onChange={(endDate) => change(item.id, (value) => ({ ...value, endDate }))} />
      <Field label={`地点 ${index + 1}`} helperText="填写城市、区域或办公地点；可在下方选择是否显示在简历中。" placeholder="例如：武汉 · 光谷" value={item.location} onChange={(location) => change(item.id, (value) => ({ ...value, location }))} />
      <Field label={`业绩贡献 ${index + 1}`} value={item.achievements} onChange={(achievements) => change(item.id, (value) => ({ ...value, achievements }))} />
    </div>
    <Field label={`列表说明 ${index + 1}`} multiline value={item.highlightsMarkdown} onChange={(highlightsMarkdown) => change(item.id, (value) => ({ ...value, highlightsMarkdown }))} />
    <fieldset className="editor-display-settings">
      <legend>展示设置 {index + 1}</legend>
      <p>控制这些补充信息是否出现在简历中。</p>
      <div className="editor-display-settings__options">
        <Toggle label={`显示地点 ${index + 1}`} checked={item.showLocation} onChange={() => change(item.id, (value) => ({ ...value, showLocation: !value.showLocation }))} />
        <Toggle label={`显示业绩贡献 ${index + 1}`} checked={item.showAchievements} onChange={() => change(item.id, (value) => ({ ...value, showAchievements: !value.showAchievements }))} />
      </div>
    </fieldset>
    <button aria-label={`删除${itemLabel}条目 ${index + 1}`} onClick={() => update((current) => ({ ...section, content: { items: section.content.items.filter((value) => value.id !== item.id) } }))} type="button">删除</button>
  </div>)}</>;
}

function ProjectItemsEditor({ section, update }: { section: Extract<ResumeSection, { type: 'project' }>; update: (fn: (s: ResumeSection) => ResumeSection) => void }) {
  const change = (id: string, fn: (item: ProjectItem) => ProjectItem) => update(() => ({ ...section, content: { items: updateItems(section.content.items, id, fn) } }));
  const add = () => update(() => ({ ...section, content: { items: [...section.content.items, { id: nextId(), name: '', role: '', startDate: '', endDate: '', location: '', link: '', introduction: '', achievements: '', highlightsMarkdown: '', showLocation: true, showLink: true, showIntroduction: true, showAchievements: true }] } }));
  return <><div className="editor-section__actions"><button onClick={add} type="button">添加项目条目</button></div>{section.content.items.map((item, index) => <div className="editor-card" key={item.id}>
    <div className="editor-fields editor-fields--two">
      <Field label={`项目名称 ${index + 1}`} value={item.name} onChange={(name) => change(item.id, (value) => ({ ...value, name }))} />
      <Field label={`项目角色 ${index + 1}`} value={item.role} onChange={(role) => change(item.id, (value) => ({ ...value, role }))} />
      <Field label={`开始时间 ${index + 1}`} value={item.startDate} onChange={(startDate) => change(item.id, (value) => ({ ...value, startDate }))} />
      <Field label={`结束时间 ${index + 1}`} value={item.endDate} onChange={(endDate) => change(item.id, (value) => ({ ...value, endDate }))} />
      <Field label={`地点 ${index + 1}`} value={item.location} onChange={(location) => change(item.id, (value) => ({ ...value, location }))} />
      <Field label={`项目链接 ${index + 1}`} value={item.link} onChange={(link) => change(item.id, (value) => ({ ...value, link }))} />
    </div>
    <Field label={`项目介绍 ${index + 1}`} multiline value={item.introduction} onChange={(introduction) => change(item.id, (value) => ({ ...value, introduction }))} />
    <Field label={`项目成绩 ${index + 1}`} value={item.achievements} onChange={(achievements) => change(item.id, (value) => ({ ...value, achievements }))} />
    <Field label={`列表说明 ${index + 1}`} multiline value={item.highlightsMarkdown} onChange={(highlightsMarkdown) => change(item.id, (value) => ({ ...value, highlightsMarkdown }))} />
    <Toggle label={`显示地点 ${index + 1}`} checked={item.showLocation} onChange={() => change(item.id, (value) => ({ ...value, showLocation: !value.showLocation }))} />
    <Toggle label={`显示项目链接 ${index + 1}`} checked={item.showLink} onChange={() => change(item.id, (value) => ({ ...value, showLink: !value.showLink }))} />
    <Toggle label={`显示项目介绍 ${index + 1}`} checked={item.showIntroduction} onChange={() => change(item.id, (value) => ({ ...value, showIntroduction: !value.showIntroduction }))} />
    <Toggle label={`显示项目成绩 ${index + 1}`} checked={item.showAchievements} onChange={() => change(item.id, (value) => ({ ...value, showAchievements: !value.showAchievements }))} />
    <button aria-label={`删除项目条目 ${index + 1}`} onClick={() => update((current) => {
      const currentSection = current as typeof section;
      return { ...currentSection, content: { items: currentSection.content.items.filter((value) => value.id !== item.id) } };
    })} type="button">删除</button>
  </div>)}</>;
}

function SimpleItemsEditor<T extends { id: string }>({ addLabel, items, addItem, removeItem, children }: { addLabel: string; items: T[]; addItem: () => void; removeItem: (id: string) => void; children: (item: T, index: number) => ReactNode }) {
  return <><div className="editor-section__actions"><button onClick={addItem} type="button">{addLabel}</button></div>{items.map((item, index) => <div className="editor-card" key={item.id}>{children(item, index)}<button aria-label={`删除条目 ${index + 1}`} onClick={() => removeItem(item.id)} type="button">删除</button></div>)}</>;
}

function EducationItemsEditor({ section, update }: { section: Extract<ResumeSection, { type: 'education' }>; update: (fn: (s: ResumeSection) => ResumeSection) => void }) {
  const change = (id: string, fn: (item: EducationItem) => EducationItem) => update(() => ({ ...section, content: { items: updateItems(section.content.items, id, fn) } }));
  return <SimpleItemsEditor addLabel="添加教育条目" items={section.content.items} addItem={() => update(() => ({ ...section, content: { items: [...section.content.items, { id: nextId(), school: '', startDate: '', endDate: '', degree: '', college: '', major: '' }] } }))} removeItem={(id) => update(() => ({ ...section, content: { items: section.content.items.filter((item) => item.id !== id) } }))}>{(item, index) => <div className="editor-fields editor-fields--two"><Field label={`院校 ${index + 1}`} value={item.school} onChange={(school) => change(item.id, (value) => ({ ...value, school }))} /><Field label={`开始时间 ${index + 1}`} value={item.startDate} onChange={(startDate) => change(item.id, (value) => ({ ...value, startDate }))} /><Field label={`结束时间 ${index + 1}`} value={item.endDate} onChange={(endDate) => change(item.id, (value) => ({ ...value, endDate }))} /><Field label={`学历层次 ${index + 1}`} value={item.degree} onChange={(degree) => change(item.id, (value) => ({ ...value, degree }))} /><Field label={`学院 ${index + 1}`} value={item.college} onChange={(college) => change(item.id, (value) => ({ ...value, college }))} /><Field label={`专业 ${index + 1}`} value={item.major} onChange={(major) => change(item.id, (value) => ({ ...value, major }))} /></div>}</SimpleItemsEditor>;
}

function SkillsItemsEditor({ section, update }: { section: Extract<ResumeSection, { type: 'skills' }>; update: (fn: (s: ResumeSection) => ResumeSection) => void }) {
  const change = (id: string, fn: (item: SkillItem) => SkillItem) => update(() => ({ ...section, content: { items: updateItems(section.content.items, id, fn) } }));
  return <SimpleItemsEditor addLabel="添加技能" items={section.content.items} addItem={() => update(() => ({ ...section, content: { items: [...section.content.items, { id: nextId(), name: '', note: '' }] } }))} removeItem={(id) => update(() => ({ ...section, content: { items: section.content.items.filter((item) => item.id !== id) } }))}>{(item, index) => <><Field label={`技能 ${index + 1}`} value={item.name} onChange={(name) => change(item.id, (value) => ({ ...value, name }))} /><Field label={`备注 ${index + 1}`} value={item.note} onChange={(note) => change(item.id, (value) => ({ ...value, note }))} /></>}</SimpleItemsEditor>;
}

function CampusItemsEditor({ section, update }: { section: Extract<ResumeSection, { type: 'campus' }>; update: (fn: (s: ResumeSection) => ResumeSection) => void }) {
  const change = (id: string, fn: (item: CampusItem) => CampusItem) => update(() => ({ ...section, content: { items: updateItems(section.content.items, id, fn) } }));
  return <SimpleItemsEditor addLabel="添加校园条目" items={section.content.items} addItem={() => update(() => ({ ...section, content: { items: [...section.content.items, { id: nextId(), name: '', role: '', highlightsMarkdown: '' }] } }))} removeItem={(id) => update(() => ({ ...section, content: { items: section.content.items.filter((item) => item.id !== id) } }))}>{(item, index) => <><Field label={`活动或部门 ${index + 1}`} value={item.name} onChange={(name) => change(item.id, (value) => ({ ...value, name }))} /><Field label={`担任角色 ${index + 1}`} value={item.role} onChange={(role) => change(item.id, (value) => ({ ...value, role }))} /><Field label={`列表说明 ${index + 1}`} multiline value={item.highlightsMarkdown} onChange={(highlightsMarkdown) => change(item.id, (value) => ({ ...value, highlightsMarkdown }))} /></>}</SimpleItemsEditor>;
}

function HonoursItemsEditor({ section, update }: { section: Extract<ResumeSection, { type: 'honours' }>; update: (fn: (s: ResumeSection) => ResumeSection) => void }) {
  const change = (id: string, fn: (item: HonourItem) => HonourItem) => update(() => ({ ...section, content: { items: updateItems(section.content.items, id, fn) } }));
  return <SimpleItemsEditor addLabel="添加荣誉奖项" items={section.content.items} addItem={() => update(() => ({ ...section, content: { items: [...section.content.items, { id: nextId(), name: '' }] } }))} removeItem={(id) => update(() => ({ ...section, content: { items: section.content.items.filter((item) => item.id !== id) } }))}>{(item, index) => <Field label={`奖项名称 ${index + 1}`} value={item.name} onChange={(name) => change(item.id, (value) => ({ ...value, name }))} />}</SimpleItemsEditor>;
}
