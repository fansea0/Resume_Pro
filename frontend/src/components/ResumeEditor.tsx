import type {
  Education,
  Project,
  ResumeDocument,
  ResumeProfile,
  Skill,
  WorkExperience,
} from '../types/resume';

interface ResumeEditorProps {
  document: ResumeDocument;
  onChange: (document: ResumeDocument) => void;
}

function nextId() {
  return crypto.randomUUID();
}

function Field({ label, value, onChange, multiline = false }: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  multiline?: boolean;
}) {
  const id = `field-${label}`;
  return (
    <label className="editor-field" htmlFor={id}>
      <span>{label}</span>
      {multiline ? (
        <textarea id={id} onChange={(event) => onChange(event.target.value)} value={value} />
      ) : (
        <input id={id} onChange={(event) => onChange(event.target.value)} value={value} />
      )}
    </label>
  );
}

function Section({ title, addLabel, onAdd, children }: {
  title: string;
  addLabel: string;
  onAdd: () => void;
  children: React.ReactNode;
}) {
  return (
    <section className="editor-section">
      <div className="editor-section__heading"><h2>{title}</h2><button onClick={onAdd} type="button">{addLabel}</button></div>
      {children}
    </section>
  );
}

export function ResumeEditor({ document, onChange }: ResumeEditorProps) {
  const updateProfile = (field: keyof ResumeProfile, value: string) => {
    onChange({ ...document, profile: { ...document.profile, [field]: value } });
  };
  const updateItem = <T extends { id: string }>(key: 'experiences' | 'projects' | 'education' | 'skills', id: string, field: keyof T, value: string) => {
    const items = document[key] as unknown as T[];
    onChange({ ...document, [key]: items.map((item) => item.id === id ? { ...item, [field]: value } : item) });
  };
  const removeItem = (key: 'experiences' | 'projects' | 'education' | 'skills', id: string) => {
    onChange({ ...document, [key]: document[key].filter((item) => item.id !== id) });
  };
  const addExperience = () => onChange({ ...document, experiences: [...document.experiences, {
    id: nextId(), company: '', role: '', startDate: '', endDate: '', highlights: [],
  }] });
  const addProject = () => onChange({ ...document, projects: [...document.projects, {
    id: nextId(), name: '', role: '', link: '', startDate: '', endDate: '', description: '', highlights: [],
  }] });
  const addEducation = () => onChange({ ...document, education: [...document.education, {
    id: nextId(), school: '', degree: '', field: '', startDate: '', endDate: '',
  }] });
  const addSkill = () => onChange({ ...document, skills: [...document.skills, { id: nextId(), name: '', level: '' }] });

  return (
    <form className="resume-editor" onSubmit={(event) => event.preventDefault()}>
      <section className="editor-section">
        <h2>基本信息</h2>
        <div className="editor-fields editor-fields--two">
          <Field label="姓名" onChange={(value) => updateProfile('fullName', value)} value={document.profile.fullName} />
          <Field label="求职方向" onChange={(value) => updateProfile('headline', value)} value={document.profile.headline} />
          <Field label="邮箱" onChange={(value) => updateProfile('email', value)} value={document.profile.email} />
          <Field label="电话" onChange={(value) => updateProfile('phone', value)} value={document.profile.phone} />
          <Field label="所在地" onChange={(value) => updateProfile('location', value)} value={document.profile.location} />
          <Field label="个人网站" onChange={(value) => updateProfile('website', value)} value={document.profile.website} />
        </div>
        <Field label="个人简介" multiline onChange={(value) => updateProfile('summary', value)} value={document.profile.summary} />
      </section>

      <Section addLabel="添加工作经历" onAdd={addExperience} title="工作经历">
        {document.experiences.map((item, index) => <div className="editor-card" key={item.id}>
          <div className="editor-card__heading"><h3>工作经历 {index + 1}</h3><button aria-label={`删除工作经历 ${index + 1}`} onClick={() => removeItem('experiences', item.id)} type="button">删除</button></div>
          <div className="editor-fields editor-fields--two">
            <Field label={`公司 ${index + 1}`} onChange={(value) => updateItem<WorkExperience>('experiences', item.id, 'company', value)} value={item.company} />
            <Field label={`职位 ${index + 1}`} onChange={(value) => updateItem<WorkExperience>('experiences', item.id, 'role', value)} value={item.role} />
            <Field label={`开始时间 ${index + 1}`} onChange={(value) => updateItem<WorkExperience>('experiences', item.id, 'startDate', value)} value={item.startDate} />
            <Field label={`结束时间 ${index + 1}`} onChange={(value) => updateItem<WorkExperience>('experiences', item.id, 'endDate', value)} value={item.endDate} />
          </div>
        </div>)}
      </Section>

      <Section addLabel="添加项目经历" onAdd={addProject} title="项目经历">
        {document.projects.map((item, index) => <div className="editor-card" key={item.id}>
          <div className="editor-card__heading"><h3>项目经历 {index + 1}</h3><button aria-label={`删除项目经历 ${index + 1}`} onClick={() => removeItem('projects', item.id)} type="button">删除</button></div>
          <div className="editor-fields editor-fields--two">
            <Field label={`项目名称 ${index + 1}`} onChange={(value) => updateItem<Project>('projects', item.id, 'name', value)} value={item.name} />
            <Field label={`项目角色 ${index + 1}`} onChange={(value) => updateItem<Project>('projects', item.id, 'role', value)} value={item.role} />
            <Field label={`项目链接 ${index + 1}`} onChange={(value) => updateItem<Project>('projects', item.id, 'link', value)} value={item.link} />
            <Field label={`项目描述 ${index + 1}`} onChange={(value) => updateItem<Project>('projects', item.id, 'description', value)} value={item.description} />
          </div>
        </div>)}
      </Section>

      <Section addLabel="添加教育经历" onAdd={addEducation} title="教育经历">
        {document.education.map((item, index) => <div className="editor-card" key={item.id}>
          <div className="editor-card__heading"><h3>教育经历 {index + 1}</h3><button aria-label={`删除教育经历 ${index + 1}`} onClick={() => removeItem('education', item.id)} type="button">删除</button></div>
          <div className="editor-fields editor-fields--two">
            <Field label={`学校 ${index + 1}`} onChange={(value) => updateItem<Education>('education', item.id, 'school', value)} value={item.school} />
            <Field label={`学位 ${index + 1}`} onChange={(value) => updateItem<Education>('education', item.id, 'degree', value)} value={item.degree} />
            <Field label={`专业 ${index + 1}`} onChange={(value) => updateItem<Education>('education', item.id, 'field', value)} value={item.field} />
          </div>
        </div>)}
      </Section>

      <Section addLabel="添加技能" onAdd={addSkill} title="专业技能">
        {document.skills.map((item, index) => <div className="editor-card editor-card--inline" key={item.id}>
          <Field label={`技能 ${index + 1}`} onChange={(value) => updateItem<Skill>('skills', item.id, 'name', value)} value={item.name} />
          <Field label={`熟练度 ${index + 1}`} onChange={(value) => updateItem<Skill>('skills', item.id, 'level', value)} value={item.level} />
          <button aria-label={`删除技能 ${index + 1}`} onClick={() => removeItem('skills', item.id)} type="button">删除</button>
        </div>)}
      </Section>
    </form>
  );
}
