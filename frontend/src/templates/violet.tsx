import type { ResumeDocument } from '../types/resume';

interface TemplateProps {
  document: ResumeDocument;
}

function SectionHeading({ children }: { children: string }) {
  return <h2 className="template-violet__section-heading">{children}</h2>;
}

function EntryMeta({ startDate, endDate, location }: { startDate: string; endDate: string; location?: string }) {
  return <span className="template-violet__entry-meta">{[startDate, endDate].filter(Boolean).join(' — ')}{location && ` · ${location}`}</span>;
}

export function VioletTemplate({ document }: TemplateProps) {
  const { profile, experiences, projects, education, skills } = document;
  const contact = [profile.phone, profile.email, profile.website].filter(Boolean).join('  |  ');
  const initial = profile.fullName.trim().slice(0, 1) || '简';

  return (
    <article className="resume-template template-violet">
      <header className="template-violet__header">
        <div className="template-violet__identity">
          <h1>{profile.fullName}</h1>
          {contact && <p>{contact}</p>}
          <p>{profile.headline}</p>
        </div>
        <div aria-label="候选人头像占位" className="template-violet__avatar">{initial}</div>
      </header>

      {profile.summary && <section><SectionHeading>个人总结</SectionHeading><p className="template-violet__summary">{profile.summary}</p></section>}
      {experiences.length > 0 && <section><SectionHeading>工作经历</SectionHeading>{experiences.map((experience) => <div className="template-violet__entry" key={experience.id}>
        <div className="template-violet__entry-top"><h3>{experience.company}</h3><EntryMeta endDate={experience.endDate} location={profile.location} startDate={experience.startDate} /></div>
        <p className="template-violet__role">{experience.role}</p>
        {experience.highlights.length > 0 && <ul>{experience.highlights.map((highlight, index) => <li key={index}>{highlight}</li>)}</ul>}
      </div>)}</section>}
      {projects.length > 0 && <section><SectionHeading>项目经历</SectionHeading>{projects.map((project) => <div className="template-violet__entry" key={project.id}>
        <div className="template-violet__entry-top"><h3>{project.name}</h3><EntryMeta endDate={project.endDate} startDate={project.startDate} /></div>
        <p className="template-violet__role">{[project.role, project.link].filter(Boolean).join(' · ')}</p>
        {project.description && <p>{project.description}</p>}
        {project.highlights.length > 0 && <ul>{project.highlights.map((highlight, index) => <li key={index}>{highlight}</li>)}</ul>}
      </div>)}</section>}
      {education.length > 0 && <section><SectionHeading>教育经历</SectionHeading>{education.map((item) => <div className="template-violet__entry template-violet__education" key={item.id}>
        <div className="template-violet__entry-top"><h3>{item.school}</h3><EntryMeta endDate={item.endDate} startDate={item.startDate} /></div><p className="template-violet__role">{item.degree}{item.field && ` · ${item.field}`}</p>
      </div>)}</section>}
      {skills.length > 0 && <section><SectionHeading>专业技能</SectionHeading><div className="template-violet__skills">{skills.map((skill) => <span key={skill.id}>{skill.name}{skill.level && <small>{skill.level}</small>}</span>)}</div></section>}
    </article>
  );
}
