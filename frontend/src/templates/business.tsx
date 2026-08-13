import type { ResumeDocument } from '../types/resume';

interface TemplateProps {
  document: ResumeDocument;
}

export function BusinessTemplate({ document }: TemplateProps) {
  const { profile, experiences, projects, education, skills } = document;

  return (
    <article className="resume-template template-business">
      <header className="template-business__header">
        <div><p className="template-business__eyebrow">CURRICULUM VITAE</p><h1>{profile.fullName}</h1><p>{profile.headline}</p></div>
        <address>{[profile.email, profile.phone, profile.location, profile.website].filter(Boolean).map((item) => <span key={item}>{item}</span>)}</address>
      </header>
      {profile.summary && <section className="template-business__intro"><h2>个人简介</h2><p>{profile.summary}</p></section>}
      {experiences.length > 0 && <section><h2>工作经历</h2>{experiences.map((experience) => <div className="template-business__entry" key={experience.id}><div className="template-business__period">{experience.startDate}<br />{experience.endDate}</div><div><h3>{experience.role}</h3><p className="template-business__company">{experience.company}</p>{experience.highlights.map((highlight, index) => <p key={index}>{highlight}</p>)}</div></div>)}</section>}
      {projects.length > 0 && <section><h2>项目经历</h2>{projects.map((project) => <div className="template-business__entry" key={project.id}><div className="template-business__period">{project.startDate}<br />{project.endDate}</div><div><h3>{project.name}</h3><p>{project.role}</p><p>{project.description}</p></div></div>)}</section>}
      {education.length > 0 && <section><h2>教育背景</h2>{education.map((item) => <div className="template-business__entry" key={item.id}><div className="template-business__period">{item.startDate}<br />{item.endDate}</div><div><h3>{item.school}</h3><p>{item.degree} · {item.field}</p></div></div>)}</section>}
      {skills.length > 0 && <section><h2>技能</h2><p>{skills.map((skill) => `${skill.name}${skill.level ? ` (${skill.level})` : ''}`).join('　')}</p></section>}
    </article>
  );
}
