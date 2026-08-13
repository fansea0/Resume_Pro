import type { ResumeDocument } from '../types/resume';

interface TemplateProps {
  document: ResumeDocument;
}

export function EditorialTemplate({ document }: TemplateProps) {
  const { profile, experiences, projects, education, skills } = document;
  const contact = [profile.email, profile.phone, profile.location, profile.website].filter(Boolean);

  return (
    <article className="resume-template template-editorial">
      <header className="template-editorial__header"><p>PROFILE</p><h1>{profile.fullName}</h1><div><span>{profile.headline}</span><span>{contact.join(' · ')}</span></div></header>
      <div className="template-editorial__layout">
        <aside className="template-editorial__sidebar">
          {profile.summary && <section><h2>自我介绍</h2><p>{profile.summary}</p></section>}
          {skills.length > 0 && <section><h2>技能专长</h2>{skills.map((skill) => <p key={skill.id}><strong>{skill.name}</strong>{skill.level && <span>{skill.level}</span>}</p>)}</section>}
          {education.length > 0 && <section><h2>教育背景</h2>{education.map((item) => <p key={item.id}><strong>{item.school}</strong><span>{item.degree} · {item.field}</span><small>{item.startDate} — {item.endDate}</small></p>)}</section>}
        </aside>
        <main className="template-editorial__content">
          {experiences.length > 0 && <section><h2>职业经历</h2>{experiences.map((experience) => <div className="template-editorial__entry" key={experience.id}><p className="template-editorial__date">{experience.startDate} — {experience.endDate}</p><h3>{experience.company}</h3><p className="template-editorial__role">{experience.role}</p>{experience.highlights.map((highlight, index) => <p key={index}>{highlight}</p>)}</div>)}</section>}
          {projects.length > 0 && <section><h2>代表项目</h2>{projects.map((project) => <div className="template-editorial__entry" key={project.id}><p className="template-editorial__date">{project.startDate} — {project.endDate}</p><h3>{project.name}</h3><p className="template-editorial__role">{project.role}{project.link && ` · ${project.link}`}</p><p>{project.description}</p>{project.highlights.map((highlight, index) => <p key={index}>{highlight}</p>)}</div>)}</section>}
        </main>
      </div>
    </article>
  );
}
