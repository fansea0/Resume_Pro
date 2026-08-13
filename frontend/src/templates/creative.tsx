import type { ResumeDocument } from '../types/resume';

interface TemplateProps {
  document: ResumeDocument;
}

export function CreativeTemplate({ document }: TemplateProps) {
  const { profile, experiences, projects, education, skills } = document;

  return (
    <article className="resume-template template-creative">
      <aside className="template-creative__sidebar">
        <p className="template-creative__label">PORTFOLIO</p><h1>{profile.fullName}</h1><p className="template-creative__headline">{profile.headline}</p>
        <section><h2>联系</h2>{[profile.email, profile.phone, profile.location, profile.website].filter(Boolean).map((item) => <p key={item}>{item}</p>)}</section>
        {skills.length > 0 && <section><h2>技能</h2>{skills.map((skill) => <p key={skill.id}>{skill.name}{skill.level && <small>{skill.level}</small>}</p>)}</section>}
        {education.length > 0 && <section><h2>教育</h2>{education.map((item) => <p key={item.id}><strong>{item.school}</strong><br />{item.degree} · {item.field}</p>)}</section>}
      </aside>
      <main className="template-creative__content">
        {profile.summary && <section className="template-creative__summary"><h2>关于我</h2><p>{profile.summary}</p></section>}
        {experiences.length > 0 && <section><h2>经历</h2>{experiences.map((experience) => <div className="template-creative__entry" key={experience.id}><p className="template-creative__date">{experience.startDate} — {experience.endDate}</p><h3>{experience.company}</h3><p className="template-creative__role">{experience.role}</p>{experience.highlights.map((highlight, index) => <p key={index}>{highlight}</p>)}</div>)}</section>}
        {projects.length > 0 && <section><h2>项目</h2>{projects.map((project) => <div className="template-creative__entry" key={project.id}><p className="template-creative__date">{project.startDate} — {project.endDate}</p><h3>{project.name}</h3><p className="template-creative__role">{project.role}</p><p>{project.description}</p></div>)}</section>}
      </main>
    </article>
  );
}
