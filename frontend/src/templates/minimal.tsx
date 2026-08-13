import type { ResumeDocument } from '../types/resume';

interface TemplateProps {
  document: ResumeDocument;
}

export function MinimalTemplate({ document }: TemplateProps) {
  const { profile, experiences, projects, education, skills } = document;

  return (
    <article className="resume-template template-minimal">
      <header className="template-minimal__header">
        <h1>{profile.fullName}</h1>
        <p className="template-minimal__headline">{profile.headline}</p>
        <p className="template-minimal__contact">
          {[profile.email, profile.phone, profile.location, profile.website].filter(Boolean).join(' · ')}
        </p>
      </header>
      {profile.summary && <p className="template-minimal__summary">{profile.summary}</p>}
      {experiences.length > 0 && <section><h2>工作经历</h2>{experiences.map((experience) => (
        <div className="template-minimal__entry" key={experience.id}>
          <div><strong>{experience.company}</strong><span>{experience.role}</span></div>
          <time>{experience.startDate} — {experience.endDate}</time>
          {experience.highlights.map((highlight, index) => <p key={index}>{highlight}</p>)}
        </div>
      ))}</section>}
      {projects.length > 0 && <section><h2>项目经历</h2>{projects.map((project) => <div className="template-minimal__entry" key={project.id}><strong>{project.name}</strong><span>{project.role}</span><p>{project.description}</p></div>)}</section>}
      {education.length > 0 && <section><h2>教育经历</h2>{education.map((item) => <div className="template-minimal__entry" key={item.id}><strong>{item.school}</strong><span>{item.degree} · {item.field}</span></div>)}</section>}
      {skills.length > 0 && <section><h2>专业技能</h2><p>{skills.map((skill) => skill.name).join(' · ')}</p></section>}
    </article>
  );
}
