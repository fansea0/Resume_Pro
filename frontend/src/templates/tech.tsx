import type { ResumeDocument } from '../types/resume';

interface TemplateProps {
  document: ResumeDocument;
}

export function TechTemplate({ document }: TemplateProps) {
  const { profile, experiences, projects, education, skills } = document;
  const contact = [profile.email, profile.phone, profile.location, profile.website].filter(Boolean);

  return (
    <article className="resume-template template-tech">
      <header className="template-tech__header">
        <p className="template-tech__eyebrow">CAREER SIGNAL</p>
        <h1>{profile.fullName}</h1>
        <p className="template-tech__headline">{profile.headline}</p>
        {contact.length > 0 && <p className="template-tech__contact">{contact.join(' / ')}</p>}
      </header>
      <main className="template-tech__content">
        {profile.summary && <section className="template-tech__summary"><h2>个人摘要</h2><p>{profile.summary}</p></section>}
        {experiences.length > 0 && <section><h2>工作经历</h2>{experiences.map((experience) => (
          <div className="template-tech__entry" key={experience.id}>
            <p className="template-tech__date">{experience.startDate} — {experience.endDate}</p><h3>{experience.role}</h3><p className="template-tech__company">{experience.company}</p>
            {experience.highlights.map((highlight, index) => <p key={index}>{highlight}</p>)}
          </div>
        ))}</section>}
        {projects.length > 0 && <section><h2>项目经历</h2>{projects.map((project) => (
          <div className="template-tech__entry" key={project.id}>
            <p className="template-tech__date">{project.startDate} — {project.endDate}</p><h3>{project.name}</h3><p className="template-tech__company">{project.role}{project.link && ` · ${project.link}`}</p><p>{project.description}</p>
            {project.highlights.map((highlight, index) => <p key={index}>{highlight}</p>)}
          </div>
        ))}</section>}
        {education.length > 0 && <section><h2>教育经历</h2>{education.map((item) => <div className="template-tech__education" key={item.id}><strong>{item.school}</strong><span>{item.degree} · {item.field}</span><time>{item.startDate} — {item.endDate}</time></div>)}</section>}
        {skills.length > 0 && <section><h2>核心技能</h2><div className="template-tech__skills">{skills.map((skill) => <span key={skill.id}>{skill.name}{skill.level && <small>{skill.level}</small>}</span>)}</div></section>}
      </main>
    </article>
  );
}
